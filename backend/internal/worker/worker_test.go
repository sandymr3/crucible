package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPoolRunsJobs(t *testing.T) {
	p := NewPool(testLogger(), Config{Workers: 2, QueueSize: 16, MaxAttempts: 1})

	var done sync.WaitGroup
	done.Add(3)
	var count atomic.Int32

	p.Register(JobEvaluateTurn, func(context.Context, Job) error {
		count.Add(1)
		done.Done()
		return nil
	})
	p.Start(context.Background())
	defer p.Stop()

	for i := 0; i < 3; i++ {
		if err := p.Submit(Job{Kind: JobEvaluateTurn, SessionID: "s1"}); err != nil {
			t.Fatalf("submit failed: %v", err)
		}
	}

	waitOrFail(t, &done, 3*time.Second)
	if got := count.Load(); got != 3 {
		t.Errorf("ran %d jobs, want 3", got)
	}
}

func TestPoolRetriesThenGivesUp(t *testing.T) {
	p := NewPool(testLogger(), Config{Workers: 1, QueueSize: 16, MaxAttempts: 3})

	var attempts atomic.Int32
	var done sync.WaitGroup
	done.Add(3)

	p.Register(JobEvaluateTurn, func(context.Context, Job) error {
		attempts.Add(1)
		done.Done()
		return errors.New("vertex unavailable")
	})
	p.Start(context.Background())
	defer p.Stop()

	if err := p.Submit(Job{Kind: JobEvaluateTurn, SessionID: "s1"}); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	waitOrFail(t, &done, 3*time.Second)
	// Settle, then confirm it stopped rather than retrying forever — an
	// unbounded retry against a failing model call would burn credits silently.
	time.Sleep(200 * time.Millisecond)
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempted %d times, want exactly 3", got)
	}
}

func TestPoolStopsRetryingAfterSuccess(t *testing.T) {
	p := NewPool(testLogger(), Config{Workers: 1, QueueSize: 16, MaxAttempts: 3})

	var attempts atomic.Int32
	p.Register(JobEvaluateTurn, func(context.Context, Job) error {
		if attempts.Add(1) < 2 {
			return errors.New("transient")
		}
		return nil
	})
	p.Start(context.Background())
	defer p.Stop()

	_ = p.Submit(Job{Kind: JobEvaluateTurn, SessionID: "s1"})

	time.Sleep(500 * time.Millisecond)
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempted %d times, want 2 (one failure then success)", got)
	}
}

// A full queue must drop rather than block. The caller is a live interview, and
// blocking it to wait for a grader would invert the design principle that the
// conversation never waits on enrichment.
func TestSubmitDropsWhenQueueFullRatherThanBlocking(t *testing.T) {
	const queueSize = 2
	p := NewPool(testLogger(), Config{Workers: 1, QueueSize: queueSize, MaxAttempts: 1})

	occupied := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	p.Register(JobEvaluateTurn, func(context.Context, Job) error {
		once.Do(func() { close(occupied) })
		<-release
		return nil
	})
	p.Start(context.Background())

	// Occupy the single worker, and wait for confirmation. Without this
	// barrier the test races the scheduler: whether the queue is full after N
	// submits depends on how many jobs the worker has already dequeued.
	if err := p.Submit(Job{Kind: JobEvaluateTurn, SessionID: "occupier"}); err != nil {
		t.Fatalf("initial submit failed: %v", err)
	}
	select {
	case <-occupied:
	case <-time.After(2 * time.Second):
		t.Fatal("worker never picked up the first job")
	}

	// The worker is now blocked, so nothing drains the queue. Exactly
	// queueSize submits fit.
	for i := 0; i < queueSize; i++ {
		if err := p.Submit(Job{Kind: JobEvaluateTurn, SessionID: "filler"}); err != nil {
			t.Fatalf("submit %d should fit in the queue: %v", i, err)
		}
	}

	returned := make(chan error, 1)
	go func() { returned <- p.Submit(Job{Kind: JobEvaluateTurn, SessionID: "overflow"}) }()

	select {
	case err := <-returned:
		if err == nil {
			t.Error("Submit accepted a job into a full queue, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Submit blocked on a full queue — it must never block the caller")
	}

	close(release)
	p.Stop()
}

func TestUnregisteredKindDoesNotCrashPool(t *testing.T) {
	p := NewPool(testLogger(), Config{Workers: 1, QueueSize: 8, MaxAttempts: 1})

	var ran atomic.Bool
	p.Register(JobFinalize, func(context.Context, Job) error {
		ran.Store(true)
		return nil
	})
	p.Start(context.Background())
	defer p.Stop()

	// An unhandled kind must be logged and skipped, not panic the worker —
	// a dead worker would silently stop grading every later turn.
	_ = p.Submit(Job{Kind: JobBuildRoadmap, SessionID: "s1"})
	_ = p.Submit(Job{Kind: JobFinalize, SessionID: "s1"})

	time.Sleep(300 * time.Millisecond)
	if !ran.Load() {
		t.Error("pool stopped processing after an unregistered job kind")
	}
}

func TestDefaultConfigAppliedForZeroValues(t *testing.T) {
	p := NewPool(testLogger(), Config{})
	if p.cfg.Workers == 0 || p.cfg.QueueSize == 0 || p.cfg.MaxAttempts == 0 {
		t.Errorf("zero-value config not defaulted: %+v", p.cfg)
	}
}

func waitOrFail(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for jobs")
	}
}
