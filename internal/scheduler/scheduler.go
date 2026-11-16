// Package scheduler runs the application's background jobs on a wall-clock tick, in memory, one goroutine per due job.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultInterval = time.Minute
	defaultTimeout  = 2 * time.Minute
)

// Job is one unit of background work; it must be idempotent because a restart forgets when it last ran.
type Job struct {
	Name    string
	Trigger Trigger
	// Timeout bounds one run through its context; zero means 2 minutes.
	Timeout time.Duration
	Run     func(ctx context.Context, now time.Time) error
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithInterval sets the time between ticks, one minute by default.
func WithInterval(d time.Duration) Option {
	return func(s *Scheduler) {
		if d > 0 {
			s.interval = d
		}
	}
}

// WithClock replaces the wall clock that Run reads.
func WithClock(now func() time.Time) Option {
	return func(s *Scheduler) { s.clock = now }
}

// entry is a job with its in-memory run state.
type entry struct {
	Job
	running atomic.Bool
	last    time.Time // guarded by Scheduler.mu
}

// Scheduler decides on every tick which jobs are due and runs each in its own goroutine.
type Scheduler struct {
	logger   *slog.Logger
	interval time.Duration
	clock    func() time.Time
	entries  []*entry

	mu sync.Mutex // serialises ticks and guards entry.last
	wg sync.WaitGroup
}

// New returns a scheduler for jobs; nothing runs until Run or Tick is called.
func New(logger *slog.Logger, jobs []Job, opts ...Option) *Scheduler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	s := &Scheduler{logger: logger, interval: defaultInterval, clock: time.Now}
	for _, j := range jobs {
		s.entries = append(s.entries, &entry{Job: j})
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run ticks once immediately, then at every interval boundary of the wall clock, and returns after ctx ends and the running jobs return.
func (s *Scheduler) Run(ctx context.Context) {
	defer s.wg.Wait()
	for {
		current := s.clock()
		s.start(ctx, current.Truncate(s.interval))
		timer := time.NewTimer(nextTick(current, s.interval).Sub(current))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// Tick starts the jobs due at now and waits for them to finish, which tests use to step the scheduler by hand.
func (s *Scheduler) Tick(ctx context.Context, now time.Time) {
	s.start(ctx, now).Wait()
}

// nextTick is the first interval boundary after now.
func nextTick(now time.Time, interval time.Duration) time.Time {
	return now.Truncate(interval).Add(interval)
}

// start launches every due job that is not still running and returns a group that completes when those runs finish.
func (s *Scheduler) start(ctx context.Context, now time.Time) *sync.WaitGroup {
	s.mu.Lock()
	defer s.mu.Unlock()
	var started sync.WaitGroup
	for _, e := range s.entries {
		if e.running.Load() {
			s.logger.Debug("job still running, skipped", slog.String("job", e.Name))
			continue
		}
		due, err := e.Trigger.Due(ctx, now, e.last)
		if err != nil {
			s.logger.Warn("job trigger failed", slog.String("job", e.Name), slog.Any("error", err))
			continue
		}
		if !due {
			continue
		}
		e.last = now
		e.running.Store(true)
		started.Add(1)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer started.Done()
			s.runJob(ctx, e, now)
		}()
	}
	return &started
}

// runJob runs one job under its timeout and logs how it ended, so a panic or an error never reaches the other jobs.
func (s *Scheduler) runJob(ctx context.Context, e *entry, now time.Time) {
	defer e.running.Store(false)
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	began := time.Now()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("job panicked", slog.String("job", e.Name), slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
		}
	}()
	err := e.Run(jobCtx, now)
	switch {
	case err != nil && errors.Is(err, context.DeadlineExceeded) && jobCtx.Err() != nil:
		s.logger.Warn("job timed out", slog.String("job", e.Name), slog.Duration("duration", time.Since(began)), slog.Duration("timeout", timeout))
	case err != nil:
		s.logger.Warn("job failed", slog.String("job", e.Name), slog.Duration("duration", time.Since(began)), slog.Any("error", err))
	}
}
