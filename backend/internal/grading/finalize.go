package grading

import (
	"context"
	"fmt"
	"time"

	"github.com/santh/crucible/internal/blob"
	"github.com/santh/crucible/internal/delivery"
	"github.com/santh/crucible/internal/report"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/vertexai"
	"github.com/santh/crucible/internal/worker"
)

// handleDelivery analyses one turn's answer audio.
//
// Separate from evaluation on purpose: it is slower, less important, and must
// never delay the heatmap. A turn can be fully graded with no delivery metrics
// at all.
func (s *Service) handleDelivery(ctx context.Context, job worker.Job) error {
	t, err := s.store.GetTurn(ctx, job.SessionID, job.TurnID)
	if err != nil {
		return fmt.Errorf("loading turn: %w", err)
	}
	if t.Delivery != nil {
		return nil // idempotent
	}

	ctx = vertexai.WithSession(ctx, job.SessionID)

	d, analysisErr := s.delivery.Analyse(ctx, delivery.Input{
		TurnID:          job.TurnID,
		Transcript:      t.UserTranscript,
		AudioURI:        t.AudioGCSURI,
		AudioDurationMs: t.AudioDurationMs,
	})
	// Analyse degrades rather than failing: even on error it returns the
	// deterministic metrics, which are still true. Persist them either way.
	if err := s.store.UpdateTurn(ctx, job.SessionID, job.TurnID, map[string]any{
		"delivery": d,
	}); err != nil {
		return fmt.Errorf("saving delivery: %w", err)
	}

	if analysisErr != nil {
		s.log.Warn("delivery audio analysis degraded to deterministic metrics",
			"session_id", job.SessionID, "turn_id", job.TurnID, "error", analysisErr.Error())
	}
	return nil
}

// handleFinalize aggregates a completed session into its report.
//
// Idempotent and re-drivable from Firestore alone (AD-5): if the instance dies
// mid-finalization, the next attempt reads the same turns and produces the same
// report. That is what lets the whole background pipeline live in-process
// without an external queue.
func (s *Service) handleFinalize(ctx context.Context, job worker.Job) error {
	sess, err := s.loadSession(ctx, job.SessionID)
	if err != nil {
		return fmt.Errorf("loading session: %w", err)
	}

	turns, err := s.store.ListTurns(ctx, job.SessionID)
	if err != nil {
		return fmt.Errorf("listing turns: %w", err)
	}

	// Wait briefly for in-flight grading so the report is not built from a
	// half-graded session. Bounded: a stuck grader must not block the report
	// forever, and an ungraded turn renders honestly as "couldn't grade this
	// one" rather than as a missing row.
	turns = s.awaitPendingGrades(ctx, job.SessionID, turns)

	r := report.Build(sess, turns)
	if err := s.store.PutReport(ctx, job.SessionID, r); err != nil {
		return fmt.Errorf("saving report: %w", err)
	}

	s.log.Info("report generated",
		"session_id", job.SessionID,
		"turns_graded", r.TurnsGraded,
		"overall", r.OverallScore,
		"domains", len(r.DomainScores),
		"gaps", len(r.Gaps),
		"fillers", r.Delivery.FillerTotal,
		"wpm", r.Delivery.WPM)

	// The roadmap depends on the report's gaps, so it is queued only once the
	// report exists.
	if err := s.pool.Submit(worker.Job{
		Kind:      worker.JobBuildRoadmap,
		SessionID: job.SessionID,
	}); err != nil {
		s.log.Warn("could not queue roadmap", "session_id", job.SessionID, "error", err.Error())
	}
	return nil
}

// gradeWaitTimeout bounds how long finalization waits for outstanding grades.
const (
	gradeWaitTimeout  = 45 * time.Second
	gradeWaitInterval = 3 * time.Second
)

// awaitPendingGrades re-reads turns until none are still pending, or the wait
// budget runs out.
func (s *Service) awaitPendingGrades(ctx context.Context, sessionID string, turns []*store.Turn) []*store.Turn {
	deadline := time.Now().Add(gradeWaitTimeout)

	for time.Now().Before(deadline) {
		pending := 0
		for _, t := range turns {
			if t.GradingStatus == store.GradingPending {
				pending++
			}
		}
		if pending == 0 {
			return turns
		}

		s.log.Info("finalization waiting on grades",
			"session_id", sessionID, "pending", pending)

		select {
		case <-ctx.Done():
			return turns
		case <-time.After(gradeWaitInterval):
		}

		refreshed, err := s.store.ListTurns(ctx, sessionID)
		if err != nil {
			return turns
		}
		turns = refreshed
	}
	return turns
}

// handleRoadmap turns the session's gaps into a study plan.
//
// Queued by handleFinalize once the report exists, because the roadmap is built
// from the same gap analysis and there is no point computing it twice.
func (s *Service) handleRoadmap(ctx context.Context, job worker.Job) error {
	sess, err := s.loadSession(ctx, job.SessionID)
	if err != nil {
		return fmt.Errorf("loading session: %w", err)
	}
	turns, err := s.store.ListTurns(ctx, job.SessionID)
	if err != nil {
		return fmt.Errorf("listing turns: %w", err)
	}

	ctx = vertexai.WithSession(ctx, job.SessionID)

	plan, err := s.roadmap.Build(ctx, sess, turns, s.cfg.RoadmapHorizonDays)
	if err != nil {
		return err
	}
	if err := s.store.PutRoadmap(ctx, job.SessionID, plan); err != nil {
		return fmt.Errorf("saving roadmap: %w", err)
	}

	s.log.Info("roadmap saved",
		"session_id", job.SessionID, "days", len(plan.Days),
		"grounded", plan.Grounded, "links", plan.LinksFound)
	return nil
}

// Finalize queues report generation for a session. Called from the /end handler,
// which returns immediately; the client polls GET /report.
func (s *Service) Finalize(sessionID string) error {
	return s.pool.Submit(worker.Job{Kind: worker.JobFinalize, SessionID: sessionID})
}

// QueueDelivery queues audio analysis for a turn.
func (s *Service) QueueDelivery(sessionID, turnID string) {
	if err := s.pool.Submit(worker.Job{
		Kind: worker.JobDeliveryMetrics, SessionID: sessionID, TurnID: turnID,
	}); err != nil {
		s.log.Warn("could not queue delivery analysis",
			"session_id", sessionID, "turn_id", turnID, "error", err.Error())
	}
}

var _ = blob.AudioPath
