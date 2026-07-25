package grading

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/santh/crucible/internal/difficulty"
	"github.com/santh/crucible/internal/live"
	"github.com/santh/crucible/internal/persona"
	"github.com/santh/crucible/internal/prompts"
	"github.com/santh/crucible/internal/store"
)

// pendingInjection tracks one turn's race between the grader and the deadline.
//
// The central design problem of a speech-to-speech architecture: there is one
// long-lived session and the model decides what to say next on its own. A grade
// computed after turn 3 reaches turn 4 only by being injected as context
// between them — and the window for that is the couple of seconds the
// interviewer spends acknowledging the answer.
type pendingInjection struct {
	once sync.Once
	stop chan struct{}
}

// fire runs the injection exactly once, whichever path gets there first.
func (p *pendingInjection) fire(send func()) {
	p.once.Do(func() {
		close(p.stop)
		send()
	})
}

// armInjection starts the deadline timer for a turn (AD-3).
//
// Whichever arrives first wins:
//   - the evaluation, carrying the grader's followup_probe — the sharp path,
//     and the normal one;
//   - the deadline, carrying a question built from deterministic data we
//     already hold — duller, but the interviewer never sits in silence.
//
// The PRD's design has the conversation waiting on a model call. Under any
// retry, rate limit, or slow response that means a silent interviewer on stage.
// A deadline converts a hang into a slightly less pointed question, which
// nobody in the audience can detect.
func (s *Service) armInjection(sessionID string) *pendingInjection {
	p := &pendingInjection{stop: make(chan struct{})}

	go func() {
		select {
		case <-p.stop:
			// The grader won. Nothing to do.
		case <-time.After(s.cfg.InjectionDeadline):
			p.fire(func() {
				s.log.Info("injection deadline fired, using deterministic fallback",
					"session_id", sessionID,
					"deadline_ms", s.cfg.InjectionDeadline.Milliseconds())
				s.injectFallback(sessionID)
			})
		}
	}()

	return p
}

// injectFallback sends a coach state built entirely from data already in
// Firestore — no model call, so it cannot itself be slow.
func (s *Service) injectFallback(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := s.loadSession(ctx, sessionID)
	if err != nil {
		s.log.Warn("could not load session for fallback injection", "error", err.Error())
		return
	}

	question := nextPlannedQuestion(sess)
	if question == "" {
		// With no plan left, ask the interviewer to keep going on its own
		// judgement rather than injecting an empty directive.
		question = "Move to the next area of the interview plan, or probe deeper on the candidate's last answer."
	}

	s.inject(sessionID, sess, question, "")
}

// injectCoachState sends the graded coach state, including the grader's
// followup_probe.
//
// The probe is the point: the grader saw exactly where the answer thinned out,
// so its next question is sharper than anything the interviewer would
// improvise. This is where adaptation stops being a checkbox.
func (s *Service) injectCoachState(sessionID string, sess *store.Session, eval *store.Evaluation, d difficulty.Decision) {
	question := strings.TrimSpace(eval.FollowupProbe)
	if question == "" {
		question = nextPlannedQuestion(sess)
	}

	note := ""
	if d.Changed {
		// One verbal acknowledgement of a difficulty change, achieved by
		// injecting a hint into the state rather than by scripting the audio.
		if d.Promoted() {
			note = "The candidate has earned a harder question. You may acknowledge this in at most four words before asking."
		} else {
			note = "The candidate is struggling. Ask something more concrete. Do not mention that you are easing off."
		}
	}

	s.inject(sessionID, sess, question, note)
}

// inject renders the directive and pushes it into the live session.
func (s *Service) inject(sessionID string, sess *store.Session, question, bandNote string) {
	p, err := prompts.Get(prompts.InjectionState)
	if err != nil {
		s.log.Error("injection prompt unavailable", "error", err.Error())
		return
	}

	band := sess.DifficultyBand
	if band == 0 {
		band = 3
	}

	directive := p.Render(map[string]string{
		"BAND":             fmt.Sprint(band),
		"BAND_DESCRIPTION": persona.BandDescription(band),
		"BAND_NOTE":        bandNote,
		"CONCEPTS_PROVEN":  orNone(sess.Coverage.Proven),
		"CONCEPTS_SHAKY":   orNone(sess.Coverage.Shaky),
		"NEXT_QUESTION":    question,
	})

	if !s.relay.InjectCoachState(sessionID, directive) {
		// Normal when the user ended the session before their last turn
		// finished grading.
		s.log.Debug("no live session to inject into", "session_id", sessionID)
		return
	}
	s.log.Info("coach state injected",
		"session_id", sessionID, "band", band,
		"question_chars", len(question),
		"proven", len(sess.Coverage.Proven),
		"shaky", len(sess.Coverage.Shaky))
}

// nextPlannedQuestion returns the opening seed of the first plan area that has
// not been covered yet.
func nextPlannedQuestion(sess *store.Session) string {
	plan, ok := sess.Digest["interview_plan"].([]any)
	if !ok {
		return ""
	}
	for _, raw := range plan {
		area, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if dropped, _ := area["dropped"].(bool); dropped {
			continue
		}
		name, _ := area["area"].(string)
		if coveredAlready(sess.Coverage.Proven, name) {
			continue
		}
		if seed, _ := area["opening_question_seed"].(string); seed != "" {
			return seed
		}
	}
	return ""
}

// coveredAlready reports whether a plan area overlaps something already proven,
// so the fallback does not re-ask about ground the candidate has covered.
func coveredAlready(proven []string, area string) bool {
	if area == "" {
		return false
	}
	a := strings.ToLower(area)
	for _, p := range proven {
		if strings.Contains(a, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func orNone(items []string) string {
	if len(items) == 0 {
		return "none yet"
	}
	return strings.Join(items, ", ")
}

// ensure the live package stays referenced when this file is edited alone.
var _ = live.TypeBand
