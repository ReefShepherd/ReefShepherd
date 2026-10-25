package dashboard

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func wantShape(t *testing.T, got []Card, want ...string) {
	t.Helper()
	if s := shape(got); !reflect.DeepEqual(s, want) {
		t.Fatalf("cards = %v, want %v", s, want)
	}
	for i, c := range got {
		if c.Position != int64(i) {
			t.Errorf("card %d has position %d", i, c.Position)
		}
	}
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

func TestDefaultLayoutIsCreatedOnFirstRequest(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	first, second, third, off, archived := e.param("First"), e.param("Second"), e.param("Third"), e.param("Off"), e.param("Archived")
	// Tank-level order differs from creation order, a disabled parameter and an archived definition are skipped.
	e.enable(tank, third, true, 1)
	e.enable(tank, first, true, 2)
	e.enable(tank, second, true, 3)
	e.enable(tank, off, false, 0)
	e.enable(tank, archived, true, 0)
	testutil.Exec(e.t, e.db, `UPDATE parameter_definitions SET archived_at = '2026-01-01T00:00:00Z' WHERE id = ?`, archived)

	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards`); n != 0 {
		t.Fatalf("%d cards exist before the first request", n)
	}
	cards := e.get(userA, tank)
	wantShape(t, cards, "tests_due:-:normal", "parameter:"+id(third)+":normal", "parameter:"+id(first)+":normal", "parameter:"+id(second)+":normal")
	for _, c := range cards {
		if c.ID == 0 || string(c.Options) != "{}" {
			t.Errorf("card = %+v", c)
		}
	}

	// A second request returns the stored cards instead of making new ones.
	again := e.get(userA, tank)
	if !reflect.DeepEqual(cards, again) {
		t.Errorf("second GET = %+v, want %+v", again, cards)
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards`); n != 4 {
		t.Errorf("%d cards stored, want 4", n)
	}
}

func TestDefaultLayoutForTankWithoutParameters(t *testing.T) {
	e := newEnv(t)
	wantShape(t, e.get(userA, e.tank("Empty")), "tests_due:-:normal")
}

func TestConcurrentFirstRequestsCreateOneLayout(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	e.enable(tank, e.param("A"), true, 0)
	e.enable(tank, e.param("B"), true, 1)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			e.do(userA, http.MethodGet, dashPath(tank, ""), nil)
		}()
	}
	close(start)
	wg.Wait()
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards WHERE user_id = ? AND tank_id = ?`, userA, tank); n != 3 {
		t.Errorf("%d cards stored after concurrent first requests, want 3", n)
	}
}

func TestReplaceStoresTheWholeListInOrder(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	a, b := e.param("A"), e.param("B")
	e.enable(tank, a, true, 0)
	e.enable(tank, b, true, 1)
	e.get(userA, tank)

	// The array order decides; the positions the client sends are not trusted.
	got := e.put(userA, tank, []map[string]any{
		{"card_type": "parameter_chart", "parameter_id": b, "position": 7, "size": "wide", "options": map[string]any{"range": "30d"}},
		{"card_type": "tests_due", "position": 7},
		{"card_type": "parameter", "parameter_id": a, "position": -3, "size": "normal", "options": nil},
		{"card_type": "parameter", "parameter_id": b},
	})
	wantShape(t, got, "parameter_chart:"+id(b)+":wide", "tests_due:-:normal", "parameter:"+id(a)+":normal", "parameter:"+id(b)+":normal")
	if string(got[0].Options) != `{"range":"30d"}` || string(got[1].Options) != "{}" || string(got[2].Options) != "{}" {
		t.Errorf("options = %s %s %s", got[0].Options, got[1].Options, got[2].Options)
	}
	if !reflect.DeepEqual(e.get(userA, tank), got) {
		t.Error("GET after PUT differs from the PUT response")
	}

	// A second replace drops what the first one stored, including cards it left out.
	got = e.put(userA, tank, []map[string]any{{"card_type": "tests_due", "size": "wide"}})
	wantShape(t, got, "tests_due:-:wide")
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards`); n != 1 {
		t.Errorf("%d cards stored, want 1", n)
	}
}

