// Package difficulty is the adaptive engine: the band ladder and the concept
// coverage sets.
//
// Deliberately pure — no I/O, no model calls, no clock. This is the one
// component in the system with real logic and no external dependency, which
// makes it the one place where tests genuinely pay. Every rule below is
// exercised by a table-driven test asserting an exact band trajectory.
//
// Never ask a model to do this. Adaptation driven by an LLM's opinion of its own
// difficulty is unreproducible and cannot be explained to a user.
package difficulty

import (
	"fmt"
	"strings"

	"github.com/santh/crucible/internal/store"
)

// Band bounds (PRD §10.2).
const (
	// MinBand is 2, not 1. Demoting an adult with a real resume to definitional
	// questions is demoralising, and it makes the demo look easy.
	MinBand = 2
	MaxBand = 5

	// PromoteThreshold and DemoteThreshold act on the rolling score.
	PromoteThreshold = 7.5
	DemoteThreshold  = 4.0

	// ConsecutiveRequired is how many turns in a row must clear a threshold.
	// One strong answer is luck; two is a signal.
	ConsecutiveRequired = 2

	// CooldownTurns prevents oscillation. Without it a candidate hovering
	// around 7.5 gets promoted and demoted on alternate turns, which reads as
	// the system being confused rather than adaptive.
	CooldownTurns = 2

	// RollingWeightCurrent weights the latest turn against the previous one.
	// 0.6/0.4 responds quickly without letting a single bad answer undo a
	// established trend.
	RollingWeightCurrent  = 0.6
	RollingWeightPrevious = 0.4
)

// State is everything the engine needs to decide. Caller-owned and
// round-tripped through Firestore, so the engine itself holds nothing.
type State struct {
	Band           int
	TurnIndex      int
	LastScore      float64 // the previous turn's score, for the rolling average
	HasLastScore   bool
	StrongStreak   int // consecutive turns with rolling >= PromoteThreshold
	WeakStreak     int // consecutive turns with rolling <= DemoteThreshold
	LastChangeTurn int // turn index of the most recent band change
	Coverage       store.Coverage
}

// Decision is the outcome of one turn.
type Decision struct {
	FromBand int
	ToBand   int
	Changed  bool
	// Reason is shown to the user in a toast and stored in bandHistory.
	// Adaptation the user cannot perceive is worthless, so this must read as an
	// explanation rather than a log line.
	Reason       string
	RollingScore float64
	Coverage     store.Coverage
}

// Promoted reports whether the band went up.
func (d Decision) Promoted() bool { return d.Changed && d.ToBand > d.FromBand }

// Apply folds one graded turn into the state and returns the decision.
//
// The state is updated in place so the caller can persist it; the decision
// carries what the UI and the injection loop need.
func Apply(s *State, eval *store.Evaluation) Decision {
	if s.Band == 0 {
		s.Band = 3 // mid-level default
	}
	s.Band = clamp(s.Band, MinBand, MaxBand)

	d := Decision{FromBand: s.Band, ToBand: s.Band}
	if eval == nil {
		// An ungraded turn must not move the band in either direction. A
		// grader outage is not evidence about the candidate.
		d.Coverage = s.Coverage
		return d
	}

	s.TurnIndex++

	rolling := eval.TurnScore
	if s.HasLastScore {
		rolling = RollingWeightCurrent*eval.TurnScore + RollingWeightPrevious*s.LastScore
	}
	d.RollingScore = rolling

	s.LastScore = eval.TurnScore
	s.HasLastScore = true

	switch {
	case rolling >= PromoteThreshold:
		s.StrongStreak++
		s.WeakStreak = 0
	case rolling <= DemoteThreshold:
		s.WeakStreak++
		s.StrongStreak = 0
	default:
		// The middle band breaks both streaks: a mediocre turn is evidence
		// against a trend in either direction.
		s.StrongStreak = 0
		s.WeakStreak = 0
	}

	s.Coverage = MergeCoverage(s.Coverage, eval)
	d.Coverage = s.Coverage

	// Cooldown. Checked after the streaks update so evidence still accumulates
	// while a change is on hold.
	if s.TurnIndex-s.LastChangeTurn < CooldownTurns && s.LastChangeTurn > 0 {
		return d
	}

	switch {
	case s.StrongStreak >= ConsecutiveRequired && s.Band < MaxBand:
		s.Band++
		s.StrongStreak = 0
		s.LastChangeTurn = s.TurnIndex
		d.ToBand, d.Changed = s.Band, true
		d.Reason = "Difficulty raised — you've proven the fundamentals."

	case s.WeakStreak >= ConsecutiveRequired && s.Band > MinBand:
		s.Band--
		s.WeakStreak = 0
		s.LastChangeTurn = s.TurnIndex
		d.ToBand, d.Changed = s.Band, true
		d.Reason = "Easing off — let's rebuild from the mechanism."
	}

	return d
}

