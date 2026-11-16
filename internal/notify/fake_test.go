package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// fakeCall is one send the fake channel kind received.
type fakeCall struct {
	Config    string
	Msg       Message
	Remaining time.Duration // time left before the send's deadline, 0 when it had none
	UserAgent string
	HasHTTP   bool
}

// fakeState records what the fake kind was asked to send and decides how each send ends.
var fakeState struct {
	mu      sync.Mutex
	calls   []fakeCall
	outcome func(ctx context.Context, call fakeCall) error
}

func init() {
	Register(Kind{Name: "fake", Secrets: []string{"token", "url"}, Build: fakeBuild})
}

// fakeConfig is the config of the fake kind: a required secret token, an optional secret url and an optional note.
type fakeConfig struct {
	Token string `json:"token"`
	Note  string `json:"note"`
}

func fakeBuild(config json.RawMessage, env Env) (Notifier, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(config, &raw); err != nil {
		return nil, httpx.Reject("invalid config", "config", httpx.CodeInvalid)
	}
	f := httpx.FieldErrors{Message: "invalid config"}
	for name := range raw {
		if name != "token" && name != "note" && name != "url" {
			f.Add("config."+name, httpx.CodeInvalid)
		}
	}
	var cfg fakeConfig
	_ = json.Unmarshal(config, &cfg)
	if cfg.Token == "" {
		f.Add("config.token", httpx.CodeRequired)
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	return fakeNotifier{config: string(config), env: env}, nil
}

type fakeNotifier struct {
	config string
	env    Env
}

func (n fakeNotifier) Send(ctx context.Context, msg Message) error {
	call := fakeCall{Config: n.config, Msg: msg, UserAgent: n.env.UserAgent, HasHTTP: n.env.HTTP != nil}
	if deadline, ok := ctx.Deadline(); ok {
		call.Remaining = time.Until(deadline)
	}
	fakeState.mu.Lock()
	fakeState.calls = append(fakeState.calls, call)
	outcome := fakeState.outcome
	fakeState.mu.Unlock()
	if outcome == nil {
		return nil
	}
	return outcome(ctx, call)
}

// fakeReset clears the recorded sends and makes every send succeed, now and when the test ends.
func fakeReset(t *testing.T) {
	t.Helper()
	reset := func() {
		fakeState.mu.Lock()
		fakeState.calls, fakeState.outcome = nil, nil
		fakeState.mu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

// fakeOutcome sets how the fake kind answers from now on.
func fakeOutcome(fn func(ctx context.Context, call fakeCall) error) {
	fakeState.mu.Lock()
	fakeState.outcome = fn
	fakeState.mu.Unlock()
}

func fakeCalls() []fakeCall {
	fakeState.mu.Lock()
	defer fakeState.mu.Unlock()
	return append([]fakeCall(nil), fakeState.calls...)
}

const coreSecret = "s3cret-token"

var coreStart = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// coreEnv is the notification service and its routes on a real migrated database; the service's clock is coreEnv.clock.
type coreEnv struct {
	t        *testing.T
	db       *db.DB
	settings *settings.Store
	svc      *Service
	mux      http.Handler
	clock    time.Time
}

func newCoreEnv(t *testing.T) *coreEnv {
	t.Helper()
	return newCoreEnvAt(t, "https://reef.example.org")
}

func newCoreEnvAt(t *testing.T, publicURL string) *coreEnv {
	t.Helper()
	fakeReset(t)
	database := testutil.OpenDB(t)
	store := settings.NewStore(database)
	deps := Deps{DB: database, Settings: store, Logger: testutil.Logger(t), PublicURL: publicURL}
	e := &coreEnv{t: t, db: database, settings: store, svc: NewService(deps), clock: coreStart}
	e.svc.now = func() time.Time { return e.clock }
	mux := http.NewServeMux()
	Routes(mux, deps)
	e.mux = mux
	return e
}

// addUser inserts a user created at the given time.
func (e *coreEnv) addUser(id int64, role string, createdAt time.Time) {
	e.t.Helper()
	at := db.FormatTime(createdAt)
	testutil.Exec(e.t, e.db, `INSERT INTO users (id, username, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, id, "user"+string(rune('a'+id)), role, at, at)
}

func (e *coreEnv) do(userID int64, role, method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	return testutil.DoJSON(e.t, e.mux, method, path, body, testutil.AsUser(userID, role))
}

// admin sends the request as the admin user 1.
func (e *coreEnv) admin(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(1, httpx.RoleAdmin, method, path, body)
}

// addChannel creates a fake channel through the API and returns it.
func (e *coreEnv) addChannel(name string, events ...string) Channel {
	e.t.Helper()
	body := map[string]any{"name": name, "kind": "fake", "config": map[string]any{"token": coreSecret, "note": "n"}}
	if len(events) > 0 {
		body["events"] = events
	}
	rec := e.admin(http.MethodPost, "/api/v1/notification-channels", body)
	testutil.ExpectStatus(e.t, rec, http.StatusCreated)
	return testutil.Decode[Channel](e.t, rec)
}

// coreEvent is a maintenance_due event whose occurrence tells it apart from the others.
func coreEvent(occurrence string) Event {
	return Event{
		Type: "maintenance_due", Severity: "warning", Title: "Water change is due", Body: "Main tank",
		Link: "/tanks/1/maintenance", SourceKind: "maintenance_task", SourceID: 7, Occurrence: occurrence,
	}
}

func (e *coreEnv) raise(ev Event) bool {
	e.t.Helper()
	raised, err := e.svc.Raise(e.t.Context(), ev)
	if err != nil {
		e.t.Fatal(err)
	}
	return raised
}

// coreDelivery is a stored delivery row.
type coreDelivery struct {
	ID             int64
	NotificationID int64
	ChannelID      int64
	Status         string
	Attempts       int64
	LastError      string
	NextAttemptAt  *string
	LastAttemptAt  *string
	SentAt         *string
}

func (e *coreEnv) deliveries() []coreDelivery {
	e.t.Helper()
	rows, err := e.db.R.QueryContext(e.t.Context(), `SELECT id, notification_id, channel_id, status, attempts, last_error, next_attempt_at, last_attempt_at, sent_at FROM notification_deliveries ORDER BY id`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []coreDelivery
	for rows.Next() {
		var d coreDelivery
		if err := rows.Scan(&d.ID, &d.NotificationID, &d.ChannelID, &d.Status, &d.Attempts, &d.LastError, &d.NextAttemptAt, &d.LastAttemptAt, &d.SentAt); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		e.t.Fatal(err)
	}
	return out
}

// dispatch runs the dispatcher at now, with the service's clock at now as well.
func (e *coreEnv) dispatch(now time.Time) {
	e.t.Helper()
	e.clock = now
	if err := e.svc.Dispatch(e.t.Context(), now); err != nil {
		e.t.Fatal(err)
	}
}

// coreTime is t as the database stores it.
func coreTime(t time.Time) string { return db.FormatTime(t) }

// coreID is an id as it appears in a URL path.
func coreID(id int64) string { return strconv.FormatInt(id, 10) }
