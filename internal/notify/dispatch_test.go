package notify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// coreThrottled is a send error that asks for a wait.
type coreThrottled struct{ wait time.Duration }

func (e coreThrottled) Error() string             { return "slow down" }
func (e coreThrottled) RetryAfter() time.Duration { return e.wait }

// coreGone is a send error for a destination that no longer exists.
type coreGone struct{}

func (coreGone) Error() string { return "webhook deleted" }
func (coreGone) Gone() bool    { return true }

// coreFailing makes every send fail with err.
func coreFailing(err error) {
	fakeOutcome(func(context.Context, fakeCall) error { return err })
}

func coreSingle(t *testing.T, e *coreEnv) coreDelivery {
	t.Helper()
	ds := e.deliveries()
	if len(ds) != 1 {
		t.Fatalf("deliveries = %+v, want exactly one", ds)
	}
	return ds[0]
}

func TestDispatchSendsTheMessageAndMarksTheDeliverySent(t *testing.T) {
	e := newCoreEnv(t)
	if err := e.settings.SetInstanceName(t.Context(), "Garage Reef"); err != nil {
		t.Fatal(err)
	}
	tank := testutil.Insert(t, e.db, `INSERT INTO tanks (name) VALUES ('Display')`)
	e.addChannel("main")
	ev := coreEvent("a")
	ev.TankID = &tank
	e.raise(ev)

	later := coreStart.Add(30 * time.Second)
	e.dispatch(later)

	calls := fakeCalls()
	if len(calls) != 1 {
		t.Fatalf("sends = %d, want 1", len(calls))
	}
	c := calls[0]
	want := Message{
		EventType: "maintenance_due", Severity: "warning", Title: "Water change is due", Body: "Main tank",
		Link: "https://reef.example.org/tanks/1/maintenance", TankID: tank, TankName: "Display",
		OccurredAt: coreStart, Instance: "Garage Reef",
	}
	if c.Msg != want {
		t.Errorf("message = %+v, want %+v", c.Msg, want)
	}
	if !strings.Contains(c.Config, coreSecret) {
		t.Errorf("the channel was built from %q, not its stored config", c.Config)
	}
	if !strings.HasPrefix(c.UserAgent, "ReefShepherd/") || !c.HasHTTP {
		t.Errorf("env = user agent %q, http client %v", c.UserAgent, c.HasHTTP)
	}
	if c.Remaining <= 9*time.Second || c.Remaining > 10*time.Second {
		t.Errorf("time left to send = %v, want just under 10s", c.Remaining)
	}

	d := coreSingle(t, e)
	if d.Status != "sent" || d.Attempts != 1 || d.LastError != "" || d.NextAttemptAt != nil || d.SentAt == nil || *d.SentAt != coreTime(later) ||
		d.LastAttemptAt == nil || *d.LastAttemptAt != coreTime(later) {
		t.Errorf("delivery = %+v, want sent at %s after one attempt", d, coreTime(later))
	}

	e.dispatch(later.Add(time.Hour))
	if n := len(fakeCalls()); n != 1 {
		t.Errorf("a sent delivery was sent again: %d sends", n)
	}
}

func TestDispatchLeavesTheLinkEmptyWithoutAPublicURLAndTheTankEmptyWithoutATank(t *testing.T) {
	e := newCoreEnvAt(t, "")
	e.addChannel("main")
	e.raise(coreEvent("a"))
	e.dispatch(coreStart)

	c := fakeCalls()[0]
	if c.Msg.Link != "" || c.Msg.TankID != 0 || c.Msg.TankName != "" {
		t.Errorf("message = %+v, want no link and no tank", c.Msg)
	}
}

func TestDispatchSendsOnlyWhatIsDue(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.raise(coreEvent("a"))
	testutil.Exec(t, e.db, `UPDATE notification_deliveries SET next_attempt_at = ?`, coreTime(coreStart.Add(time.Minute)))

	e.dispatch(coreStart.Add(time.Minute - time.Second))
	if n := len(fakeCalls()); n != 0 {
		t.Fatalf("sent %d messages a second before the delivery was due", n)
	}
	e.dispatch(coreStart.Add(time.Minute))
	if n := len(fakeCalls()); n != 1 {
		t.Errorf("sent %d messages at the due time, want 1", n)
	}
}

