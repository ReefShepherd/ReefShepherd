package app

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

type tankBody struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Volume     *float64 `json:"volume"`
	VolumeUnit string   `json:"volume_unit"`
	ArchivedAt *string  `json:"archived_at"`
}

func TestTankEndpointsRequireSignIn(t *testing.T) {
	a, admin := adminAndApp(t)
	created := admin.do(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "Reef"})
	testutil.ExpectStatus(t, created, http.StatusCreated)
	var tank tankBody
	testutil.DecodeInto(t, created, &tank)

	anon := newClient(t, a)
	base := "/api/v1/tanks/" + strconv.FormatInt(tank.ID, 10)
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/tanks", nil},
		{http.MethodPost, "/api/v1/tanks", map[string]any{"name": "Sneaky"}},
		{http.MethodGet, base, nil},
		{http.MethodPatch, base, map[string]any{"name": "Sneaky"}},
		{http.MethodDelete, base + "?confirm=Reef", nil},
		{http.MethodPost, base + "/archive", nil},
		{http.MethodPost, base + "/unarchive", nil},
	} {
		testutil.ExpectError(t, anon.do(c.method, c.path, c.body), http.StatusUnauthorized, "unauthorized")
	}
	if n := testutil.Count(t, a.db, `SELECT COUNT(*) FROM tanks WHERE name = 'Reef' AND archived_at IS NULL`); n != 1 {
		t.Errorf("an anonymous request changed the tank")
	}
}

func TestMembersShareTanksButCannotDeleteThem(t *testing.T) {
	a, admin := adminAndApp(t)
	addUser(t, admin, "member1", "member")
	member := signedInAs(t, a, "member1")

	var tank tankBody
	rec := member.do(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "Shared", "volume": 100, "presets": []string{"ph"}})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	testutil.DecodeInto(t, rec, &tank)

	// Every signed-in user sees and edits every tank.
	var seen []tankBody
	rec = admin.do(http.MethodGet, "/api/v1/tanks", nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	testutil.DecodeInto(t, rec, &seen)
	if len(seen) != 1 || seen[0].ID != tank.ID || seen[0].Volume == nil || *seen[0].Volume != 100 || seen[0].VolumeUnit != "l" {
		t.Errorf("admin sees %+v", seen)
	}
	base := "/api/v1/tanks/" + strconv.FormatInt(tank.ID, 10)
	testutil.ExpectStatus(t, admin.do(http.MethodPatch, base, map[string]any{"name": "Renamed"}), http.StatusOK)
	testutil.ExpectStatus(t, member.do(http.MethodPost, base+"/archive", nil), http.StatusOK)
	testutil.ExpectStatus(t, member.do(http.MethodPost, base+"/unarchive", nil), http.StatusOK)

	testutil.ExpectError(t, member.do(http.MethodDelete, base+"?confirm=Renamed", nil), http.StatusForbidden, "forbidden")
	if n := testutil.Count(t, a.db, `SELECT COUNT(*) FROM tanks`); n != 1 {
		t.Fatal("a member deleted a tank")
	}
	testutil.ExpectError(t, admin.do(http.MethodDelete, base+"?confirm=Shared", nil), http.StatusUnprocessableEntity, "validation")
	testutil.ExpectStatus(t, admin.do(http.MethodDelete, base+"?confirm=Renamed", nil), http.StatusNoContent)
	if n := testutil.Count(t, a.db, `SELECT COUNT(*) FROM tanks`) + testutil.Count(t, a.db, `SELECT COUNT(*) FROM tank_parameters`); n != 0 {
		t.Errorf("%d tank rows survived the delete", n)
	}
}
