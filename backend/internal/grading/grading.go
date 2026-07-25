// Package grading connects a finished turn to its evaluation.
//
// It is the TurnSink the relay talks to, and the worker handler that grades.
// Keeping both here means the relay never imports Firestore, the evaluator, or
// the worker pool.
//
// The governing rule: the conversation is the product and everything here is
// enrichment. Every failure path degrades to "the interview keeps working".
package grading

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/santh/crucible/internal/audio"
	"github.com/santh/crucible/internal/blob"
	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/delivery"
	"github.com/santh/crucible/internal/evaluator"
	"github.com/santh/crucible/internal/guardrails"
	"github.com/santh/crucible/internal/live"
	"github.com/santh/crucible/internal/persona"
	"github.com/santh/crucible/internal/roadmap"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/study"
	"github.com/santh/crucible/internal/vertexai"
	"github.com/santh/crucible/internal/worker"
)

// Service wires turn closure to persistence and grading.
type Service struct {
	cfg      *config.Config
	log      *slog.Logger
	store    *store.Store
	blob     *blob.Store
	eval     *evaluator.Evaluator
	pool     *worker.Pool
	guard    *guardrails.Guard
	relay    *live.Relay
	vx       *vertexai.Client
	delivery *delivery.Analyser
	roadmap  *roadmap.Builder
	study    *study.Decomposer

	// pending tracks each session's in-flight injection race, so the grader
	// and the deadline timer cannot both fire.
	pendingMu sync.Mutex
	pending   map[string]*pendingInjection
}

// New builds the service and registers its worker handler.
func New(cfg *config.Config, log *slog.Logger, st *store.Store, bl *blob.Store,
	ev *evaluator.Evaluator, pool *worker.Pool, guard *guardrails.Guard,
	relay *live.Relay, vx *vertexai.Client, dl *delivery.Analyser,
	rm *roadmap.Builder, sd *study.Decomposer) *Service {

	s := &Service{
		cfg: cfg, log: log, store: st, blob: bl, eval: ev,
		pool: pool, guard: guard, relay: relay, vx: vx, delivery: dl, roadmap: rm, study: sd,
		pending: make(map[string]*pendingInjection),
	}
	pool.Register(worker.JobEvaluateTurn, s.handleEvaluate)
	pool.Register(worker.JobDeliveryMetrics, s.handleDelivery)
	pool.Register(worker.JobFinalize, s.handleFinalize)
	pool.Register(worker.JobBuildRoadmap, s.handleRoadmap)
	relay.SetTurnSink(s)
	relay.SetHintProvider(s)
	return s
}

// TurnClosed persists a finished turn and queues it for grading.
//
// Runs on its own goroutine, off the conversation's path. Nothing here is
// allowed to make the interviewer wait.
func (s *Service) TurnClosed(ctx context.Context, sessionID string, t *store.Turn, pcm []byte) {
	log := s.log.With("session_id", sessionID, "turn_index", t.Index)

	// Upload the answer audio first so the turn document can reference it.
	// Best-effort: losing the audio costs the delivery metrics, not the grade.
	if len(pcm) > 0 {
		t.AudioDurationMs = audio.Duration(pcm, audio.SampleRateIn)
		var buf bytes.Buffer
		if err := audio.WriteWAV(&buf, pcm, audio.SampleRateIn); err != nil {
			log.Warn("could not encode turn audio", "error", err.Error())
		} else {
			path := blob.AudioPath(sessionID, fmt.Sprintf("t%d", t.Index))
			uri, err := s.blob.Upload(ctx, path, "audio/wav", &buf, int64(buf.Len())+1)
			if err != nil {
				log.Warn("could not upload turn audio", "error", err.Error())
			} else {
				t.AudioGCSURI = uri
			}
		}
	}

	// Skip grading answers too short to carry signal. Grading "yes" or "can you
	// repeat that" costs a model call and returns noise.
	words := len(splitWords(t.UserTranscript))
	if !s.guard.ShouldEvaluate(words) {
		t.GradingStatus = store.GradingSkipped
		log.Info("skipping evaluation, answer too short", "words", words, "min", s.cfg.EvalMinWords)
	}

	saved, err := s.store.CreateTurn(ctx, sessionID, t)
	if err != nil {
		log.Error("could not persist turn", "error", err.Error())
		return
	}

	if t.GradingStatus == store.GradingSkipped {
		s.relay.Publish(sessionID, live.ServerFrame{
			Type: live.TypeUngraded, TurnID: saved.ID,
			Message: "Too short to grade.",
		})
		return
	}

	// Start the injection race now, alongside the grading job. Whichever
	// finishes first drives the next question (AD-3).
	s.pendingMu.Lock()
	s.pending[sessionID] = s.armInjection(sessionID)
	s.pendingMu.Unlock()

	if err := s.pool.Submit(worker.Job{
		Kind:      worker.JobEvaluateTurn,
		SessionID: sessionID,
		TurnID:    saved.ID,
	}); err != nil {
		// A full queue must not silently swallow the turn: tell the user it
		// went ungraded rather than leaving a shimmer spinning forever.
		log.Error("could not queue evaluation", "error", err.Error())
		s.markUngraded(ctx, sessionID, saved.ID, "the grader was busy")
	}
}

