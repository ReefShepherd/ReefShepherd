package params

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

func resultPath(id int64) string { return "/api/v1/results/" + strconv.FormatInt(id, 10) }

func TestMultiParameterEntryStoresEveryReadingAtOneTime(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Entry")
	temp, ph, alk := e.preset("temperature"), e.preset("ph"), e.preset("alkalinity")

	var out struct {
		Results []ResultView `json:"results"`
	}
	e.must(http.StatusCreated, http.MethodPost, tankPath(tank, "/results"), map[string]any{
		"measured_at": "2026-10-24T08:30:00-04:00",
		"results": []map[string]any{
			{"parameter_id": temp, "value": 25.2},
			{"parameter_id": ph, "value": 8.1, "note": "  after water change  "},
			{"parameter_id": alk, "value": 8.4},
		},
	}).decode(&out)
	if len(out.Results) != 3 {
		t.Fatalf("results = %+v", out.Results)
	}
	for i, want := range []struct {
		pid   int64
		name  string
		value float64
		note  string
	}{{temp, "Temperature", 25.2, ""}, {ph, "pH", 8.1, "after water change"}, {alk, "Alkalinity", 8.4, ""}} {
		r := out.Results[i]
		if r.ID == 0 || r.TankID != tank || r.ParameterID != want.pid || r.ParameterName != want.name || r.Value != want.value || r.Note != want.note ||
			r.MeasuredAt != "2026-10-24T12:30:00Z" || r.Source != "manual" || r.CreatedBy == nil || *r.CreatedBy != e.userID ||
			r.CreatedAt != "2026-10-24T12:00:00Z" || r.UpdatedAt != r.CreatedAt || !r.InRange {
			t.Errorf("result %d = %+v", i, r)
		}
	}
	if out.Results[0].ID >= out.Results[1].ID || out.Results[1].ID >= out.Results[2].ID {
		t.Errorf("ids = %d, %d, %d", out.Results[0].ID, out.Results[1].ID, out.Results[2].ID)
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM test_results WHERE measured_at = '2026-10-24T12:30:00Z'`); n != 3 {
		t.Errorf("%d rows at the shared time, want 3", n)
	}
}

func TestEntryWithoutATimeUsesNowAndDropsFractionalSeconds(t *testing.T) {
	e := newEnv(t)
	tank, ph := e.addTank("Now"), e.preset("ph")
	e.now = time.Date(2026, 10, 24, 12, 0, 7, 900_000_000, time.UTC)
	if got := e.addResults(tank, "", val(ph, 8.1)); got[0].MeasuredAt != "2026-10-24T12:00:07Z" {
		t.Errorf("default time = %s", got[0].MeasuredAt)
	}
	if got := e.addResults(tank, "2026-10-24T10:11:12.987Z", val(ph, 8.1)); got[0].MeasuredAt != "2026-10-24T10:11:12Z" {
		t.Errorf("fractional seconds kept: %s", got[0].MeasuredAt)
	}
}

func TestEntryIsAllOrNothing(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Atomic")
	ph, calcium := e.preset("ph"), e.preset("calcium")
	// The database refuses the second reading, after the first has already been written.
	testutil.Exec(e.t, e.db, `CREATE TRIGGER refuse_99 BEFORE INSERT ON test_results WHEN NEW.value = 99 BEGIN SELECT RAISE(ABORT, 'refused'); END`)

	hooked := 0
	e.svc.OnResultCreated = func(context.Context, Result) { hooked++ }
	e.fail(http.StatusInternalServerError, "internal", http.MethodPost, tankPath(tank, "/results"), map[string]any{
		"measured_at": "2026-10-24T09:00:00Z",
		"results":     []entry{val(ph, 8.1), val(calcium, 99)},
	})
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM test_results`); n != 0 {
		t.Errorf("%d results left behind by a failed entry", n)
	}
	if hooked != 0 {
		t.Errorf("hook called %d times for an entry that failed", hooked)
	}
	// The same entry without the refused value goes through.
	e.addResults(tank, "2026-10-24T09:00:00Z", val(ph, 8.1), val(calcium, 98))
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM test_results`); n != 2 {
		t.Errorf("%d results, want 2", n)
	}
}

func TestEntryIsCheckedInsideTheWriteTransaction(t *testing.T) {
	cases := []struct {
		name, change string
		status       int
		code         string
		field        string
	}{
		{"parameter archived while the entry waits for the writer", `UPDATE parameter_definitions SET archived_at = '2026-10-24T11:59:00Z' WHERE preset_key = 'ph'`, http.StatusUnprocessableEntity, "validation", "results.0.parameter_id"},
		{"tank deleted while the entry waits for the writer", `DELETE FROM tanks`, http.StatusNotFound, "not_found", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			tank, ph := e.addTank("Racy"), e.preset("ph")

			// The change is made by a writer that still holds the only write connection when the entry arrives.
			hold, err := e.db.W.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := hold.ExecContext(t.Context(), tc.change); err != nil {
				t.Fatal(err)
			}
			done := make(chan *resp, 1)
			go func() {
				done <- e.do(http.MethodPost, tankPath(tank, "/results"), map[string]any{"results": []entry{val(ph, 8.1)}})
			}()
			for deadline := time.Now().Add(5 * time.Second); e.db.W.Stats().WaitCount == 0; {
				if time.Now().After(deadline) {
					t.Fatal("the entry never waited for the writer")
				}
				time.Sleep(time.Millisecond)
			}
			if err := hold.Commit(); err != nil {
				t.Fatal(err)
			}

			r := <-done
			var b testutil.ErrorBody
			_ = json.Unmarshal(r.Body.Bytes(), &b)
			if r.Code != tc.status || b.Error.Code != tc.code || (tc.field != "" && b.Error.Fields[tc.field] == "") {
				t.Errorf("status=%d code=%q fields=%v, want %d %q %s; body=%s", r.Code, b.Error.Code, b.Error.Fields, tc.status, tc.code, tc.field, r.Body)
			}
			if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM test_results`); n != 0 {
				t.Errorf("%d results stored", n)
			}
		})
	}
}