func TestDispatchRetriesAfterOneTenAndSixtyMinutesThenFails(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.raise(coreEvent("a"))
	coreFailing(errors.New("boom"))

	steps := []struct {
		at        time.Duration
		sends     int
		status    string
		attempts  int64
		nextAfter time.Duration // zero when no further attempt is planned
		attempted time.Duration // when the latest attempt was made
	}{
		{0, 1, "pending", 1, time.Minute, 0},
		{time.Minute - time.Second, 1, "pending", 1, time.Minute, 0},
		{time.Minute, 2, "pending", 2, time.Minute + 10*time.Minute, time.Minute},
		{11*time.Minute - time.Second, 2, "pending", 2, time.Minute + 10*time.Minute, time.Minute},
		{11 * time.Minute, 3, "pending", 3, 11*time.Minute + time.Hour, 11 * time.Minute},
		{71*time.Minute - time.Second, 3, "pending", 3, 11*time.Minute + time.Hour, 11 * time.Minute},
		{71 * time.Minute, 4, "failed", 4, 0, 71 * time.Minute},
		{24 * time.Hour, 4, "failed", 4, 0, 71 * time.Minute},
	}
	for _, s := range steps {
		e.dispatch(coreStart.Add(s.at))
		d := coreSingle(t, e)
		if n := len(fakeCalls()); n != s.sends || d.Status != s.status || d.Attempts != s.attempts {
			t.Fatalf("at +%v: sends=%d status=%s attempts=%d, want %d %s %d", s.at, n, d.Status, d.Attempts, s.sends, s.status, s.attempts)
		}
		switch {
		case s.nextAfter == 0 && d.NextAttemptAt != nil:
			t.Fatalf("at +%v: next attempt %s planned for a failed delivery", s.at, *d.NextAttemptAt)
		case s.nextAfter != 0 && (d.NextAttemptAt == nil || *d.NextAttemptAt != coreTime(coreStart.Add(s.nextAfter))):
			got := "none"
			if d.NextAttemptAt != nil {
				got = *d.NextAttemptAt
			}
			t.Fatalf("at +%v: next attempt = %s, want %s", s.at, got, coreTime(coreStart.Add(s.nextAfter)))
		}
		if d.LastError != "boom" {
			t.Fatalf("at +%v: last error = %q", s.at, d.LastError)
		}
		if d.LastAttemptAt == nil || *d.LastAttemptAt != coreTime(coreStart.Add(s.attempted)) {
			t.Fatalf("at +%v: last attempt = %v, want +%v", s.at, d.LastAttemptAt, s.attempted)
		}
	}
}

func TestDispatchRecoversAfterARetry(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.raise(coreEvent("a"))
	failed := true
	fakeOutcome(func(context.Context, fakeCall) error {
		if failed {
			failed = false
			return errors.New("boom")
		}
		return nil
	})

	e.dispatch(coreStart)
	e.dispatch(coreStart.Add(time.Minute))

	d := coreSingle(t, e)
	if d.Status != "sent" || d.Attempts != 2 || d.LastError != "" || d.NextAttemptAt != nil || d.SentAt == nil || *d.SentAt != coreTime(coreStart.Add(time.Minute)) {
		t.Errorf("delivery = %+v, want sent on the second attempt with the error cleared", d)
	}
}

func TestDispatchHonoursALongerRetryAfterAndCapsIt(t *testing.T) {
	cases := []struct {
		name string
		err  error
		next time.Duration
	}{
		{"longer than the backoff", fmt.Errorf("send: %w", coreThrottled{5 * time.Minute}), 5 * time.Minute},
		{"shorter than the backoff", coreThrottled{10 * time.Second}, time.Minute},
		{"far too long", coreThrottled{72 * time.Hour}, 24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCoreEnv(t)
			e.addChannel("main")
			e.raise(coreEvent("a"))
			coreFailing(tc.err)

			e.dispatch(coreStart)

			d := coreSingle(t, e)
			if d.Status != "pending" || d.NextAttemptAt == nil || *d.NextAttemptAt != coreTime(coreStart.Add(tc.next)) {
				t.Errorf("delivery = %+v, want the next attempt at +%v", d, tc.next)
			}
		})
	}
}

