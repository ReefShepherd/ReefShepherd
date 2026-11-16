package app

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/params"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// reefWithTemperatureRange creates a tank whose temperature must stay between 25 and 27 and returns the tank and parameter ids.
func reefWithTemperatureRange(t *testing.T, a *testApp, admin *apiClient) (tankID, parameterID int64) {
	t.Helper()
	rec := admin.do(http.MethodPost, "/api/v1/tanks", map[string]any{"name": "Reef", "presets": []string{"temperature"}})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	var tank tankBody
	testutil.DecodeInto(t, rec, &tank)
	row := a.db.R.QueryRowContext(t.Context(), `SELECT id FROM parameter_definitions WHERE preset_key = 'temperature'`)
	if err := row.Scan(&parameterID); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/tanks/" + strconv.FormatInt(tank.ID, 10) + "/parameters/" + strconv.FormatInt(parameterID, 10)
	testutil.ExpectStatus(t, admin.do(http.MethodPut, path, map[string]any{"enabled": true, "ideal_low": 25, "ideal_high": 27}), http.StatusOK)
	return tank.ID, parameterID
}

func saveResult(t *testing.T, admin *apiClient, tankID, parameterID int64, value float64, measuredAt time.Time) {
	t.Helper()
	body := map[string]any{"results": []map[string]any{{"parameter_id": parameterID, "value": value}}}
	if !measuredAt.IsZero() {
		body["measured_at"] = measuredAt.UTC().Format(time.RFC3339)
	}
	rec := admin.do(http.MethodPost, "/api/v1/tanks/"+strconv.FormatInt(tankID, 10)+"/results", body)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
}

func outOfRangeCount(t *testing.T, a *testApp) int {
	t.Helper()
	return testutil.Count(t, a.db, `SELECT COUNT(*) FROM notifications WHERE event_type = 'out_of_range'`)
}

func TestSavingAResultThroughTheAPIRaisesAnOutOfRangeNotification(t *testing.T) {
	a, admin := adminAndApp(t)
	tankID, parameterID := reefWithTemperatureRange(t, a, admin)

	// In range: nothing is raised.
	saveResult(t, admin, tankID, parameterID, 26, time.Time{})
	if n := outOfRangeCount(t, a); n != 0 {
		t.Fatalf("an in-range result raised %d notifications", n)
	}

	// Out of range and measured more than 24 hours ago, as when backfilling old tests: nothing is raised.
	saveResult(t, admin, tankID, parameterID, 22, time.Now().Add(-25*time.Hour))
	if n := outOfRangeCount(t, a); n != 0 {
		t.Fatalf("a result measured 25 hours ago raised %d notifications", n)
	}

	// Out of range and measured now: one notification, visible in the inbox.
	saveResult(t, admin, tankID, parameterID, 23, time.Time{})
	if n := outOfRangeCount(t, a); n != 1 {
		t.Fatalf("out-of-range notifications = %d, want 1", n)
	}
	rec := admin.do(http.MethodGet, "/api/v1/notifications", nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	items := testutil.DecodeItems[struct {
		EventType string `json:"event_type"`
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		Link      string `json:"link"`
		TankID    *int64 `json:"tank_id"`
	}](t, rec)
	if len(items) != 1 {
		t.Fatalf("inbox = %+v, want one item", items)
	}
	got := items[0]
	wantLink := "/tanks/" + strconv.FormatInt(tankID, 10) + "/charts/" + strconv.FormatInt(parameterID, 10)
	if got.EventType != "out_of_range" || got.Severity != "warning" || got.Title != "Temperature is out of range on Reef" ||
		got.Body != "Measured 23.0 °C, below the ideal range of 25.0 °C to 27.0 °C." || got.Link != wantLink || got.TankID == nil || *got.TankID != tankID {
		t.Errorf("inbox item = %+v", got)
	}
}

// The params service the reminders read from has no result hook, so storing a result through it can never call the reminders back.
func TestTheParamsServiceTheRemindersUseDoesNotCallThemBack(t *testing.T) {
	a, admin := adminAndApp(t)
	tankID, parameterID := reefWithTemperatureRange(t, a, admin)
	var userID int64
	if err := a.db.R.QueryRowContext(t.Context(), `SELECT id FROM users WHERE username = ?`, adminName).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	d := newRouteDeps(config.Config{}, testutil.Logger(t), a.db, a.store, nil)
	if d.RemindersDeps.Params.OnResultCreated != nil {
		t.Fatal("the reminders' params service has the result hook")
	}
	value := 22.0
	_, err := d.RemindersDeps.Params.CreateResults(t.Context(), tankID, userID, params.CreateResultsInput{
		Results: []params.ResultEntry{{ParameterID: parameterID, Value: &value}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := testutil.Count(t, a.db, `SELECT COUNT(*) FROM test_results`); n != 1 {
		t.Fatalf("results = %d, want the one just stored", n)
	}
	if n := testutil.Count(t, a.db, `SELECT COUNT(*) FROM notifications`); n != 0 {
		t.Errorf("storing a result through the hookless service raised %d notifications", n)
	}
}
