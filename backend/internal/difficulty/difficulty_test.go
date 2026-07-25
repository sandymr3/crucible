package difficulty

import (
	"testing"

	"github.com/santh/crucible/internal/store"
)

func evalScoring(score float64) *store.Evaluation {
	return &store.Evaluation{TurnScore: score}
}

// drive runs a sequence of turn scores and returns the band after each.
func drive(s *State, scores ...float64) []int {
	bands := make([]int, 0, len(scores))
	for _, sc := range scores {
		Apply(s, evalScoring(sc))
		bands = append(bands, s.Band)
	}
	return bands
}

// The headline behaviour: two strong answers raise the band, and the demo
// depends on it happening within three turns.
func TestTwoStrongAnswersPromote(t *testing.T) {
	s := &State{Band: 3}

	got := drive(s, 9.0, 9.0)
	if want := []int{3, 4}; !equal(got, want) {
		t.Errorf("band trajectory = %v, want %v", got, want)
	}
}

func TestTwoWeakAnswersDemote(t *testing.T) {
	s := &State{Band: 4}

	got := drive(s, 2.0, 2.0)
	if want := []int{4, 3}; !equal(got, want) {
		t.Errorf("band trajectory = %v, want %v", got, want)
	}
}

// One strong answer is luck, not evidence.
func TestSingleStrongAnswerDoesNotPromote(t *testing.T) {
	s := &State{Band: 3}

	if got := drive(s, 9.5); got[0] != 3 {
		t.Errorf("band = %d after one strong answer, want it unchanged at 3", got[0])
	}
}

// A mediocre turn between two strong ones breaks the streak. Otherwise the
// band climbs on evidence that was never consecutive.
func TestMediocreTurnBreaksTheStreak(t *testing.T) {
	s := &State{Band: 3}

	got := drive(s, 9.0, 5.5, 9.0)
	for i, band := range got {
		if band != 3 {
			t.Errorf("turn %d: band = %d, want 3 — the streak should have been broken", i, band)
		}
	}
}

// The rule that keeps a borderline candidate from being promoted and demoted on
// alternate turns, which reads as confusion rather than adaptation.
func TestCooldownPreventsOscillation(t *testing.T) {
	s := &State{Band: 3}

	// Promote on turn 2.
	drive(s, 9.0, 9.0)
	if s.Band != 4 {
		t.Fatalf("setup failed: band = %d, want 4", s.Band)
	}

	// Two weak turns immediately after. The second one falls inside the
	// cooldown window and must not demote.
	got := drive(s, 1.0, 1.0)
	if got[0] != 4 || got[1] != 4 {
		t.Errorf("band moved to %v during cooldown, want it held at 4", got)
	}

	// Once the cooldown has passed, the accumulated weak evidence applies.
	if final := drive(s, 1.0); final[0] != 3 {
		t.Errorf("band = %d after the cooldown expired, want 3", final[0])
	}
}

// Band 1 is deliberately unreachable: demoting an adult with a real resume to
// definitional questions is demoralising and makes the demo look easy.
func TestNeverDemotesBelowBandTwo(t *testing.T) {
	s := &State{Band: 3}

	for i := 0; i < 12; i++ {
		Apply(s, evalScoring(1.0))
	}
	if s.Band != MinBand {
		t.Errorf("band bottomed out at %d, want %d", s.Band, MinBand)
	}
}

func TestNeverPromotesAboveBandFive(t *testing.T) {
	s := &State{Band: 3}

	for i := 0; i < 12; i++ {
		Apply(s, evalScoring(10.0))
	}
	if s.Band != MaxBand {
		t.Errorf("band topped out at %d, want %d", s.Band, MaxBand)
	}
}