func TestDispatchStopsRetryingADestinationThatIsGone(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.raise(coreEvent("a"))
	coreFailing(fmt.Errorf("discord: %w", coreGone{}))

	e.dispatch(coreStart)

	d := coreSingle(t, e)
	if d.Status != "failed" || d.Attempts != 1 || d.NextAttemptAt != nil || !strings.Contains(d.LastError, "webhook deleted") {
		t.Errorf("delivery = %+v, want failed at once", d)
	}
	e.dispatch(coreStart.Add(24 * time.Hour))
	if n := len(fakeCalls()); n != 1 {
		t.Errorf("sends = %d, want no retry", n)
	}
}

func TestDispatchFailsAtOnceForADisabledChannelWithoutSending(t *testing.T) {
	e := newCoreEnv(t)
	ch := e.addChannel("main")
	e.raise(coreEvent("a"))
	rec := e.admin(http.MethodPatch, "/api/v1/notification-channels/"+coreID(ch.ID), map[string]any{"enabled": false})
	testutil.ExpectStatus(t, rec, http.StatusOK)

	e.dispatch(coreStart)

	d := coreSingle(t, e)
	if d.Status != "failed" || d.Attempts != 1 || d.NextAttemptAt != nil || !strings.Contains(d.LastError, "disabled") {
		t.Errorf("delivery = %+v, want failed at once as disabled", d)
	}
	if n := len(fakeCalls()); n != 0 {
		t.Errorf("sends to a disabled channel = %d", n)
	}
}

func TestDispatchFailsAtOnceForAnUnknownKindOrAStoredConfigThatNoLongerBuilds(t *testing.T) {
	cases := []struct {
		name, kind, config, wantError string
	}{
		{"unknown kind", "nosuch", `{"url":"x"}`, `unknown channel kind "nosuch"`},
		{"invalid config", "fake", `{}`, "config.token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCoreEnv(t)
			ch := testutil.Insert(t, e.db, `INSERT INTO notification_channels (name, kind, config_json) VALUES ('legacy', ?, ?)`, tc.kind, tc.config)
			testutil.Exec(t, e.db, `INSERT INTO notification_subscriptions (channel_id, event_type) VALUES (?, 'maintenance_due')`, ch)
			e.raise(coreEvent("a"))

			e.dispatch(coreStart)

			d := coreSingle(t, e)
			if d.Status != "failed" || d.Attempts != 1 || d.NextAttemptAt != nil || !strings.Contains(d.LastError, tc.wantError) {
				t.Errorf("delivery = %+v, want failed at once with %q", d, tc.wantError)
			}
			if n := len(fakeCalls()); n != 0 {
				t.Errorf("sends = %d", n)
			}
		})
	}
}

func TestDispatchKeepsTheFirst500CharactersOfAnError(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.raise(coreEvent("a"))
	coreFailing(errors.New(strings.Repeat("é", 1000)))

	e.dispatch(coreStart)

	d := coreSingle(t, e)
	if utf8.RuneCountInString(d.LastError) != 500 || !utf8.ValidString(d.LastError) || d.LastError != strings.Repeat("é", 500) {
		t.Errorf("last error has %d characters, valid=%v", utf8.RuneCountInString(d.LastError), utf8.ValidString(d.LastError))
	}
}

func TestDispatchSendsOneBatchOfFiftyOldestFirst(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	for i := 1; i <= 60; i++ {
		ev := coreEvent(fmt.Sprintf("o%02d", i))
		ev.Title = fmt.Sprintf("n%02d", i)
		e.raise(ev)
	}

	e.dispatch(coreStart)
	calls := fakeCalls()
	if len(calls) != 50 {
		t.Fatalf("first tick sent %d, want 50", len(calls))
	}
	for i, c := range calls {
		if want := fmt.Sprintf("n%02d", i+1); c.Msg.Title != want {
			t.Fatalf("send %d was %s, want %s", i, c.Msg.Title, want)
		}
	}

	e.dispatch(coreStart)
	if n := len(fakeCalls()); n != 60 {
		t.Errorf("after the second tick %d were sent, want 60", n)
	}
}

