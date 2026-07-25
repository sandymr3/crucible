package grading

import (
	"context"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/santh/crucible/internal/difficulty"
	"github.com/santh/crucible/internal/live"
	"github.com/santh/crucible/internal/prompts"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/vertexai"
)

// adapt folds a graded turn into the session's band and coverage, persists the
// result, and tells the client.
//
// Runs after the evaluation is saved so a failure here costs the adaptation,
// never the grade.
func (s *Service) adapt(ctx context.Context, sessionID string, sess *store.Session, eval *store.Evaluation) difficulty.Decision {
	// Rehydrate the FULL engine state from Firestore, streaks included.
	//
	// Firestore is the authority (AD-5) and the worker grading this turn may be
	// on a different instance from the one that graded the last. Rebuilding a
	// partial state — as this originally did, carrying band and coverage but
	// not the streaks — silently disables adaptation: the streak resets each
	// turn and never reaches the two consecutive turns a promotion needs.
	state := &difficulty.State{
		Band:           sess.DifficultyBand,
		TurnIndex:      sess.Adapt.TurnIndex,
		LastScore:      sess.Adapt.LastScore,
		HasLastScore:   sess.Adapt.HasLastScore,
		StrongStreak:   sess.Adapt.StrongStreak,
		WeakStreak:     sess.Adapt.WeakStreak,
		LastChangeTurn: sess.Adapt.LastChangeTurn,
		Coverage:       sess.Coverage,
	}

	d := difficulty.Apply(state, eval)

	fields := map[string]any{
		"coverage.proven":      state.Coverage.Proven,
		"coverage.shaky":       state.Coverage.Shaky,
		"coverage.missing":     state.Coverage.Missing,
		"adapt.lastScore":      state.LastScore,
		"adapt.hasLastScore":   state.HasLastScore,
		"adapt.strongStreak":   state.StrongStreak,
		"adapt.weakStreak":     state.WeakStreak,
		"adapt.lastChangeTurn": state.LastChangeTurn,
		"adapt.turnIndex":      state.TurnIndex,
	}
	if d.Changed {
		fields["difficultyBand"] = state.Band
		fields["bandHistory"] = append(sess.BandHistory, store.BandChange{
			TurnIndex: state.TurnIndex,
			Band:      state.Band,
			Reason:    d.Reason,
			At:        time.Now(),
		})
	}

	if err := s.store.UpdateSessionUnchecked(ctx, sessionID, fields); err != nil {
		s.log.Warn("could not persist adaptation", "session_id", sessionID, "error", err.Error())
	}

	// Keep the caller's copy in step so the injection that follows sees the
	// new band and coverage rather than the pre-turn values.
	sess.DifficultyBand = state.Band
	sess.Coverage = state.Coverage
	sess.Adapt = store.AdaptState{
		LastScore:      state.LastScore,
		HasLastScore:   state.HasLastScore,
		StrongStreak:   state.StrongStreak,
		WeakStreak:     state.WeakStreak,
		LastChangeTurn: state.LastChangeTurn,
		TurnIndex:      state.TurnIndex,
	}

	if d.Changed {
		s.log.Info("difficulty band changed",
			"session_id", sessionID,
			"from", d.FromBand, "to", d.ToBand,
			"rolling", d.RollingScore, "reason", d.Reason)

		s.relay.Publish(sessionID, live.ServerFrame{
			Type:    live.TypeBand,
			From:    d.FromBand,
			To:      d.ToBand,
			Message: d.Reason,
			Text:    difficulty.NextBandDescription(d.ToBand),
		})
	}

	return d
}

// Hint generates a Socratic nudge for a turn in progress.
//
// Uses the cheap model: this is a small, latency-sensitive task and the
// candidate is sitting in silence waiting for it.
func (s *Service) Hint(ctx context.Context, sessionID, question, partial string) (string, error) {
	p, err := prompts.Get(prompts.HintSocratic)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(partial) == "" {
		partial = "(they have not started answering yet)"
	}

	instruction := p.Render(map[string]string{
		"QUESTION":       question,
		"PARTIAL_ANSWER": partial,
	})

	ctx, cancel := context.WithTimeout(vertexai.WithSession(ctx, sessionID), 20*time.Second)
	defer cancel()

	// Higher temperature than grading: a hint should feel like a person
	// thinking, and there is no single right phrasing.
	temp := float32(0.6)
	maxTokens := int32(120)

	text, err := s.vx.GenerateText(ctx, s.cfg.ModelCheap, instruction, &genai.GenerateContentConfig{
		Temperature:     &temp,
		MaxOutputTokens: maxTokens,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(text), `"`)), nil
}
