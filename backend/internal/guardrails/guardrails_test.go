package guardrails

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/santh/crucible/internal/config"
)

// fakeCounters stands in for Firestore.
type fakeCounters struct {
	mu sync.Mutex

	active    int
	activeErr error

	daily      map[string]int
	dailyErr   error
	dailyCalls int
}

func newFakeCounters() *fakeCounters {
	return &fakeCounters{daily: map[string]int{}}
}

func (f *fakeCounters) CountActiveSessions(context.Context, string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active, f.activeErr
}

func (f *fakeCounters) IncrementDailySessions(_ context.Context, uid string, cap int) (bool, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dailyCalls++
	if f.dailyErr != nil {
		return false, 0, f.dailyErr
	}
	if f.daily[uid] >= cap {
		return false, f.daily[uid], nil
	}
	f.daily[uid]++
	return true, f.daily[uid], nil
}

func testGuard(t *testing.T, fc *fakeCounters, maxConcurrent, dailyCap int) *Guard {
	t.Helper()
	return New(&config.Config{
		MaxConcurrent:   maxConcurrent,
		DailySessionCap: dailyCap,
		EvalMinWords:    15,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), fc)
}

func TestDailyCapAllowsExactlyTheCap(t *testing.T) {
	fc := newFakeCounters()
	g := testGuard(t, fc, 10, 5)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		if err := g.CheckDailyCap(ctx, "u1"); err != nil {
			t.Fatalf("session %d rejected, want allowed: %v", i, err)
		}
	}
	err := g.CheckDailyCap(ctx, "u1")
	if !errors.Is(err, ErrDailyCapReached) {
		t.Errorf("6th session error = %v, want ErrDailyCapReached", err)
	}
}

func TestDailyCapIsPerUser(t *testing.T) {
	// One enthusiastic tester must not exhaust anyone else's allocation.
	fc := newFakeCounters()
	g := testGuard(t, fc, 10, 2)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := g.CheckDailyCap(ctx, "u1"); err != nil {
			t.Fatalf("u1 session %d rejected: %v", i, err)
		}
	}
	if err := g.CheckDailyCap(ctx, "u1"); !errors.Is(err, ErrDailyCapReached) {
		t.Fatalf("u1 should be capped, got %v", err)
	}
	if err := g.CheckDailyCap(ctx, "u2"); err != nil {
		t.Errorf("u2 rejected because of u1's usage: %v", err)
	}
}

// A Firestore outage must not make the product unusable. The hard duration and
// idle caps still bound the damage, and a demo that refuses to start is worse
// than one that costs slightly more.
func TestDailyCapFailsOpenOnStoreError(t *testing.T) {
	fc := newFakeCounters()
	fc.dailyErr = errors.New("firestore unavailable")
	g := testGuard(t, fc, 10, 5)

	if err := g.CheckDailyCap(context.Background(), "u1"); err != nil {
		t.Errorf("CheckDailyCap failed closed on store error: %v", err)
	}
}

func TestConcurrencyCapRejectsBeyondLimit(t *testing.T) {
	fc := newFakeCounters()
	g := testGuard(t, fc, 3, 100)
	ctx := context.Background()

	var releases []func()
	for i := 1; i <= 3; i++ {
		rel, err := g.AcquireSession(ctx, uidFor(i))
		if err != nil {
			t.Fatalf("acquire %d failed: %v", i, err)
		}
		releases = append(releases, rel)
	}

	if _, err := g.AcquireSession(ctx, uidFor(4)); !errors.Is(err, ErrTooManyConcurrent) {
		t.Errorf("4th acquire error = %v, want ErrTooManyConcurrent", err)
	}
	if got := g.ActiveCount(); got != 3 {
		t.Errorf("ActiveCount = %d, want 3 — a rejected acquire must not leak a slot", got)
	}

	releases[0]()
	if got := g.ActiveCount(); got != 2 {
		t.Errorf("ActiveCount after release = %d, want 2", got)
	}
	if _, err := g.AcquireSession(ctx, uidFor(4)); err != nil {
		t.Errorf("acquire after release failed: %v", err)
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	// Teardown can race: the relay defers a release and an error path may call
	// it too. Double-releasing must not free a slot that was never held, or the
	// concurrency cap drifts upward over the life of the process.
	fc := newFakeCounters()
	g := testGuard(t, fc, 5, 100)

	rel, err := g.AcquireSession(context.Background(), "u1")
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	rel()
	rel()
	rel()

	if got := g.ActiveCount(); got != 0 {
		t.Errorf("ActiveCount = %d, want 0", got)
	}
}

func TestRejectsSecondLiveSessionForSameUser(t *testing.T) {
	fc := newFakeCounters()
	fc.active = 1 // this user already has a live session
	g := testGuard(t, fc, 10, 100)

	_, err := g.AcquireSession(context.Background(), "u1")
	if !errors.Is(err, ErrAlreadyLive) {
		t.Errorf("error = %v, want ErrAlreadyLive", err)
	}
	if got := g.ActiveCount(); got != 0 {
		t.Errorf("ActiveCount = %d, want 0 — rejection must release the reserved slot", got)
	}
}

func TestShouldEvaluateSkipsTrivialTurns(t *testing.T) {
	g := testGuard(t, newFakeCounters(), 10, 5)

	cases := []struct {
		words int
		want  bool
	}{
		{0, false}, {1, false}, {14, false}, {15, true}, {200, true},
	}
	for _, tc := range cases {
		if got := g.ShouldEvaluate(tc.words); got != tc.want {
			t.Errorf("ShouldEvaluate(%d) = %v, want %v", tc.words, got, tc.want)
		}
	}
}

func TestConcurrentAcquireRespectsCap(t *testing.T) {
	// Judges clicking at once is the scenario this exists for.
	fc := newFakeCounters()
	const limit = 5
	g := testGuard(t, fc, limit, 1000)

	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if _, err := g.AcquireSession(context.Background(), uidFor(n)); err == nil {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if granted != limit {
		t.Errorf("granted %d concurrent sessions, want exactly %d", granted, limit)
	}
}

func uidFor(n int) string {
	return string(rune('a'+n%26)) + "-user"
}