func TestEntryValidationReportsEveryProblemAndStoresNothing(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Invalid")
	ph, alk, nitrate := e.preset("ph"), e.preset("alkalinity"), e.preset("nitrate")
	archived := e.customParameter(map[string]any{"name": "Old"})
	e.must(http.StatusOK, http.MethodPost, paramPath(archived, "/archive"), nil)
	path := tankPath(tank, "/results")

	tests := []struct {
		name   string
		body   map[string]any
		fields []string
	}{
		{"no results key", map[string]any{"measured_at": "2026-10-24T09:00:00Z"}, []string{"results"}},
		{"empty list", map[string]any{"results": []entry{}}, []string{"results"}},
		{"too many", map[string]any{"results": tooMany(ph)}, []string{"results"}},
		{"unknown parameter", map[string]any{"results": []entry{val(9999, 1)}}, []string{"results.0.parameter_id"}},
		{"archived parameter", map[string]any{"results": []entry{val(archived, 1)}}, []string{"results.0.parameter_id"}},
		{"listed twice", map[string]any{"results": []entry{val(ph, 8), val(ph, 8.1)}}, []string{"results.1.parameter_id"}},
		{"missing value", map[string]any{"results": []map[string]any{{"parameter_id": ph}}}, []string{"results.0.value"}},
		{"negative alkalinity", map[string]any{"results": []entry{val(alk, -1)}}, []string{"results.0.value"}},
		{"long note", map[string]any{"results": []entry{{ParameterID: ph, Value: ptr(8.1), Note: repeat("n", 2001)}}}, []string{"results.0.note"}},
		{"time without an offset", map[string]any{"measured_at": "2026-10-24 09:00", "results": []entry{val(ph, 8)}}, []string{"measured_at"}},
		{"time a week ahead", map[string]any{"measured_at": "2026-10-31T12:00:00Z", "results": []entry{val(ph, 8)}}, []string{"measured_at"}},
		{"value of the wrong type", map[string]any{"results": []map[string]any{{"parameter_id": ph, "value": "8.1"}}}, []string{"results.0.value"}},
		{"unknown entry field", map[string]any{"results": []map[string]any{{"parameter_id": ph, "value": 8, "unit": "x"}}}, []string{"unit"}},
		{"several at once", map[string]any{
			"measured_at": "yesterday",
			"results":     []entry{val(ph, 8.1), val(9999, 1), val(alk, -2), val(nitrate, 5), {ParameterID: nitrate}},
		}, []string{"measured_at", "results.1.parameter_id", "results.2.value", "results.4.parameter_id", "results.4.value"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e.invalid(http.MethodPost, path, tc.body, tc.fields...)
		})
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM test_results`); n != 0 {
		t.Errorf("%d results stored by rejected entries", n)
	}
	e.fail(http.StatusNotFound, "not_found", http.MethodPost, tankPath(9999, "/results"), map[string]any{"results": []entry{val(ph, 8)}})
}

func tooMany(pid int64) []entry {
	out := make([]entry, 101)
	for i := range out {
		out[i] = val(pid, 8)
	}
	return out
}

func TestOnResultCreatedRunsAfterCommitForEachResult(t *testing.T) {
	e := newEnv(t)
	tank, ph, calcium := e.addTank("Hook"), e.preset("ph"), e.preset("calcium")
	// With no hook set, which is how the app wires it, an entry just works.
	e.addResults(tank, "2026-10-24T08:00:00Z", val(ph, 8))

	var got []Result
	var visible []int
	e.svc.OnResultCreated = func(ctx context.Context, r Result) {
		got = append(got, r)
		visible = append(visible, testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM test_results WHERE id = ?`, r.ID))
	}
	created := e.addResults(tank, "2026-10-24T09:00:00.75Z", val(ph, 8.1), entry{ParameterID: calcium, Value: ptr(421.0), Note: "x"})
	if len(got) != 2 || !slices.Equal(visible, []int{1, 1}) {
		t.Fatalf("hook calls = %+v, rows visible when called = %v", got, visible)
	}
	want := Result{ID: created[1].ID, TankID: tank, ParameterID: calcium, MeasuredAt: at("2026-10-24T09:00:00Z"), Value: 421, Note: "x", Source: "manual", CreatedBy: &e.userID}
	if got[1].ID != want.ID || got[1].TankID != want.TankID || got[1].ParameterID != want.ParameterID || !got[1].MeasuredAt.Equal(want.MeasuredAt) ||
		got[1].Value != want.Value || got[1].Note != want.Note || got[1].Source != want.Source || got[1].CreatedBy == nil || *got[1].CreatedBy != e.userID {
		t.Errorf("hook result = %+v, want %+v", got[1], want)
	}
	if got[0].ID != created[0].ID || got[0].Value != 8.1 {
		t.Errorf("first hook result = %+v", got[0])
	}

	// A rejected entry never reaches the hook.
	e.invalid(http.MethodPost, tankPath(tank, "/results"), map[string]any{"results": []entry{val(ph, 8), val(9999, 1)}}, "results.1.parameter_id")
	if len(got) != 2 {
		t.Errorf("hook called for a rejected entry: %d calls", len(got))
	}
}