func TestDispatchDatesEachAttemptByTheClockAtThatAttempt(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail bool
	}{{"failed attempts", true}, {"sent attempts", false}} {
		t.Run(tc.name, func(t *testing.T) {
			e := newCoreEnv(t)
			e.addChannel("main")
			for i := 1; i <= 3; i++ {
				e.raise(coreEvent(fmt.Sprintf("o%d", i)))
			}
			// Every send takes 20 seconds, so the third attempt of one tick happens a minute after the tick.
			fakeOutcome(func(context.Context, fakeCall) error {
				e.clock = e.clock.Add(20 * time.Second)
				if tc.fail {
					return errors.New("slow failure")
				}
				return nil
			})

			e.dispatch(coreStart)

			ds := e.deliveries()
			if len(ds) != 3 {
				t.Fatalf("deliveries = %d", len(ds))
			}
			for i, d := range ds {
				attempted := coreStart.Add(time.Duration(i+1) * 20 * time.Second)
				if d.LastAttemptAt == nil || *d.LastAttemptAt != coreTime(attempted) {
					t.Errorf("delivery %d last attempt = %v, want %s", i+1, d.LastAttemptAt, coreTime(attempted))
				}
				switch {
				case tc.fail && (d.NextAttemptAt == nil || *d.NextAttemptAt != coreTime(attempted.Add(time.Minute))):
					t.Errorf("delivery %d next attempt = %v, want a minute after %s", i+1, d.NextAttemptAt, coreTime(attempted))
				case !tc.fail && (d.SentAt == nil || *d.SentAt != coreTime(attempted)):
					t.Errorf("delivery %d sent at = %v, want %s", i+1, d.SentAt, coreTime(attempted))
				}
			}
		})
	}
}

func TestDispatchTreatsAPanickingChannelAsAFailedAttempt(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.raise(coreEvent("a"))
	fakeOutcome(func(context.Context, fakeCall) error { panic("kaboom") })

	e.dispatch(coreStart)

	d := coreSingle(t, e)
	if d.Status != "pending" || d.Attempts != 1 || !strings.Contains(d.LastError, "kaboom") {
		t.Errorf("delivery = %+v, want one failed attempt mentioning the panic", d)
	}
}

func TestDispatchLeavesAnInterruptedDeliveryPendingWithoutCountingTheAttempt(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.raise(coreEvent("a"))
	ctx, cancel := context.WithCancel(t.Context())
	fakeOutcome(func(context.Context, fakeCall) error {
		cancel()
		return errors.New("interrupted")
	})

	if err := e.svc.Dispatch(ctx, coreStart); !errors.Is(err, context.Canceled) {
		t.Errorf("dispatch error = %v, want context.Canceled", err)
	}

	d := coreSingle(t, e)
	if d.Status != "pending" || d.Attempts != 0 || d.LastError != "" || d.NextAttemptAt == nil || *d.NextAttemptAt != coreTime(coreStart) {
		t.Errorf("delivery = %+v, want untouched", d)
	}
}

func TestDispatchJobRunsAtEveryTickAndSendsWhatIsDue(t *testing.T) {
	e := newCoreEnv(t)
	e.addChannel("main")
	e.raise(coreEvent("a"))

	jobs := Jobs(e.svc.Deps)
	names := make([]string, len(jobs))
	for i, j := range jobs {
		names[i] = j.Name
	}
	if len(jobs) != 2 || names[0] != "notifications-dispatch" || names[1] != "notifications-prune" {
		t.Fatalf("jobs = %v", names)
	}

	dispatch := jobs[0]
	for _, last := range []time.Time{{}, coreStart, coreStart.Add(-time.Second)} {
		if due, err := dispatch.Trigger.Due(t.Context(), coreStart, last); err != nil || !due {
			t.Errorf("dispatch due with last=%v: %v, %v", last, due, err)
		}
	}
	if err := dispatch.Run(t.Context(), coreStart); err != nil {
		t.Fatal(err)
	}
	if n := len(fakeCalls()); n != 1 {
		t.Errorf("sends from the job = %d, want 1", n)
	}

	prune := jobs[1]
	for _, tc := range []struct {
		last time.Time
		want bool
	}{{time.Time{}, true}, {coreStart.Add(-23 * time.Hour), false}, {coreStart.Add(-24 * time.Hour), true}} {
		if due, err := prune.Trigger.Due(t.Context(), coreStart, tc.last); err != nil || due != tc.want {
			t.Errorf("prune due with last=%v: %v, %v, want %v", tc.last, due, err, tc.want)
		}
	}
}
