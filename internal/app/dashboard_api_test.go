package app

import (
	"net/http"
	"strconv"
	"testing"
)

type dashboardCardBody struct {
	ID          int64  `json:"id"`
	CardType    string `json:"card_type"`
	ParameterID *int64 `json:"parameter_id"`
	Position    int64  `json:"position"`
	Size        string `json:"size"`
}

type dashboardBody struct {
	Cards []dashboardCardBody `json:"cards"`
}

func TestDashboardEndpointsRequireSignIn(t *testing.T) {
	a, admin := adminAndApp(t)
	tank := mustCreateTank(t, admin)
	anon := newClient(t, a)
	base := "/api/v1/tanks/" + strconv.FormatInt(tank.ID, 10) + "/dashboard"
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, base, nil},
		{http.MethodPut, base, map[string]any{"cards": []map[string]any{{"card_type": "tests_due"}}}},
		{http.MethodPost, base + "/reset", nil},
	} {
		expect(t, anon.do(c.method, c.path, c.body), http.StatusUnauthorized, "unauthorized")
	}
	if n := a.count(t, `SELECT COUNT(*) FROM dashboard_cards`); n != 0 {
		t.Errorf("an anonymous request wrote %d cards", n)
	}
}

func mustCreateTank(t *testing.T, c *apiClient) tankBody {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "Reef"})
	expectStatus(t, rec, http.StatusCreated)
	var tank tankBody
	decodeBody(t, rec, &tank)
	return tank
}

func TestDashboardsBelongToTheSignedInUser(t *testing.T) {
	a, admin := adminAndApp(t)
	addUser(t, admin, "member1", "member")
	member := signedInAs(t, a, "member1")
	tank := mustCreateTank(t, admin)
	base := "/api/v1/tanks/" + strconv.FormatInt(tank.ID, 10) + "/dashboard"
	a.exec(t, `INSERT INTO tank_parameters (tank_id, parameter_id, enabled, sort_order)
		SELECT ?, id, 1, sort_order FROM parameter_definitions WHERE preset_key IN ('ph', 'calcium')`, tank.ID)

	read := func(c *apiClient) dashboardBody {
		rec := c.do(http.MethodGet, base, nil)
		expectStatus(t, rec, http.StatusOK)
		var d dashboardBody
		decodeBody(t, rec, &d)
		return d
	}
	if d := read(admin); len(d.Cards) != 3 || d.Cards[0].CardType != "tests_due" || d.Cards[1].CardType != "parameter" || d.Cards[1].ParameterID == nil {
		t.Fatalf("default dashboard = %+v", d)
	}

	// The admin hides everything but the clock; the member still has the full default.
	expectStatus(t, admin.do(http.MethodPut, base, map[string]any{"cards": []map[string]any{{"card_type": "tests_due", "size": "wide"}}}), http.StatusOK)
	if d := read(admin); len(d.Cards) != 1 || d.Cards[0].Size != "wide" {
		t.Errorf("admin dashboard = %+v", d)
	}
	if d := read(member); len(d.Cards) != 3 || d.Cards[0].Size != "normal" {
		t.Errorf("member dashboard = %+v", d)
	}
	expectStatus(t, member.do(http.MethodPost, base+"/reset", nil), http.StatusOK)
	if d := read(admin); len(d.Cards) != 1 || d.Cards[0].Size != "wide" {
		t.Errorf("a member's reset changed the admin's dashboard: %+v", d)
	}

	// Form posts and a missing tank are refused through the whole stack.
	expect(t, admin.raw(http.MethodPost, base+"/reset", "application/x-www-form-urlencoded", "a=b", nil), http.StatusUnsupportedMediaType, "unsupported_media_type")
	expect(t, admin.do(http.MethodGet, "/api/v1/tanks/9999/dashboard", nil), http.StatusNotFound, "not_found")
}