func TestReplaceStoresOptionsCompacted(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	p := e.param("A")
	e.enable(tank, p, true, 0)
	body := `{"cards":[{"card_type":"parameter_chart","parameter_id":` + id(p) + `,"options":{ "b" : 1,  "a" : [1, 2] }}]}`
	rec := testutil.DoRaw(e.t, e.mux, http.MethodPut, dashPath(tank, ""), "application/json", body, testutil.AsUser(userA, httpx.RoleMember))
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	var stored string
	if err := e.db.R.QueryRow(`SELECT options_json FROM dashboard_cards`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != `{"b":1,"a":[1,2]}` {
		t.Errorf("options_json = %q", stored)
	}
	if got := testutil.Decode[Layout](t, rec).Cards[0]; string(got.Options) != stored {
		t.Errorf("response options = %s", got.Options)
	}
}

func TestReplaceValidation(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	other := e.tank("Other")
	on, off, foreign := e.param("On"), e.param("Off"), e.param("Foreign")
	e.enable(tank, on, true, 0)
	e.enable(tank, off, false, 1)
	e.enable(other, foreign, true, 0)
	before := e.get(userA, tank)

	longOptions := map[string]any{"x": string(make([]byte, 0))}
	pad := make([]byte, maxOptionsBytes)
	for i := range pad {
		pad[i] = 'a'
	}
	longOptions["x"] = string(pad)

	tooMany := make([]map[string]any, maxCards+1)
	for i := range tooMany {
		tooMany[i] = map[string]any{"card_type": "tests_due"}
	}

	cases := []struct {
		name  string
		cards any
		want  map[string]string
	}{
		{"no cards", []map[string]any{}, map[string]string{"cards": "must have at least one card"}},
		{"cards missing", nil, map[string]string{"cards": "must have at least one card"}},
		{"too many", tooMany, map[string]string{"cards": fmt.Sprintf("must have at most %d cards", maxCards)}},
		{"missing type", []map[string]any{{}}, map[string]string{"cards.0.card_type": "is required"}},
		{"unknown type", []map[string]any{{"card_type": "feeding_today"}}, map[string]string{"cards.0.card_type": "must be one of parameter, parameter_chart, tests_due"}},
		{"parameter without id", []map[string]any{{"card_type": "tests_due"}, {"card_type": "parameter"}}, map[string]string{"cards.1.parameter_id": "is required for this card type"}},
		{"chart without id", []map[string]any{{"card_type": "parameter_chart"}}, map[string]string{"cards.0.parameter_id": "is required for this card type"}},
		{"disabled parameter", []map[string]any{{"card_type": "parameter", "parameter_id": off}}, map[string]string{"cards.0.parameter_id": "must be a parameter enabled on this tank"}},
		{"other tank's parameter", []map[string]any{{"card_type": "parameter", "parameter_id": foreign}}, map[string]string{"cards.0.parameter_id": "must be a parameter enabled on this tank"}},
		{"unknown parameter", []map[string]any{{"card_type": "parameter_chart", "parameter_id": 99999}}, map[string]string{"cards.0.parameter_id": "must be a parameter enabled on this tank"}},
		{"tests_due with parameter", []map[string]any{{"card_type": "tests_due", "parameter_id": on}}, map[string]string{"cards.0.parameter_id": "must be empty for this card type"}},
		{"bad size", []map[string]any{{"card_type": "tests_due", "size": "huge"}}, map[string]string{"cards.0.size": "must be one of normal, wide"}},
		{"options not an object", []map[string]any{{"card_type": "tests_due", "options": []int{1}}}, map[string]string{"cards.0.options": "must be a JSON object"}},
		{"options a string", []map[string]any{{"card_type": "tests_due", "options": "x"}}, map[string]string{"cards.0.options": "must be a JSON object"}},
		{"options too big", []map[string]any{{"card_type": "tests_due", "options": longOptions}}, map[string]string{"cards.0.options": fmt.Sprintf("must be at most %d bytes", maxOptionsBytes)}},
		{"duplicate tests_due", []map[string]any{{"card_type": "tests_due"}, {"card_type": "tests_due"}}, map[string]string{"cards.1": "duplicates cards.0"}},
		{"duplicate parameter", []map[string]any{
			{"card_type": "parameter", "parameter_id": on}, {"card_type": "parameter_chart", "parameter_id": on}, {"card_type": "parameter", "parameter_id": on},
		}, map[string]string{"cards.2": "duplicates cards.0"}},
		{"several problems at once", []map[string]any{
			{"card_type": "nope"}, {"card_type": "parameter"}, {"card_type": "tests_due", "size": "tiny"},
		}, map[string]string{
			"cards.0.card_type":    "must be one of parameter, parameter_chart, tests_due",
			"cards.1.parameter_id": "is required for this card type",
			"cards.2.size":         "must be one of normal, wide",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := map[string]any{"cards": c.cards}
			rec := e.do(userA, http.MethodPut, dashPath(tank, ""), body)
			got := testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation").Error.Fields
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("fields = %v, want %v", got, c.want)
			}
			if after := e.get(userA, tank); !reflect.DeepEqual(after, before) {
				t.Errorf("a rejected replace changed the stored cards: %+v", after)
			}
		})
	}
}