func TestResultListPagesNewestFirstWithACursor(t *testing.T) {
	e := newEnv(t)
	tank := e.addTank("Paging")
	temp, ph, calcium := e.preset("temperature"), e.preset("ph"), e.preset("calcium")
	for day := 18; day <= 21; day++ {
		e.addResults(tank, "2026-10-"+strconv.Itoa(day)+"T10:00:00Z", val(temp, 25))
	}
	// Three readings share the newest time, so a page can end in the middle of that group.
	e.addResults(tank, "2026-10-22T10:00:00Z", val(temp, 25), val(ph, 8), val(calcium, 420))
	otherTank := e.addTank("Elsewhere")
	e.addResults(otherTank, "2026-10-23T10:00:00Z", val(ph, 8))

	var want []int64
	rows, err := e.db.R.QueryContext(t.Context(), `SELECT id FROM test_results WHERE tank_id = ? ORDER BY measured_at DESC, id DESC`, tank)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		want = append(want, id)
	}
	_ = rows.Close()
	if len(want) != 7 {
		t.Fatalf("seeded %d results", len(want))
	}

	for _, limit := range []int{1, 2, 3, 6, 7, 50} {
		var got []int64
		cursor := ""
		pages := 0
		for {
			q := "?limit=" + strconv.Itoa(limit)
			if cursor != "" {
				q += "&cursor=" + url.QueryEscape(cursor)
			}
			var page ResultPage
			e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"+q), nil).decode(&page)
			pages++
			if len(page.Results) > limit {
				t.Fatalf("limit %d returned %d results", limit, len(page.Results))
			}
			for _, r := range page.Results {
				got = append(got, r.ID)
			}
			if page.NextCursor == nil {
				break
			}
			cursor = *page.NextCursor
			if pages > 20 {
				t.Fatal("paging does not end")
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("limit %d: got %v, want %v", limit, got, want)
		}
		// A full last page does not promise another one.
		if wantPages := (len(want) + limit - 1) / limit; pages != wantPages {
			t.Errorf("limit %d: %d pages, want %d", limit, pages, wantPages)
		}
	}

	// The default page size holds everything here and leaves no cursor.
	var page ResultPage
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"), nil).decode(&page)
	if len(page.Results) != 7 || page.NextCursor != nil || page.Results[0].MeasuredAt != "2026-10-22T10:00:00Z" || page.Results[6].MeasuredAt != "2026-10-18T10:00:00Z" {
		t.Errorf("default page = %d results, cursor %v", len(page.Results), page.NextCursor)
	}
}

