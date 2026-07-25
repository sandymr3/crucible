// Package guardrails enforces the credit protections from PRD §21.2.
//
// Every one of these is server-side, because a client-enforced cap is not a
// cap. Live audio is the dominant cost in this system by a wide margin, and the
// failure mode is not a spike — it is a forgotten open tab quietly draining
// credits for an hour.
package guardrails

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/store"
)

// Rejection reasons, distinguished so the client can say something useful
// rather than a generic "try again".
var (
	// ErrDailyCapReached means this user has started too many sessions today.
	ErrDailyCapReached = errors.New("guardrails: daily session cap reached")
	// ErrTooManyConcurrent means the service is at capacity.
	ErrTooManyConcurrent = errors.New("guardrails: too many concurrent sessions")
	// ErrAlreadyLive means this user already has a live session open.
	ErrAlreadyLive = errors.New("guardrails: user already has a live session")
)

// Counters is the persistence this package needs.
//
// An interface rather than *store.Store so the cap logic can be tested without
// a Firestore emulator. These rules decide whether the demo budget survives, so
// they need real tests rather than hopeful ones.
type Counters interface {
	// CountActiveSessions reports how many of a user's sessions are live.
	CountActiveSessions(ctx context.Context, uid string) (int, error)
	// IncrementDailySessions atomically consumes one daily allocation.
	IncrementDailySessions(ctx context.Context, uid string, cap int) (allowed bool, count int, err error)
}

// Ensure the real store satisfies the interface.
var _ Counters = (*store.Store)(nil)

// Guard holds the runtime counters.
type Guard struct {
	cfg   *config.Config
	log   *slog.Logger
	store Counters

	// concurrent is process-local. With min-instances=1 and max-instances=5
	// this under-counts across instances, so it is a backstop rather than the
	// whole story — the per-user checks below are the ones that actually bound
	// spend, because they are transactional in Firestore.
	concurrent atomic.Int32
}

// New builds the guard.
func New(cfg *config.Config, log *slog.Logger, st Counters) *Guard {
	return &Guard{cfg: cfg, log: log, store: st}
}

// AcquireSession checks every precondition for starting a live session and
// reserves a concurrency slot.
//
// The returned release function MUST be called when the session ends. Callers
// should defer it immediately.
func (g *Guard) AcquireSession(ctx context.Context, uid string) (release func(), err error) {
	// Cheapest check first: a process-local counter costs nothing, so reject an
	// overloaded instance before spending Firestore reads on it.
	n := g.concurrent.Add(1)
	if int(n) > g.cfg.MaxConcurrent {
		g.concurrent.Add(-1)
		g.log.Warn("rejecting session: concurrency cap",
			"active", n-1, "cap", g.cfg.MaxConcurrent, "uid", uid)
		return nil, ErrTooManyConcurrent
	}

	// One live session per user. Two open tabs would otherwise double this
	// user's burn rate for no benefit, and the second is almost always an
	// accident.
	active, err := g.store.CountActiveSessions(ctx, uid)
	if err != nil {
		// Fail open on an infrastructure error rather than blocking a
		// legitimate interview: the hard duration and idle caps still bound
		// the damage, and a demo that refuses to start is worse than one that
		// costs slightly more.
		g.log.Error("could not count active sessions, allowing", "error", err.Error())
	} else if active > 0 {
		g.concurrent.Add(-1)
		return nil, ErrAlreadyLive
	}

	var released atomic.Bool
	return func() {
		if released.CompareAndSwap(false, true) {
			g.concurrent.Add(-1)
		}
	}, nil
}

// CheckDailyCap atomically consumes one of the user's daily session
// allocations. Called at session creation, not at connect, so a user cannot
// burn their quota by reloading a page.
func (g *Guard) CheckDailyCap(ctx context.Context, uid string) error {
	allowed, count, err := g.store.IncrementDailySessions(ctx, uid, g.cfg.DailySessionCap)
	if err != nil {
		// Fail open, same reasoning as above: the per-session duration cap is
		// the real backstop.
		g.log.Error("daily cap check failed, allowing", "error", err.Error(), "uid", uid)
		return nil
	}
	if !allowed {
		g.log.Warn("rejecting session: daily cap",
			"uid", uid, "count", count, "cap", g.cfg.DailySessionCap)
		return fmt.Errorf("%w (%d of %d today)", ErrDailyCapReached, count, g.cfg.DailySessionCap)
	}
	return nil
}

// ActiveCount reports the number of live sessions on this instance.
func (g *Guard) ActiveCount() int { return int(g.concurrent.Load()) }

// ShouldEvaluate reports whether a turn carries enough signal to be worth
// grading. Answers below the threshold cost a model call and return noise.
func (g *Guard) ShouldEvaluate(wordCount int) bool {
	return wordCount >= g.cfg.EvalMinWords
}
