// Package worker is the background job pool.
//
// A buffered channel and a handful of goroutines is the entire "queue
// infrastructure" this system needs (AD-1). Pub/Sub or Cloud Tasks would buy
// nothing at ten concurrent sessions and cost a whole class of deployment bug.
//
// The tradeoff that comes with that: jobs die with the instance. Every job type
// is therefore idempotent and re-drivable from Firestore state, so a restart
// costs a retry rather than a lost report.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// JobKind names the background work this system performs.
type JobKind string

const (
	// JobEvaluateTurn grades one answer. Never blocks the conversation.
	JobEvaluateTurn JobKind = "evaluate_turn"
	// JobDeliveryMetrics analyses answer audio for pace and disfluency.
	JobDeliveryMetrics JobKind = "delivery_metrics"
	// JobFinalize aggregates a completed session into a report.
	JobFinalize JobKind = "finalize"
	// JobBuildRoadmap turns missing concepts into a day-by-day plan.
	JobBuildRoadmap JobKind = "build_roadmap"
)

// Job is one unit of background work.
type Job struct {
	Kind      JobKind
	SessionID string
	TurnID    string

	// Payload carries job-specific data. Kept generic so later phases add job
	// types without touching the pool.
	Payload any

	// enqueuedAt measures queue wait, which is the first thing to look at when
	// evaluations start feeling slow.
	enqueuedAt time.Time
	attempt    int
}

// Handler processes one job. Returning an error triggers a bounded retry.
type Handler func(ctx context.Context, job Job) error

// Pool runs jobs across a fixed set of goroutines.
type Pool struct {
	log      *slog.Logger
	cfg      Config
	jobs     chan Job
	handlers map[JobKind]Handler

	wg   sync.WaitGroup
	once sync.Once
}

// Config tunes the pool.
type Config struct {
	// Workers is the number of concurrent goroutines. Jobs are all
	// network-bound Vertex calls, so this is about API concurrency rather
	// than CPU.
	Workers int
	// QueueSize bounds the backlog. Deep enough to absorb a burst of turn
	// completions, shallow enough that a stuck downstream is visible.
	QueueSize int
	// MaxAttempts bounds retries per job.
	MaxAttempts int
}

// DefaultConfig is sized for the PRD's ten concurrent sessions.
func DefaultConfig() Config {
	return Config{Workers: 8, QueueSize: 128, MaxAttempts: 3}
}

// NewPool builds an unstarted pool.
func NewPool(log *slog.Logger, cfg Config) *Pool {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultConfig().Workers
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = DefaultConfig().QueueSize
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = DefaultConfig().MaxAttempts
	}
	p := &Pool{
		log:      log,
		jobs:     make(chan Job, cfg.QueueSize),
		handlers: make(map[JobKind]Handler),
	}
	p.cfg = cfg
	return p
}

// Register wires a handler for a job kind. Call before Start.
func (p *Pool) Register(kind JobKind, h Handler) {
	p.handlers[kind] = h
}

// Start launches the workers.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.cfg.Workers; i++ {
		p.wg.Add(1)
		go p.run(ctx, i)
	}
	p.log.Info("worker pool started", "workers", p.cfg.Workers, "queue", cap(p.jobs))
}

// Submit queues a job without blocking.
//
// A full queue drops the job rather than stalling the caller. The caller is a
// live interview: blocking it to wait for a grader would invert the whole
// design principle that the conversation never waits on enrichment.
func (p *Pool) Submit(job Job) error {
	job.enqueuedAt = time.Now()
	select {
	case p.jobs <- job:
		return nil
	default:
		p.log.Error("job queue full, dropping",
			"kind", job.Kind, "session_id", job.SessionID, "queued", len(p.jobs))
		return fmt.Errorf("worker: queue full, dropped %s job", job.Kind)
	}
}

// Stop drains in-flight work and waits for the workers to exit.
func (p *Pool) Stop() {
	p.once.Do(func() { close(p.jobs) })
	p.wg.Wait()
	p.log.Info("worker pool stopped")
}

func (p *Pool) run(ctx context.Context, id int) {
	defer p.wg.Done()

	for job := range p.jobs {
		h, ok := p.handlers[job.Kind]
		if !ok {
			p.log.Error("no handler registered", "kind", job.Kind)
			continue
		}

		waited := time.Since(job.enqueuedAt)
		started := time.Now()
		err := h(ctx, job)
		duration := time.Since(started)

		if err == nil {
			p.log.Info("job complete",
				"kind", job.Kind,
				"session_id", job.SessionID,
				"turn_id", job.TurnID,
				"queue_wait_ms", waited.Milliseconds(),
				"duration_ms", duration.Milliseconds(),
				"worker", id)
			continue
		}

		job.attempt++
		if job.attempt < p.cfg.MaxAttempts && ctx.Err() == nil {
			p.log.Warn("job failed, retrying",
				"kind", job.Kind, "session_id", job.SessionID,
				"attempt", job.attempt, "error", err.Error())
			// Re-submit rather than retrying inline so one poisoned job cannot
			// monopolise a worker while other sessions wait.
			job.enqueuedAt = time.Now()
			select {
			case p.jobs <- job:
			default:
				p.log.Error("could not requeue job", "kind", job.Kind, "session_id", job.SessionID)
			}
			continue
		}

		// Terminal failure. The handler is responsible for recording the
		// degraded state — a failed grade marks the turn UNGRADED and the
		// interview carries on. The conversation is the product; everything
		// else is enrichment.
		p.log.Error("job failed permanently",
			"kind", job.Kind, "session_id", job.SessionID, "turn_id", job.TurnID,
			"attempts", job.attempt, "error", err.Error())
	}
}