func TestResultListFilters(t *testing.T) {
	e := newEnv(t)
	tank, temp, ph := e.addTank("Filters"), e.preset("temperature"), e.preset("ph")
	for day := 18; day <= 22; day++ {
		e.addResults(tank, "2026-10-"+strconv.Itoa(day)+"T10:00:00Z", val(temp, 25), val(ph, 8))
	}
	count := func(query string) int {
		t.Helper()
		var page ResultPage
		e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results?"+query), nil).decode(&page)
		return len(page.Results)
	}
	if n := count("parameter_id=" + strconv.FormatInt(ph, 10)); n != 5 {
		t.Errorf("parameter filter: %d, want 5", n)
	}
	// Both ends are inclusive.
	if n := count("from=2026-10-19T10:00:00Z&to=2026-10-21T10:00:00Z"); n != 6 {
		t.Errorf("from and to: %d, want 6", n)
	}
	if n := count("from=2026-10-22T10:00:00Z"); n != 2 {
		t.Errorf("from only: %d, want 2", n)
	}
	if n := count("to=2026-10-18T10:00:00Z"); n != 2 {
		t.Errorf("to only: %d, want 2", n)
	}
	if n := count("parameter_id=" + strconv.FormatInt(temp, 10) + "&from=2026-10-20T10:00:00Z&to=2026-10-21T10:00:00Z"); n != 2 {
		t.Errorf("combined: %d, want 2", n)
	}
	// Offsets are converted before comparing.
	if n := count("from=" + url.QueryEscape("2026-10-22T06:00:00-04:00")); n != 2 {
		t.Errorf("offset from: %d, want 2", n)
	}

	path := func(q string) string { return tankPath(tank, "/results?"+q) }
	e.invalid(http.MethodGet, path("parameter_id=abc"), nil, "parameter_id")
	e.invalid(http.MethodGet, path("parameter_id=0"), nil, "parameter_id")
	e.invalid(http.MethodGet, path("from=2026-10-19"), nil, "from")
	e.invalid(http.MethodGet, path("to=soon"), nil, "to")
	e.invalid(http.MethodGet, path("from=2026-10-22T00:00:00Z&to=2026-10-21T00:00:00Z"), nil, "from")
	e.invalid(http.MethodGet, path("limit=0"), nil, "limit")
	e.invalid(http.MethodGet, path("limit=201"), nil, "limit")
	e.invalid(http.MethodGet, path("limit=ten"), nil, "limit")
	e.invalid(http.MethodGet, path("cursor=%21%21%21"), nil, "cursor")
	e.invalid(http.MethodGet, path("cursor=eHl6"), nil, "cursor")
	e.invalid(http.MethodGet, path("cursor=bm90dGltZXw1"), nil, "cursor")
	e.invalid(http.MethodGet, path("from=bad&to=worse&limit=0"), nil, "from", "to", "limit")
	e.fail(http.StatusNotFound, "not_found", http.MethodGet, tankPath(9999, "/results"), nil)
}

