package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func testLogger() (*slog.Logger, *syncBuffer) {
	buf := &syncBuffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

// counter returns a job body that counts its runs and records the tick time of the latest one.
func counter() (run func(context.Context, time.Time) error, runs *atomic.Int32, lastNow *atomic.Value) {
	runs, lastNow = new(atomic.Int32), new(atomic.Value)
	return func(_ context.Context, now time.Time) error {
		runs.Add(1)
		lastNow.Store(now)
		return nil
	}, runs, lastNow
}

func wait(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestNextTickAlignsToTheIntervalBoundary(t *testing.T) {
	cases := []struct {
		now      time.Time
		interval time.Duration
		want     time.Time
	}{
		{time.Date(2026, 5, 1, 10, 7, 23, 500, time.UTC), time.Minute, time.Date(2026, 5, 1, 10, 8, 0, 0, time.UTC)},
		{time.Date(2026, 5, 1, 10, 7, 0, 0, time.UTC), time.Minute, time.Date(2026, 5, 1, 10, 8, 0, 0, time.UTC)},
		{time.Date(2026, 5, 1, 10, 59, 59, 0, time.UTC), time.Minute, time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)},
		{time.Date(2026, 5, 1, 10, 7, 23, 0, time.UTC), 5 * time.Second, time.Date(2026, 5, 1, 10, 7, 25, 0, time.UTC)},
	}
	for _, tc := range cases {
		if got := nextTick(tc.now, tc.interval); !got.Equal(tc.want) {
			t.Errorf("nextTick(%v, %v) = %v, want %v", tc.now, tc.interval, got, tc.want)
		}
	}
}

func TestTickRunsOnlyDueJobsWithTheTickTime(t *testing.T) {
	logger, _ := testLogger()
	everyRun, everyRuns, everyNow := counter()
	tickRun, tickRuns, _ := counter()
	never := func(context.Context, time.Time, time.Time) (bool, error) { return false, nil }
	s := New(logger, []Job{
		{Name: "every", Trigger: Every(time.Hour), Run: everyRun},
		{Name: "tick", Trigger: EveryTick(), Run: tickRun},
		{Name: "never", Trigger: triggerFunc(never), Run: func(context.Context, time.Time) error {
			t.Error("a job that is not due ran")
			return nil
		}},
	})
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	s.Tick(t.Context(), t0)
	s.Tick(t.Context(), t0.Add(30*time.Minute))
	s.Tick(t.Context(), t0.Add(time.Hour))

	if everyRuns.Load() != 2 || tickRuns.Load() != 3 {
		t.Errorf("runs: every=%d tick=%d; want 2 and 3", everyRuns.Load(), tickRuns.Load())
	}
	if got, _ := everyNow.Load().(time.Time); !got.Equal(t0.Add(time.Hour)) {
		t.Errorf("last tick time = %v, want %v", got, t0.Add(time.Hour))
	}
}

func TestDailyAtCatchesUpOnceThroughTheScheduler(t *testing.T) {
	loc := berlin(t)
	logger, _ := testLogger()
	run, runs, _ := counter()
	s := New(logger, []Job{{Name: "daily", Trigger: DailyAt(fixedAt("09:00"), fixedLoc(loc)), Run: run}})

	s.Tick(t.Context(), time.Date(2026, 6, 1, 8, 0, 0, 0, loc))
	s.Tick(t.Context(), time.Date(2026, 6, 1, 9, 0, 0, 0, loc))
	if runs.Load() != 1 {
		t.Fatalf("runs at 09:00 = %d, want 1", runs.Load())
	}
	// The server was down for two days; the first tick back runs the job once, and the next ticks do not.
	for _, now := range []time.Time{
		time.Date(2026, 6, 3, 15, 0, 0, 0, loc),
		time.Date(2026, 6, 3, 15, 1, 0, 0, loc),
		time.Date(2026, 6, 3, 16, 0, 0, 0, loc),
	} {
		s.Tick(t.Context(), now)
	}
	if runs.Load() != 2 {
		t.Errorf("runs after the downtime = %d, want 2 (one catch-up)", runs.Load())
	}
	s.Tick(t.Context(), time.Date(2026, 6, 4, 9, 0, 0, 0, loc))
	if runs.Load() != 3 {
		t.Errorf("runs the next morning = %d, want 3", runs.Load())
	}
}

func TestJobNeverStartsTwiceWhileRunning(t *testing.T) {
	logger, logs := testLogger()
	var runs atomic.Int32
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRuns := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseRuns()
	s := New(logger, []Job{{Name: "slow", Trigger: EveryTick(), Run: func(context.Context, time.Time) error {
		runs.Add(1)
		started <- struct{}{}
		<-release
		return nil
	}}})
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	first := s.start(t.Context(), now)
	wait(t, started, "the first run to start")
	s.start(t.Context(), now.Add(time.Minute))
	select {
	case <-started:
		t.Fatal("a second run started while the first was still going")
	case <-time.After(100 * time.Millisecond):
	}
	if runs.Load() != 1 {
		t.Fatalf("runs while the first was going = %d, want 1", runs.Load())
	}
	if !strings.Contains(logs.String(), "job still running, skipped") {
		t.Errorf("no skip logged:\n%s", logs)
	}

	releaseRuns()
	first.Wait()
	s.Tick(t.Context(), now.Add(2*time.Minute))
	if runs.Load() != 2 {
		t.Errorf("runs after the first finished = %d, want 2", runs.Load())
	}
}

func TestPanicIsRecoveredLoggedAndOtherJobsStillRun(t *testing.T) {
	logger, logs := testLogger()
	run, runs, _ := counter()
	var panics atomic.Int32
	s := New(logger, []Job{
		{Name: "exploder", Trigger: EveryTick(), Run: func(context.Context, time.Time) error {
			panics.Add(1)
			panic("kaboom")
		}},
		{Name: "steady", Trigger: EveryTick(), Run: run},
	})
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	s.Tick(t.Context(), now)
	s.Tick(t.Context(), now.Add(time.Minute))

	if panics.Load() != 2 {
		t.Errorf("the panicking job ran %d times, want 2 (it is not left marked as running)", panics.Load())
	}
	if runs.Load() != 2 {
		t.Errorf("the other job ran %d times, want 2", runs.Load())
	}
	out := logs.String()
	if !strings.Contains(out, "job panicked") || !strings.Contains(out, "job=exploder") || !strings.Contains(out, "kaboom") {
		t.Errorf("panic not logged with the job name:\n%s", out)
	}
}

func TestFailureIsLoggedAtWarnWithNameAndDuration(t *testing.T) {
	logger, logs := testLogger()
	s := New(logger, []Job{{Name: "flaky", Trigger: EveryTick(), Run: func(context.Context, time.Time) error {
		return errors.New("disk full")
	}}})
	s.Tick(t.Context(), time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC))

	out := logs.String()
	for _, want := range []string{"level=WARN", "job failed", "job=flaky", "duration=", "disk full"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
}

func TestTriggerErrorSkipsTheJobAndIsLogged(t *testing.T) {
	logger, logs := testLogger()
	run, runs, _ := counter()
	bad := triggerFunc(func(context.Context, time.Time, time.Time) (bool, error) { return false, errors.New("no zone") })
	s := New(logger, []Job{{Name: "daily", Trigger: bad, Run: run}})
	s.Tick(t.Context(), time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC))

	if runs.Load() != 0 {
		t.Error("a job whose trigger failed ran")
	}
	if !strings.Contains(logs.String(), "job trigger failed") || !strings.Contains(logs.String(), "job=daily") {
		t.Errorf("trigger failure not logged:\n%s", logs)
	}
}