// MergeCoverage folds an evaluation's concepts into the running sets.
//
// Without coverage tracking an "adaptive" interview asks the same thing three
// times in different words, which users notice immediately.
func MergeCoverage(c store.Coverage, eval *store.Evaluation) store.Coverage {
	if eval == nil {
		return c
	}

	proven := newSet(c.Proven)
	shaky := newSet(c.Shaky)
	missing := newSet(c.Missing)

	// A concept demonstrated correctly is proven, and proving it clears any
	// earlier doubt — the candidate has since shown they know it.
	for _, concept := range eval.ConceptsDemonstrated {
		key := normalise(concept)
		if key == "" {
			continue
		}
		proven.add(key, concept)
		shaky.remove(key)
		missing.remove(key)
	}

	// An 'incomplete' span means attempted and partially correct — the precise
	// definition of shaky. Re-approach from a different angle, never repeat the
	// same question.
	for _, span := range eval.Spans {
		if span.Verdict != store.VerdictIncomplete || span.Concept == "" {
			continue
		}
		key := normalise(span.Concept)
		if proven.has(key) {
			continue // already demonstrated elsewhere; do not demote it
		}
		shaky.add(key, span.Concept)
		missing.remove(key)
	}

	// Missing concepts drive the roadmap. Anything already proven or shaky was
	// at least attempted, so it does not belong here.
	for _, concept := range eval.ConceptsMissing {
		key := normalise(concept)
		if key == "" || proven.has(key) || shaky.has(key) {
			continue
		}
		missing.add(key, concept)
	}

	return store.Coverage{
		Proven:  proven.values(),
		Shaky:   shaky.values(),
		Missing: missing.values(),
	}
}

// NextBandDescription renders the band for a user-facing toast.
func NextBandDescription(band int) string {
	names := map[int]string{
		1: "Orientation", 2: "Application", 3: "Mechanism",
		4: "Tradeoff", 5: "Adversarial",
	}
	name, ok := names[band]
	if !ok {
		name = "Mechanism"
	}
	return fmt.Sprintf("Band %d — %s", band, name)
}

// --- set helper -----------------------------------------------------------

// set preserves the original casing of the first spelling seen while comparing
// case-insensitively, so "Backpressure" and "backpressure" are one concept but
// the user still sees readable text.
type set struct {
	order []string
	byKey map[string]string
}

func newSet(items []string) *set {
	s := &set{byKey: make(map[string]string, len(items))}
	for _, item := range items {
		s.add(normalise(item), item)
	}
	return s
}

func (s *set) add(key, display string) {
	if key == "" {
		return
	}
	if _, exists := s.byKey[key]; exists {
		return
	}
	s.byKey[key] = display
	s.order = append(s.order, key)
}

func (s *set) remove(key string) {
	if _, exists := s.byKey[key]; !exists {
		return
	}
	delete(s.byKey, key)
	for i, k := range s.order {
		if k == key {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func (s *set) has(key string) bool {
	_, ok := s.byKey[key]
	return ok
}

// values returns display strings in insertion order. Never nil: a nil slice
// serialises to JSON null and forces a null check on the frontend.
func (s *set) values() []string {
	out := make([]string, 0, len(s.order))
	for _, k := range s.order {
		out = append(out, s.byKey[k])
	}
	return out
}

func normalise(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
