package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

const channelsPath = "/api/v1/notification-channels"

// coreStoredConfig reads the config as it is stored, which is the only place a secret may appear.
func coreStoredConfig(t *testing.T, e *coreEnv, id int64) map[string]any {
	t.Helper()
	var raw string
	if err := e.db.R.QueryRowContext(t.Context(), `SELECT config_json FROM notification_channels WHERE id = ?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func coreNoSecret(t *testing.T, what string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if strings.Contains(rec.Body.String(), coreSecret) {
		t.Errorf("%s shows the secret: %s", what, rec.Body)
	}
}

func TestChannelRoutesNeedAnAdminAndInboxRoutesOnlyASignedInUser(t *testing.T) {
	e := newCoreEnv(t)
	e.addUser(2, "member", coreStart.Add(-time.Hour))
	ch := e.addChannel("main")
	channelRoutes := []struct{ method, path string }{
		{http.MethodGet, channelsPath},
		{http.MethodPost, channelsPath},
		{http.MethodPatch, channelsPath + "/" + coreID(ch.ID)},
		{http.MethodDelete, channelsPath + "/" + coreID(ch.ID)},
		{http.MethodPost, channelsPath + "/" + coreID(ch.ID) + "/test"},
	}
	for _, r := range channelRoutes {
		var body any
		if r.method == http.MethodPost || r.method == http.MethodPatch {
			body = map[string]any{}
		}
		testutil.ExpectError(t, e.do(2, "member", r.method, r.path, body), http.StatusForbidden, "forbidden")
		testutil.ExpectError(t, testutil.DoJSON(t, e.mux, r.method, r.path, body), http.StatusUnauthorized, "unauthorized")
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_channels`); n != 1 {
		t.Errorf("channels after the refused requests = %d, want 1", n)
	}

	testutil.ExpectStatus(t, e.do(2, "member", http.MethodGet, "/api/v1/notifications", nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(2, "member", http.MethodGet, "/api/v1/notifications/unread-count", nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(2, "member", http.MethodPost, "/api/v1/notifications/read", map[string]any{"all": true}), http.StatusNoContent)
	for _, path := range []string{"/api/v1/notifications", "/api/v1/notifications/unread-count"} {
		testutil.ExpectError(t, testutil.DoJSON(t, e.mux, http.MethodGet, path, nil), http.StatusUnauthorized, "unauthorized")
	}
	testutil.ExpectError(t, testutil.DoJSON(t, e.mux, http.MethodPost, "/api/v1/notifications/read", map[string]any{"all": true}), http.StatusUnauthorized, "unauthorized")
}

func TestCreateChannelMasksSecretsOnEveryRead(t *testing.T) {
	e := newCoreEnv(t)

	rec := e.admin(http.MethodPost, channelsPath, map[string]any{"name": "  Phone  ", "kind": "fake", "config": map[string]any{"token": coreSecret, "note": "visible"}})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	coreNoSecret(t, "create response", rec)
	ch := testutil.Decode[struct {
		Channel
		Config map[string]any `json:"config"`
	}](t, rec)
	if ch.Name != "Phone" || ch.Kind != "fake" || !ch.Enabled || ch.Config["token"] != "********" || ch.Config["note"] != "visible" {
		t.Errorf("created channel = %+v", ch)
	}
	if !slices.Equal(ch.Events, eventTypes) {
		t.Errorf("events = %v, want all of %v", ch.Events, eventTypes)
	}
	if ch.LastSentAt != nil || ch.LastError != "" || ch.LastErrorAt != nil || ch.CreatedAt == "" {
		t.Errorf("fresh channel health = %+v", ch)
	}

	list := e.admin(http.MethodGet, channelsPath, nil)
	testutil.ExpectStatus(t, list, http.StatusOK)
	coreNoSecret(t, "list", list)
	if !strings.Contains(list.Body.String(), `"token":"********"`) {
		t.Errorf("list does not show the mask: %s", list.Body)
	}
	if stored := coreStoredConfig(t, e, ch.ID); stored["token"] != coreSecret {
		t.Errorf("stored config = %v, want the real secret kept", stored)
	}

	patch := e.admin(http.MethodPatch, channelsPath+"/"+coreID(ch.ID), map[string]any{"name": "Phone 2"})
	coreNoSecret(t, "patch response", patch)
}

func TestMaskingCoversEveryFieldOfAKindThisBuildDoesNotKnow(t *testing.T) {
	e := newCoreEnv(t)
	testutil.Insert(t, e.db, `INSERT INTO notification_channels (name, kind, config_json) VALUES ('legacy', 'nosuch', '{"url":"https://x.example/hook","other":"v","empty":"","n":5}')`)

	items := testutil.DecodeItems[struct {
		Config map[string]any `json:"config"`
	}](t, e.admin(http.MethodGet, channelsPath, nil))
	if len(items) != 1 {
		t.Fatalf("channels = %d", len(items))
	}
	c := items[0].Config
	if c["url"] != "********" || c["other"] != "********" || c["n"] != "********" || c["empty"] != "" {
		t.Errorf("config of an unknown kind = %v, want every set field masked and empty ones left", c)
	}
}

func TestCreateChannelRejectsBadInputWithTheFieldKeys(t *testing.T) {
	e := newCoreEnv(t)
	good := func() map[string]any {
		return map[string]any{"name": "Phone", "kind": "fake", "config": map[string]any{"token": "t"}}
	}
	with := func(key string, value any) map[string]any {
		b := good()
		if value == nil {
			delete(b, key)
		} else {
			b[key] = value
		}
		return b
	}
	cases := []struct {
		name   string
		body   map[string]any
		fields map[string]string
	}{
		{"no name", with("name", nil), map[string]string{"name": "required"}},
		{"blank name", with("name", "   "), map[string]string{"name": "required"}},
		{"long name", with("name", strings.Repeat("n", 101)), map[string]string{"name": "too_long"}},
		{"no kind", with("kind", nil), map[string]string{"kind": "required"}},
		{"unknown kind", with("kind", "carrier_pigeon"), map[string]string{"kind": "invalid"}},
		{"no config", with("config", nil), map[string]string{"config": "required"}},
		{"null config", with("config", json.RawMessage("null")), map[string]string{"config": "required"}},
		{"config not an object", with("config", "text"), map[string]string{"config": "invalid"}},
		{"config the kind refuses", with("config", map[string]any{}), map[string]string{"config.token": "required"}},
		{"config with an unknown field", with("config", map[string]any{"token": "t", "extra": 1}), map[string]string{"config.extra": "invalid"}},
		{"no events", with("events", []string{}), map[string]string{"events": "required"}},
		{"unknown event", with("events", []string{"maintenance_due", "bogus"}), map[string]string{"events": "invalid"}},
		{"several problems at once", map[string]any{"kind": "fake", "config": map[string]any{}, "events": []string{}}, map[string]string{"name": "required", "config.token": "required", "events": "required"}},
		{"unknown request field", with("colour", "red"), map[string]string{"colour": "invalid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.admin(http.MethodPost, channelsPath, tc.body)
			var names []string
			for f := range tc.fields {
				names = append(names, f)
			}
			body := testutil.ExpectError(t, rec, http.StatusUnprocessableEntity, "validation", names...)
			for field, code := range tc.fields {
				if body.Error.Fields[field] != code {
					t.Errorf("field %s = %q, want %q", field, body.Error.Fields[field], code)
				}
			}
		})
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_channels`); n != 0 {
		t.Errorf("channels stored after the refused requests = %d", n)
	}
}

func TestCreateChannelStoresEventsOnceInListingOrderAndTheEnabledFlag(t *testing.T) {
	e := newCoreEnv(t)
	body := map[string]any{
		"name": "Phone", "kind": "fake", "config": map[string]any{"token": "t"}, "enabled": false,
		"events": []string{"backup_failed", "tests_due", "tests_due"},
	}
	rec := e.admin(http.MethodPost, channelsPath, body)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	ch := testutil.Decode[Channel](t, rec)
	if ch.Enabled || !slices.Equal(ch.Events, []string{"tests_due", "backup_failed"}) {
		t.Errorf("channel = enabled %v, events %v", ch.Enabled, ch.Events)
	}
}

func TestPatchChannelReplacesTheConfigButKeepsSecretsSentAsTheMask(t *testing.T) {
	e := newCoreEnv(t)
	ch := e.addChannel("main")
	path := channelsPath + "/" + coreID(ch.ID)

	// A masked secret keeps the stored value while the rest of the config is replaced.
	rec := e.admin(http.MethodPatch, path, map[string]any{"config": map[string]any{"token": "********", "note": "changed"}})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	coreNoSecret(t, "patch response", rec)
	if stored := coreStoredConfig(t, e, ch.ID); stored["token"] != coreSecret || stored["note"] != "changed" {
		t.Errorf("stored config = %v, want the old secret and the new note", stored)
	}

	// A new secret replaces it, and a field left out is gone because the config is replaced as a whole.
	rec = e.admin(http.MethodPatch, path, map[string]any{"config": map[string]any{"token": "fresh-token"}})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if stored := coreStoredConfig(t, e, ch.ID); stored["token"] != "fresh-token" || len(stored) != 1 {
		t.Errorf("stored config = %v, want only the new token", stored)
	}

	// A config the kind refuses leaves the stored one alone.
	rec = e.admin(http.MethodPatch, path, map[string]any{"config": map[string]any{"token": "x", "extra": 1}})
	testutil.ExpectError(t, rec, http.StatusUnprocessableEntity, "validation", "config.extra")
	if stored := coreStoredConfig(t, e, ch.ID); stored["token"] != "fresh-token" {
		t.Errorf("stored config after a refused patch = %v", stored)
	}
}

func TestPatchChannelDropsAMaskedSecretThereIsNothingStoredFor(t *testing.T) {
	e := newCoreEnv(t)
	id := testutil.Insert(t, e.db, `INSERT INTO notification_channels (name, kind, config_json) VALUES ('bare', 'fake', '{"note":"n"}')`)

	rec := e.admin(http.MethodPatch, channelsPath+"/"+coreID(id), map[string]any{"config": map[string]any{"token": "********"}})

	testutil.ExpectError(t, rec, http.StatusUnprocessableEntity, "validation", "config.token")
}

func TestPatchChannelChangesOnlyWhatIsSent(t *testing.T) {
	e := newCoreEnv(t)
	ch := e.addChannel("main")
	path := channelsPath + "/" + coreID(ch.ID)

	rec := e.admin(http.MethodPatch, path, map[string]any{"name": "Renamed", "enabled": false, "events": []string{"out_of_range", "tests_due"}})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	got := testutil.Decode[Channel](t, rec)
	if got.Name != "Renamed" || got.Enabled || !slices.Equal(got.Events, []string{"tests_due", "out_of_range"}) {
		t.Errorf("channel = %+v", got)
	}
	if stored := coreStoredConfig(t, e, ch.ID); stored["token"] != coreSecret {
		t.Errorf("config changed without being sent: %v", stored)
	}
	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_subscriptions WHERE channel_id = ?`, ch.ID); n != 2 {
		t.Errorf("subscriptions = %d, want the two sent", n)
	}

	rec = e.admin(http.MethodPatch, path, map[string]any{"enabled": true})
	if got := testutil.Decode[Channel](t, rec); !got.Enabled || got.Name != "Renamed" {
		t.Errorf("channel after enabling = %+v", got)
	}
}

func TestPatchChannelRejectsNullsAndBadValues(t *testing.T) {
	e := newCoreEnv(t)
	ch := e.addChannel("main")
	path := channelsPath + "/" + coreID(ch.ID)
	cases := []struct {
		name   string
		body   string
		fields []string
	}{
		{"null name", `{"name":null}`, []string{"name"}},
		{"null config", `{"config":null}`, []string{"config"}},
		{"null events", `{"events":null}`, []string{"events"}},
		{"null enabled", `{"enabled":null}`, []string{"enabled"}},
		{"blank name", `{"name":" "}`, []string{"name"}},
		{"no events", `{"events":[]}`, []string{"events"}},
		{"unknown event", `{"events":["bogus"]}`, []string{"events"}},
		{"config not an object", `{"config":[1]}`, []string{"config"}},
		{"kind cannot change", `{"kind":"fake"}`, []string{"kind"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := testutil.DoRaw(t, e.mux, http.MethodPatch, path, "application/json", tc.body, testutil.AsUser(1, "admin"))
			testutil.ExpectError(t, rec, http.StatusUnprocessableEntity, "validation", tc.fields...)
		})
	}
}

func TestPatchChannelOfAKindThisBuildDoesNotKnowRefusesANewConfig(t *testing.T) {
	e := newCoreEnv(t)
	id := testutil.Insert(t, e.db, `INSERT INTO notification_channels (name, kind, config_json) VALUES ('legacy', 'nosuch', '{"url":"u"}')`)

	rec := e.admin(http.MethodPatch, channelsPath+"/"+coreID(id), map[string]any{"config": map[string]any{"url": "v"}})

	testutil.ExpectError(t, rec, http.StatusUnprocessableEntity, "validation", "config")
}

func TestChannelRoutesAnswerNotFoundForAMissingOrMalformedID(t *testing.T) {
	e := newCoreEnv(t)
	for _, path := range []string{channelsPath + "/99", channelsPath + "/abc", channelsPath + "/0"} {
		testutil.ExpectError(t, e.admin(http.MethodPatch, path, map[string]any{"name": "x"}), http.StatusNotFound, "not_found")
		testutil.ExpectError(t, e.admin(http.MethodDelete, path, nil), http.StatusNotFound, "not_found")
		testutil.ExpectError(t, e.admin(http.MethodPost, path+"/test", nil), http.StatusNotFound, "not_found")
	}
}

func TestDeleteChannelRemovesItsSubscriptionsAndDeliveries(t *testing.T) {
	e := newCoreEnv(t)
	gone := e.addChannel("gone")
	kept := e.addChannel("kept")
	e.raise(coreEvent("a"))

	testutil.ExpectStatus(t, e.admin(http.MethodDelete, channelsPath+"/"+coreID(gone.ID), nil), http.StatusNoContent)

	if n := testutil.Count(t, e.db, `SELECT COUNT(*) FROM notification_subscriptions WHERE channel_id = ?`, gone.ID); n != 0 {
		t.Errorf("subscriptions left = %d", n)
	}
	ds := e.deliveries()
	if len(ds) != 1 || ds[0].ChannelID != kept.ID {
		t.Errorf("deliveries left = %+v, want only the other channel's", ds)
	}
	testutil.ExpectError(t, e.admin(http.MethodDelete, channelsPath+"/"+coreID(gone.ID), nil), http.StatusNotFound, "not_found")
}

func TestTestSendAnswersWithTheOutcomeInsteadOfAnHTTPError(t *testing.T) {
	e := newCoreEnv(t)
	if err := e.settings.SetInstanceName(t.Context(), "Garage Reef"); err != nil {
		t.Fatal(err)
	}
	ch := e.addChannel("main")
	path := channelsPath + "/" + coreID(ch.ID) + "/test"

	rec := e.admin(http.MethodPost, path, nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if body := strings.TrimSpace(rec.Body.String()); body != `{"sent":true,"error":""}` {
		t.Errorf("body = %s", body)
	}
	calls := fakeCalls()
	if len(calls) != 1 {
		t.Fatalf("sends = %d", len(calls))
	}
	m := calls[0].Msg
	if m.EventType != "test" || m.Severity != "info" || m.Title != "Test notification from Garage Reef" ||
		m.Body != "If you can read this, the channel is set up correctly." || m.Link != "https://reef.example.org/settings/notifications" ||
		m.Instance != "Garage Reef" || m.TankID != 0 || m.TankName != "" || time.Since(m.OccurredAt) > time.Minute {
		t.Errorf("test message = %+v", m)
	}
	if calls[0].Remaining <= 9*time.Second || calls[0].Remaining > 10*time.Second {
		t.Errorf("time left to send = %v", calls[0].Remaining)
	}

	fakeOutcome(func(context.Context, fakeCall) error { return errors.New("provider said no") })
	rec = e.admin(http.MethodPost, path, nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if body := strings.TrimSpace(rec.Body.String()); body != `{"sent":false,"error":"provider said no"}` {
		t.Errorf("failed body = %s", body)
	}

	long := strings.Repeat("e", 900)
	fakeOutcome(func(context.Context, fakeCall) error { return errors.New(long) })
	got := testutil.Decode[TestResult](t, e.admin(http.MethodPost, path, nil))
	if got.Sent || got.Error != long[:500] {
		t.Errorf("long error came back as sent=%v with %d characters", got.Sent, len(got.Error))
	}

	// The test send leaves no delivery behind.
	if n := len(e.deliveries()); n != 0 {
		t.Errorf("deliveries after test sends = %d", n)
	}
}

func TestTestSendWorksOnADisabledChannelAndReportsAnUnusableOne(t *testing.T) {
	e := newCoreEnv(t)
	ch := e.addChannel("main")
	testutil.ExpectStatus(t, e.admin(http.MethodPatch, channelsPath+"/"+coreID(ch.ID), map[string]any{"enabled": false}), http.StatusOK)
	if got := testutil.Decode[TestResult](t, e.admin(http.MethodPost, channelsPath+"/"+coreID(ch.ID)+"/test", nil)); !got.Sent {
		t.Errorf("disabled channel test = %+v, want sent so it can be checked before enabling", got)
	}

	unknown := testutil.Insert(t, e.db, `INSERT INTO notification_channels (name, kind, config_json) VALUES ('legacy', 'nosuch', '{}')`)
	got := testutil.Decode[TestResult](t, e.admin(http.MethodPost, channelsPath+"/"+coreID(unknown)+"/test", nil))
	if got.Sent || !strings.Contains(got.Error, `unknown channel kind "nosuch"`) {
		t.Errorf("unknown kind test = %+v", got)
	}

	broken := testutil.Insert(t, e.db, `INSERT INTO notification_channels (name, kind, config_json) VALUES ('broken', 'fake', '{}')`)
	got = testutil.Decode[TestResult](t, e.admin(http.MethodPost, channelsPath+"/"+coreID(broken)+"/test", nil))
	if got.Sent || !strings.Contains(got.Error, "config.token") {
		t.Errorf("invalid config test = %+v", got)
	}
}

func TestChannelShowsWhenItLastSentAndWhatIsFailingNow(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.addChannel("second")
	listed := func() []Channel {
		t.Helper()
		return testutil.DecodeItems[Channel](t, e.admin(http.MethodGet, channelsPath, nil))
	}

	// Nothing attempted yet: no send, no error.
	if c := listed()[0]; c.LastSentAt != nil || c.LastError != "" || c.LastErrorAt != nil {
		t.Errorf("before any attempt: %+v", c)
	}

	e.raise(coreEvent("a"))
	coreFailing(errors.New("boom"))
	e.dispatch(coreStart)
	failing := listed()
	if c := failing[0]; c.LastSentAt != nil || c.LastError != "boom" || c.LastErrorAt == nil || *c.LastErrorAt != coreTime(coreStart) {
		t.Errorf("while failing: %+v", c)
	}
	if c := failing[1]; c.LastError != "boom" {
		t.Errorf("second channel did not fail its own delivery: %+v", c)
	}

	// The error time follows the latest failed attempt, not the time the notification was raised.
	retryAt := coreStart.Add(time.Minute)
	e.dispatch(retryAt)
	if c := listed()[0]; c.LastError != "boom" || c.LastErrorAt == nil || *c.LastErrorAt != coreTime(retryAt) {
		t.Errorf("after a second failure: %+v, want the error time %s", c, coreTime(retryAt))
	}

	fakeOutcome(nil)
	recoverAt := coreStart.Add(11 * time.Minute)
	e.dispatch(recoverAt)
	for _, c := range listed() {
		if c.LastError != "" || c.LastErrorAt != nil || c.LastSentAt == nil || *c.LastSentAt != coreTime(recoverAt) {
			t.Errorf("after recovering: %+v", c)
		}
	}
}