// The rolling average is what smooths a single outlier. A 10 after a 2 must not
// promote on its own.
func TestRollingAverageDampensOutliers(t *testing.T) {
	s := &State{Band: 3}

	// 2.0 then 10.0 -> rolling = 0.6*10 + 0.4*2 = 6.8, below the 7.5 threshold.
	drive(s, 2.0, 10.0)
	if s.StrongStreak != 0 {
		t.Errorf("StrongStreak = %d after a single outlier, want 0", s.StrongStreak)
	}
	if s.Band != 3 {
		t.Errorf("band = %d, want 3", s.Band)
	}
}

// A grader outage is not evidence about the candidate.
func TestUngradedTurnDoesNotMoveTheBand(t *testing.T) {
	s := &State{Band: 3}
	drive(s, 9.0) // one strong turn banked

	before := *s
	d := Apply(s, nil)

	if d.Changed {
		t.Error("an ungraded turn changed the band")
	}
	if s.StrongStreak != before.StrongStreak || s.TurnIndex != before.TurnIndex {
		t.Error("an ungraded turn mutated the streak or turn index")
	}
}

func TestDecisionCarriesAReasonOnChange(t *testing.T) {
	s := &State{Band: 3}
	Apply(s, evalScoring(9.0))
	d := Apply(s, evalScoring(9.0))

	if !d.Changed || !d.Promoted() {
		t.Fatalf("expected a promotion, got %+v", d)
	}
	if d.Reason == "" {
		t.Error("a band change carries no reason; the user-facing toast would be blank")
	}
	if d.FromBand != 3 || d.ToBand != 4 {
		t.Errorf("decision = %d->%d, want 3->4", d.FromBand, d.ToBand)
	}
}

// --- Coverage -------------------------------------------------------------

func TestDemonstratedConceptsBecomeProven(t *testing.T) {
	got := MergeCoverage(store.Coverage{}, &store.Evaluation{
		ConceptsDemonstrated: []string{"message queue fan-in", "bloom filters"},
		ConceptsMissing:      []string{"consumer lag monitoring"},
	})

	if len(got.Proven) != 2 {
		t.Errorf("Proven = %v, want 2 entries", got.Proven)
	}
	if len(got.Missing) != 1 {
		t.Errorf("Missing = %v, want 1 entry", got.Missing)
	}
}

// Proving a concept must clear it from the gap lists. Otherwise the roadmap
// tells the candidate to study something they have since demonstrated.
func TestProvingAConceptClearsItFromShakyAndMissing(t *testing.T) {
	start := store.Coverage{
		Shaky:   []string{"backpressure"},
		Missing: []string{"consumer lag"},
	}

	got := MergeCoverage(start, &store.Evaluation{
		ConceptsDemonstrated: []string{"Backpressure", "Consumer Lag"},
	})

	if len(got.Shaky) != 0 {
		t.Errorf("Shaky = %v, want empty once demonstrated", got.Shaky)
	}
	if len(got.Missing) != 0 {
		t.Errorf("Missing = %v, want empty once demonstrated", got.Missing)
	}
	if len(got.Proven) != 2 {
		t.Errorf("Proven = %v, want both concepts", got.Proven)
	}
}

// An 'incomplete' span is the precise definition of shaky: attempted and
// partially correct.
func TestIncompleteSpansBecomeShaky(t *testing.T) {
	got := MergeCoverage(store.Coverage{}, &store.Evaluation{
		Spans: []store.Span{
			{Verdict: store.VerdictIncomplete, Concept: "backpressure"},
			{Verdict: store.VerdictValidated, Concept: "queueing"},
			{Verdict: store.VerdictUnsupported, Concept: "throughput claims"},
		},
	})

	if len(got.Shaky) != 1 || got.Shaky[0] != "backpressure" {
		t.Errorf("Shaky = %v, want exactly [backpressure]", got.Shaky)
	}
}

func TestCoverageIsCaseInsensitiveButKeepsReadableText(t *testing.T) {
	got := MergeCoverage(store.Coverage{}, &store.Evaluation{
		ConceptsDemonstrated: []string{"Backpressure", "backpressure", "BACKPRESSURE"},
	})

	if len(got.Proven) != 1 {
		t.Fatalf("Proven = %v, want a single deduplicated entry", got.Proven)
	}
	if got.Proven[0] != "Backpressure" {
		t.Errorf("Proven[0] = %q, want the first spelling preserved", got.Proven[0])
	}
}