func TestPatchResult(t *testing.T) {
	e := newEnv(t)
	e.setUnitFahrenheit()
	tank, temp := e.addTank("Patch"), e.preset("temperature")
	created := e.addResults(tank, "2026-10-24T09:00:00Z", val(temp, 77))
	id := created[0].ID

	e.now = e.now.Add(time.Hour)
	var got ResultView
	e.must(http.StatusOK, http.MethodPatch, resultPath(id), map[string]any{"value": 82.4, "note": " retested ", "measured_at": "2026-10-23T09:00:00Z"}).decode(&got)
	if got.Value != 82.4 || got.Note != "retested" || got.MeasuredAt != "2026-10-23T09:00:00Z" || got.UpdatedAt != "2026-10-24T13:00:00Z" ||
		got.CreatedAt != "2026-10-24T12:00:00Z" || got.Unit != "f" || got.ID != id || got.TankID != tank || got.ParameterID != temp || got.InRange {
		t.Errorf("patched = %+v", got)
	}
	near(t, "stored °C", e.storedValues()[0], 28, 1e-9)

	// Fields left out keep their value.
	e.must(http.StatusOK, http.MethodPatch, resultPath(id), map[string]any{"note": ""}).decode(&got)
	if got.Value != 82.4 || got.Note != "" || got.MeasuredAt != "2026-10-23T09:00:00Z" {
		t.Errorf("note-only patch = %+v", got)
	}
	e.must(http.StatusOK, http.MethodPatch, resultPath(id), map[string]any{})

	e.invalid(http.MethodPatch, resultPath(id), map[string]any{"value": -9999}, "value")
	e.invalid(http.MethodPatch, resultPath(id), map[string]any{"measured_at": "tomorrow"}, "measured_at")
	e.invalid(http.MethodPatch, resultPath(id), map[string]any{"measured_at": "2027-01-01T00:00:00Z"}, "measured_at")
	e.invalid(http.MethodPatch, resultPath(id), map[string]any{"note": repeat("n", 2001)}, "note")
	e.invalid(http.MethodPatch, resultPath(id), map[string]any{"parameter_id": 3}, "parameter_id")
	e.invalid(http.MethodPatch, resultPath(id), map[string]any{"value": -9999, "note": repeat("n", 2001)}, "value", "note")
	near(t, "stored °C after rejected patches", e.storedValues()[0], 28, 1e-9)
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, resultPath(9999), map[string]any{"note": "x"})
	e.fail(http.StatusNotFound, "not_found", http.MethodPatch, "/api/v1/results/abc", map[string]any{"note": "x"})
}

func TestDeletesRefuseNonJSONBodies(t *testing.T) {
	e := newEnv(t)
	tank, ph := e.addTank("Delete"), e.preset("ph")
	result := e.addResults(tank, "2026-10-24T09:00:00Z", val(ph, 8))[0]
	custom := e.customParameter(map[string]any{"name": "Iodine"})

	for _, target := range []string{resultPath(result.ID), paramPath(custom, "")} {
		r := e.doRaw(http.MethodDelete, target, "text/plain", "please")
		var b testutil.ErrorBody
		_ = json.Unmarshal(r.Body.Bytes(), &b)
		if r.Code != http.StatusUnsupportedMediaType || b.Error.Code != "unsupported_media_type" {
			t.Errorf("DELETE %s with a text body: status=%d code=%q", target, r.Code, b.Error.Code)
		}
	}
	if n := testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM test_results`) + testutil.Count(e.t, e.db, `SELECT COUNT(*) FROM parameter_definitions WHERE id = ?`, custom); n != 2 {
		t.Errorf("a refused delete removed something (%d rows left, want 2)", n)
	}
	if r := e.doRaw(http.MethodDelete, resultPath(result.ID), "application/json", ""); r.Code != http.StatusNoContent {
		t.Errorf("DELETE with a JSON content type: status=%d", r.Code)
	}
}

func TestDeleteResult(t *testing.T) {
	e := newEnv(t)
	tank, ph := e.addTank("Delete"), e.preset("ph")
	first := e.addResults(tank, "2026-10-24T09:00:00Z", val(ph, 8))[0]
	second := e.addResults(tank, "2026-10-24T10:00:00Z", val(ph, 8.2))[0]

	e.must(http.StatusNoContent, http.MethodDelete, resultPath(first.ID), nil)
	var page ResultPage
	e.must(http.StatusOK, http.MethodGet, tankPath(tank, "/results"), nil).decode(&page)
	if len(page.Results) != 1 || page.Results[0].ID != second.ID {
		t.Errorf("remaining = %+v", page.Results)
	}
	e.fail(http.StatusNotFound, "not_found", http.MethodDelete, resultPath(first.ID), nil)
	e.fail(http.StatusNotFound, "not_found", http.MethodDelete, "/api/v1/results/0", nil)

	// The deleted reading no longer counts as the latest.
	e.must(http.StatusNoContent, http.MethodDelete, resultPath(second.ID), nil)
	if got := e.tankParams(tank).find(t, ph); got.Latest != nil {
		t.Errorf("latest after deleting every result = %+v", got.Latest)
	}
}

func (e *env) setUnitFahrenheit() {
	e.t.Helper()
	e.setUnit("temperature", "f")
}