func TestReplaceRejectsMalformedBodies(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")

	rec := e.do(userA, http.MethodPut, dashPath(tank, ""), map[string]any{"cards": []map[string]any{{"card_type": "tests_due", "colour": "red"}}})
	testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation")
	rec = e.do(userA, http.MethodPut, dashPath(tank, ""), map[string]any{"cards": []map[string]any{{"card_type": "tests_due", "size": 3}}})
	testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation")
	rec = e.do(userA, http.MethodPut, dashPath(tank, ""), map[string]any{"cards": "all of them"})
	testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation")
	rec = e.do(userA, http.MethodPut, dashPath(tank, ""), map[string]any{"extra": 1, "cards": []map[string]any{{"card_type": "tests_due"}}})
	testutil.ExpectError(e.t, rec, http.StatusUnprocessableEntity, "validation")
	testutil.ExpectError(e.t, e.do(userA, http.MethodPut, dashPath(tank, ""), nil), http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestResetRestoresTheDefault(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	a, b := e.param("A"), e.param("B")
	e.enable(tank, a, true, 0)
	def := e.get(userA, tank)
	wantShape(t, def, "tests_due:-:normal", "parameter:"+id(a)+":normal")

	e.put(userA, tank, []map[string]any{{"card_type": "parameter_chart", "parameter_id": a, "size": "wide"}})
	// A parameter enabled after the customisation shows up in the regenerated default.
	e.enable(tank, b, true, 1)

	rec := e.do(userA, http.MethodPost, dashPath(tank, "/reset"), nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	got := testutil.Decode[Layout](t, rec).Cards
	wantShape(t, got, "tests_due:-:normal", "parameter:"+id(a)+":normal", "parameter:"+id(b)+":normal")
	if !reflect.DeepEqual(e.get(userA, tank), got) {
		t.Error("GET after reset differs from the reset response")
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards`); n != 3 {
		t.Errorf("%d cards stored after reset, want 3", n)
	}

	// Reset on a user who never opened the dashboard also creates the default.
	rec = e.do(userB, http.MethodPost, dashPath(tank, "/reset"), nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	wantShape(t, testutil.Decode[Layout](t, rec).Cards, "tests_due:-:normal", "parameter:"+id(a)+":normal", "parameter:"+id(b)+":normal")
}

func TestResetRefusesNonJSONBodies(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	r := testutil.DoRaw(e.t, e.mux, http.MethodPost, dashPath(tank, "/reset"), "text/plain", "x", testutil.AsUser(userA, httpx.RoleMember))
	testutil.ExpectError(e.t, r, http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestEachUserHasTheirOwnLayoutPerTank(t *testing.T) {
	e := newEnv(t)
	t1, t2 := e.tank("One"), e.tank("Two")
	p := e.param("A")
	e.enable(t1, p, true, 0)
	e.enable(t2, p, true, 0)

	e.get(userA, t1)
	e.get(userB, t1)
	e.get(userA, t2)
	e.put(userA, t1, []map[string]any{{"card_type": "parameter", "parameter_id": p, "size": "wide"}})

	wantShape(t, e.get(userA, t1), "parameter:"+id(p)+":wide")
	wantShape(t, e.get(userB, t1), "tests_due:-:normal", "parameter:"+id(p)+":normal")
	wantShape(t, e.get(userA, t2), "tests_due:-:normal", "parameter:"+id(p)+":normal")

	// Another user's replace and reset leave the first user's layout alone.
	e.put(userB, t1, []map[string]any{{"card_type": "tests_due", "size": "wide"}})
	testutil.ExpectStatus(e.t, e.do(userB, http.MethodPost, dashPath(t1, "/reset"), nil), http.StatusOK)
	e.put(userB, t1, []map[string]any{{"card_type": "tests_due"}})
	wantShape(t, e.get(userA, t1), "parameter:"+id(p)+":wide")
	wantShape(t, e.get(userB, t1), "tests_due:-:normal")
}

func TestParameterCardsFollowTheParameterSwitch(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	a, b := e.param("A"), e.param("B")
	e.enable(tank, a, true, 0)
	e.enable(tank, b, true, 1)
	e.get(userA, tank)

	e.enable(tank, a, false, 0)
	wantShape(t, e.get(userA, tank), "tests_due:-:normal", "parameter:"+id(b)+":normal")
	e.enable(tank, a, true, 0)
	wantShape(t, e.get(userA, tank), "tests_due:-:normal", "parameter:"+id(a)+":normal", "parameter:"+id(b)+":normal")
}

func TestWhatGetShowsCanAlwaysBePutBackAfterAParameterIsArchived(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	a, b := e.param("A"), e.param("B")
	e.enable(tank, a, true, 0)
	e.enable(tank, b, true, 1)
	e.get(userA, tank)

	testutil.Exec(e.t, e.db, `UPDATE parameter_definitions SET archived_at = '2026-01-01T00:00:00Z' WHERE id = ?`, b)
	shown := e.get(userA, tank)
	wantShape(t, shown, "tests_due:-:normal", "parameter:"+id(a)+":normal")

	back := make([]map[string]any, len(shown))
	for i, c := range shown {
		back[i] = map[string]any{"card_type": c.CardType, "parameter_id": c.ParameterID, "position": c.Position, "size": c.Size, "options": c.Options}
	}
	wantShape(t, e.put(userA, tank, back), "tests_due:-:normal", "parameter:"+id(a)+":normal")

	// The archived parameter's card was kept and returns with the parameter.
	testutil.Exec(e.t, e.db, `UPDATE parameter_definitions SET archived_at = NULL WHERE id = ?`, b)
	wantShape(t, e.get(userA, tank), "tests_due:-:normal", "parameter:"+id(a)+":normal", "parameter:"+id(b)+":normal")
}

func TestReplaceKeepsHiddenCardsAfterTheShownOnesInTheirOrder(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	a, b, c := e.param("A"), e.param("B"), e.param("C")
	e.enable(tank, a, true, 0)
	e.enable(tank, b, true, 1)
	e.enable(tank, c, true, 2)
	e.get(userA, tank)
	e.put(userA, tank, []map[string]any{
		{"card_type": "parameter", "parameter_id": a, "options": map[string]any{"unit": "dkh"}},
		{"card_type": "parameter", "parameter_id": b},
		{"card_type": "parameter_chart", "parameter_id": c, "size": "wide"},
		{"card_type": "tests_due"},
	})

	e.enable(tank, a, false, 0)
	e.enable(tank, c, false, 2)
	wantShape(t, e.get(userA, tank), "parameter:"+id(b)+":normal", "tests_due:-:normal")

	got := e.put(userA, tank, []map[string]any{{"card_type": "tests_due"}, {"card_type": "parameter", "parameter_id": b}})
	wantShape(t, got, "tests_due:-:normal", "parameter:"+id(b)+":normal")

	e.enable(tank, a, true, 0)
	e.enable(tank, c, true, 2)
	got = e.get(userA, tank)
	wantShape(t, got, "tests_due:-:normal", "parameter:"+id(b)+":normal", "parameter:"+id(a)+":normal", "parameter_chart:"+id(c)+":wide")
	if string(got[2].Options) != `{"unit":"dkh"}` {
		t.Errorf("the kept card lost its options: %s", got[2].Options)
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards`); n != 4 {
		t.Errorf("%d cards stored, want 4", n)
	}
}

func TestEveryCardHiddenGivesAnEmptyListWithoutReseeding(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	a := e.param("A")
	e.enable(tank, a, true, 0)
	e.put(userA, tank, []map[string]any{{"card_type": "parameter", "parameter_id": a}})

	e.enable(tank, a, false, 0)
	rec := e.do(userA, http.MethodGet, dashPath(tank, ""), nil)
	testutil.ExpectStatus(e.t, rec, http.StatusOK)
	if body := strings.TrimSpace(rec.Body.String()); body != `{"cards":[]}` {
		t.Errorf("body = %s, want an empty list", body)
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards`); n != 1 {
		t.Errorf("%d cards stored, want the hidden one only", n)
	}

	// Adding a card keeps the hidden one for later.
	wantShape(t, e.put(userA, tank, []map[string]any{{"card_type": "tests_due"}}), "tests_due:-:normal")
	e.enable(tank, a, true, 0)
	wantShape(t, e.get(userA, tank), "tests_due:-:normal", "parameter:"+id(a)+":normal")
}

func TestMissingTank(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	for _, c := range []struct {
		method, suffix string
		body           any
	}{
		{http.MethodGet, "", nil},
		{http.MethodPut, "", map[string]any{"cards": []map[string]any{{"card_type": "tests_due"}}}},
		{http.MethodPost, "/reset", nil},
	} {
		testutil.ExpectError(e.t, e.do(userA, c.method, dashPath(tank+100, c.suffix), c.body), http.StatusNotFound, "not_found")
		testutil.ExpectError(e.t, e.do(userA, c.method, "/api/v1/tanks/abc/dashboard"+c.suffix, c.body), http.StatusNotFound, "not_found")
		testutil.ExpectError(e.t, e.do(userA, c.method, "/api/v1/tanks/0/dashboard"+c.suffix, c.body), http.StatusNotFound, "not_found")
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards`); n != 0 {
		t.Errorf("%d cards written for failed requests", n)
	}
}

func TestDeletingATankRemovesItsCards(t *testing.T) {
	e := newEnv(t)
	tank := e.tank("Display")
	e.get(userA, tank)
	testutil.Exec(e.t, e.db, `DELETE FROM tanks WHERE id = ?`, tank)
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM dashboard_cards`); n != 0 {
		t.Errorf("%d cards survived the tank", n)
	}
}