func TestCoverageSlicesAreNeverNil(t *testing.T) {
	// A nil slice serialises to JSON null, forcing a null check on the client
	// for every session that has not covered anything yet.
	got := MergeCoverage(store.Coverage{}, &store.Evaluation{})
	if got.Proven == nil || got.Shaky == nil || got.Missing == nil {
		t.Errorf("nil slice in coverage: %+v", got)
	}
}

func TestCoverageAccumulatesAcrossTurns(t *testing.T) {
	s := &State{Band: 3}

	Apply(s, &store.Evaluation{
		TurnScore:            8.0,
		ConceptsDemonstrated: []string{"queueing"},
		ConceptsMissing:      []string{"backpressure"},
	})
	Apply(s, &store.Evaluation{
		TurnScore:            8.0,
		ConceptsDemonstrated: []string{"backpressure"},
		ConceptsMissing:      []string{"consumer lag"},
	})

	if !contains(s.Coverage.Proven, "queueing") || !contains(s.Coverage.Proven, "backpressure") {
		t.Errorf("Proven = %v, want both concepts accumulated", s.Coverage.Proven)
	}
	if contains(s.Coverage.Missing, "backpressure") {
		t.Error("backpressure is still listed as missing after being demonstrated")
	}
	if !contains(s.Coverage.Missing, "consumer lag") {
		t.Errorf("Missing = %v, want consumer lag", s.Coverage.Missing)
	}
}

// --- helpers --------------------------------------------------------------

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}

// The bug the other tests could not see.
//
// Every test above keeps one State in memory across calls. Production does not:
// each turn is graded by a worker that rebuilds State from Firestore, possibly
// on a different instance. If any field is dropped in that round trip the
// engine silently stops adapting — the streak resets every turn and can never
// reach the two consecutive turns a promotion needs.
//
// This test simulates the persistence boundary explicitly by serialising State
// through the fields we actually store.
func TestAdaptationSurvivesThePersistenceBoundary(t *testing.T) {
	// roundTrip keeps only what the session document carries.
	roundTrip := func(s *State) *State {
		return &State{
			Band:           s.Band,
			TurnIndex:      s.TurnIndex,
			LastScore:      s.LastScore,
			HasLastScore:   s.HasLastScore,
			StrongStreak:   s.StrongStreak,
			WeakStreak:     s.WeakStreak,
			LastChangeTurn: s.LastChangeTurn,
			Coverage:       s.Coverage,
		}
	}

	s := &State{Band: 3}
	for i, score := range []float64{9.0, 9.0} {
		s = roundTrip(s) // as if reloaded from Firestore by a fresh worker
		Apply(s, evalScoring(score))
		t.Logf("turn %d: band=%d strongStreak=%d", i+1, s.Band, s.StrongStreak)
	}

	if s.Band != 4 {
		t.Errorf("band = %d after two strong answers across a persistence boundary, want 4 — "+
			"the engine state is not being fully round-tripped", s.Band)
	}
}

// A partial round trip must fail this test, proving it actually guards the bug.
func TestPartialRoundTripBreaksAdaptation(t *testing.T) {
	// Deliberately drops the streak fields, which is exactly what the first
	// implementation did.
	partial := func(s *State) *State {
		return &State{Band: s.Band, TurnIndex: s.TurnIndex, Coverage: s.Coverage}
	}

	s := &State{Band: 3}
	for _, score := range []float64{9.0, 9.0, 9.0, 9.0} {
		s = partial(s)
		Apply(s, evalScoring(score))
	}

	if s.Band != 3 {
		t.Errorf("band = %d; this test documents that a PARTIAL round trip must "+
			"leave the band stuck at 3. If it now adapts, the guard above is no longer meaningful.", s.Band)
	}
}