func TestTimeoutDefaultsToTwoMinutesAndCanBeSet(t *testing.T) {
	logger, _ := testLogger()
	deadlines := map[string]time.Duration{}
	var mu sync.Mutex
	probe := func(name string) func(context.Context, time.Time) error {
		return func(ctx context.Context, _ time.Time) error {
			dl, ok := ctx.Deadline()
			mu.Lock()
			defer mu.Unlock()
			if !ok {
				deadlines[name] = -1
				return nil
			}
			deadlines[name] = time.Until(dl)
			return nil
		}
	}
	s := New(logger, []Job{
		{Name: "default", Trigger: EveryTick(), Run: probe("default")},
		{Name: "custom", Trigger: EveryTick(), Timeout: 10 * time.Second, Run: probe("custom")},
	})
	s.Tick(t.Context(), time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC))

	within := func(got, want time.Duration) bool { return got > want-5*time.Second && got <= want }
	if !within(deadlines["default"], 2*time.Minute) {
		t.Errorf("default deadline in %v, want about 2m", deadlines["default"])
	}
	if !within(deadlines["custom"], 10*time.Second) {
		t.Errorf("custom deadline in %v, want about 10s", deadlines["custom"])
	}
}

func TestTimeoutCancelsTheJobContext(t *testing.T) {
	logger, logs := testLogger()
	var jobErr atomic.Value
	s := New(logger, []Job{{Name: "hang", Trigger: EveryTick(), Timeout: 20 * time.Millisecond, Run: func(ctx context.Context, _ time.Time) error {
		select {
		case <-ctx.Done():
			jobErr.Store(ctx.Err())
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("the timeout never fired")
		}
	}}})
	s.Tick(t.Context(), time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC))

	if err, _ := jobErr.Load().(error); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("job context error = %v, want deadline exceeded", err)
	}
	if !strings.Contains(logs.String(), "job timed out") || !strings.Contains(logs.String(), "job=hang") {
		t.Errorf("timeout not logged:\n%s", logs)
	}
}