// handleEvaluate is the worker handler. Returning an error triggers the pool's
// bounded retry; the final failure is recorded on the turn.
func (s *Service) handleEvaluate(ctx context.Context, job worker.Job) error {
	log := s.log.With("session_id", job.SessionID, "turn_id", job.TurnID)

	t, err := s.store.GetTurn(ctx, job.SessionID, job.TurnID)
	if err != nil {
		return fmt.Errorf("loading turn: %w", err)
	}
	// Idempotent: a retry after a partial success must not re-grade and
	// re-charge for a turn that already has a result.
	if t.GradingStatus == store.GradingComplete {
		return nil
	}

	sess, err := s.loadSession(ctx, job.SessionID)
	if err != nil {
		return fmt.Errorf("loading session: %w", err)
	}

	roleTitle, seniority := persona.RoleFrom(sess.Digest)

	// Attribute the grading tokens to this session.
	ctx = vertexai.WithSession(ctx, job.SessionID)

	eval, err := s.eval.Evaluate(ctx, evaluator.Input{
		TurnID:      job.TurnID,
		Question:    t.QuestionText,
		Transcript:  t.UserTranscript,
		RoleTitle:   roleTitle,
		Seniority:   seniority,
		Band:        sess.DifficultyBand,
		Persona:     sess.Persona,
		DomainVocab: domainVocab(sess.Digest),
	})
	if err != nil {
		return err
	}

	// Hints cost half a point each (PRD §10.2). Applied here rather than in
	// the evaluator so the model never sees, and cannot be influenced by, how
	// much help the candidate took.
	eval.TurnScore -= 0.5 * float64(t.HintsUsed)
	if eval.TurnScore < 0 {
		eval.TurnScore = 0
	}

	if err := s.store.UpdateTurn(ctx, job.SessionID, job.TurnID, map[string]any{
		"evaluation":    eval,
		"gradingStatus": string(store.GradingComplete),
	}); err != nil {
		return fmt.Errorf("saving evaluation: %w", err)
	}

	// Fold the grade into the band and coverage sets before injecting, so the
	// directive carries the NEW state rather than the pre-turn one.
	decision := s.adapt(ctx, job.SessionID, sess, eval)

	// Win the injection race if the deadline has not already fired.
	s.pendingMu.Lock()
	p := s.pending[job.SessionID]
	delete(s.pending, job.SessionID)
	s.pendingMu.Unlock()
	if p != nil {
		p.fire(func() { s.injectCoachState(job.SessionID, sess, eval, decision) })
	}

	// Push to the live session if it is still open. A closed session is normal
	// rather than an error — the result is persisted for the report either way.
	delivered := s.relay.Publish(job.SessionID, live.ServerFrame{
		Type:    live.TypeEvaluation,
		TurnID:  job.TurnID,
		Payload: eval,
	})

	// Delivery analysis runs after grading and only when audio exists: a typed
	// answer has nothing to listen to.
	if t.AudioGCSURI != "" {
		s.QueueDelivery(job.SessionID, job.TurnID)
	}

	log.Info("evaluation delivered",
		"turn_score", fmt.Sprintf("%.2f", eval.TurnScore),
		"spans", len(eval.Spans),
		"delivered_live", delivered)
	return nil
}

// markUngraded records that a turn could not be graded and tells the client, so
// the interview visibly continues instead of stalling.
func (s *Service) markUngraded(ctx context.Context, sessionID, turnID, reason string) {
	_ = s.store.UpdateTurn(ctx, sessionID, turnID, map[string]any{
		"gradingStatus": string(store.GradingFailed),
		"gradingError":  reason,
	})
	s.relay.Publish(sessionID, live.ServerFrame{
		Type: live.TypeUngraded, TurnID: turnID,
		Message: "We couldn't grade this one — the interview continues.",
	})
}

// loadSession reads a session without an ownership check.
//
// Safe because the caller is a worker acting on a job the relay created, and
// the relay verified ownership before opening the socket. Exposing this to a
// request path would be a bug.
func (s *Service) loadSession(ctx context.Context, sessionID string) (*store.Session, error) {
	snap, err := s.store.Raw().Collection("sessions").Doc(sessionID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var sess store.Session
	if err := snap.DataTo(&sess); err != nil {
		return nil, err
	}
	sess.ID = snap.Ref.ID
	return &sess, nil
}

// domainVocab pulls the candidate's stack and the role's areas out of the
// digest.
//
// This is what lets the evaluator read "blue filter" as "bloom filter". PRD
// §12.4 calls it out as the single instruction that resolves most false
// positives, and it only works if the vocabulary actually reaches the prompt.
func domainVocab(digest map[string]any) []string {
	var out []string
	if cand, ok := digest["candidate"].(map[string]any); ok {
		out = append(out, stringSlice(cand["primary_stack"])...)
	}
	if role, ok := digest["role"].(map[string]any); ok {
		out = append(out, stringSlice(role["domain_areas"])...)
		out = append(out, stringSlice(role["must_haves"])...)
	}
	return out
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func splitWords(s string) []string {
	var out []string
	start := -1
	for i := 0; i <= len(s); i++ {
		atEnd := i == len(s)
		isSpace := !atEnd && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r')
		if !atEnd && !isSpace {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			out = append(out, s[start:i])
			start = -1
		}
	}
	return out
}