func TestRunTicksImmediatelyAtTheAlignedTime(t *testing.T) {
	logger, _ := testLogger()
	run, _, lastNow := counter()
	ran := make(chan struct{}, 1)
	s := New(logger, []Job{{Name: "j", Trigger: EveryTick(), Run: func(ctx context.Context, now time.Time) error {
		err := run(ctx, now)
		ran <- struct{}{}
		return err
	}}},
		WithClock(func() time.Time { return time.Date(2026, 5, 1, 10, 7, 23, 500, time.UTC) }),
		WithInterval(time.Hour),
	)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	wait(t, ran, "the immediate tick")
	cancel()
	wait(t, done, "Run to return")
	if got, _ := lastNow.Load().(time.Time); !got.Equal(time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("tick time = %v, want it aligned to the interval", got)
	}
}

func TestRunKeepsTickingEveryInterval(t *testing.T) {
	logger, _ := testLogger()
	var runs atomic.Int32
	three := make(chan struct{})
	s := New(logger, []Job{{Name: "j", Trigger: EveryTick(), Run: func(context.Context, time.Time) error {
		if runs.Add(1) == 3 {
			close(three)
		}
		return nil
	}}}, WithInterval(5*time.Millisecond))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	wait(t, three, "three ticks")
	cancel()
	wait(t, done, "Run to return")
}

func TestRunReturnsOnlyAfterRunningJobsReturn(t *testing.T) {
	logger, _ := testLogger()
	started := make(chan struct{})
	release := make(chan struct{})
	s := New(logger, []Job{{Name: "slow", Trigger: EveryTick(), Run: func(context.Context, time.Time) error {
		close(started)
		<-release
		return nil
	}}}, WithInterval(time.Hour))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	wait(t, started, "the job to start")
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while a job was still running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	wait(t, done, "Run to return after the job finished")
}

func TestNewToleratesNilLoggerAndIgnoresBadInterval(t *testing.T) {
	run, runs, _ := counter()
	s := New(nil, []Job{{Name: "j", Trigger: EveryTick(), Run: run}}, WithInterval(0), WithInterval(-time.Second))
	if s.interval != time.Minute {
		t.Errorf("interval = %v, want the one-minute default", s.interval)
	}
	s.Tick(t.Context(), time.Now())
	if runs.Load() != 1 {
		t.Errorf("runs = %d, want 1", runs.Load())
	}
}
