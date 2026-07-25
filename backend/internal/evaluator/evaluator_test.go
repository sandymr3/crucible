package evaluator

import (
	"io"
	"log/slog"
	"testing"

	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/store"
)

const testTranscript = "So the ingestion layer used a Kafka topic per source and we deduplicated " +
	"downstream using a bloom filter before the feature store write backpressure was just a " +
	"bigger buffer and we were handling about 2000 requests per second"

func testEvaluator(redThreshold float64) *Evaluator {
	return New(&config.Config{EvalRedConfidenceMin: redThreshold},
		slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
}

func testInput() Input {
	return Input{
		TurnID:     "t1",
		Transcript: testTranscript,
		Band:       3,
		Persona:    store.PersonaTechLead,
	}
}

// AD-4, and the defence behind PRD §5.1's most important test. A confident red
// stands; an unconfident one becomes "unsupported", which is both more honest
// and far less damaging when the model is wrong.
func TestLowConfidenceRedIsDowngraded(t *testing.T) {
	e := testEvaluator(0.75)

	raw := rawEvaluation{}
	raw.Spans = append(raw.Spans,
		spanFixture("backpressure was just a bigger buffer", "incorrect", 0.95),
		spanFixture("a Kafka topic per source", "incorrect", 0.40),
	)

	got := e.validate(testInput(), raw)

	if len(got.Spans) != 2 {
		t.Fatalf("got %d spans, want 2", len(got.Spans))
	}
	if got.Spans[0].Verdict != store.VerdictIncorrect {
		t.Errorf("confident red became %s, want it to stand", got.Spans[0].Verdict)
	}
	if got.Spans[1].Verdict != store.VerdictUnsupported {
		t.Errorf("unconfident red became %s, want unsupported", got.Spans[1].Verdict)
	}
	if got.RedsDowngraded != 1 {
		t.Errorf("RedsDowngraded = %d, want 1", got.RedsDowngraded)
	}
}

// The threshold is tunable by env var precisely so it can be tightened at 2 a.m.
// without a redeploy; confirm it is actually consulted.
func TestRedThresholdIsConfigurable(t *testing.T) {
	raw := rawEvaluation{}
	raw.Spans = append(raw.Spans, spanFixture("backpressure was just a bigger buffer", "incorrect", 0.80))

	if got := testEvaluator(0.75).validate(testInput(), raw); got.Spans[0].Verdict != store.VerdictIncorrect {
		t.Error("0.80 confidence should survive a 0.75 threshold")
	}
	if got := testEvaluator(0.90).validate(testInput(), raw); got.Spans[0].Verdict != store.VerdictUnsupported {
		t.Error("0.80 confidence should be downgraded by a 0.90 threshold")
	}
}

// Only 'incorrect' is gated. Downgrading an amber would silently erase real
// feedback the candidate needs.
func TestOnlyRedVerdictsAreGated(t *testing.T) {
	e := testEvaluator(0.99)

	raw := rawEvaluation{}
	raw.Spans = append(raw.Spans,
		spanFixture("a Kafka topic per source", "validated", 0.10),
		spanFixture("backpressure was just a bigger buffer", "incomplete", 0.10),
		spanFixture("about 2000 requests per second", "unsupported", 0.10),
	)

	got := e.validate(testInput(), raw)
	want := []store.Verdict{store.VerdictValidated, store.VerdictIncomplete, store.VerdictUnsupported}
	for i, w := range want {
		if got.Spans[i].Verdict != w {
			t.Errorf("span %d verdict = %s, want %s (only reds are gated)", i, got.Spans[i].Verdict, w)
		}
	}
	if got.RedsDowngraded != 0 {
		t.Errorf("RedsDowngraded = %d, want 0", got.RedsDowngraded)
	}
}

// An LLM asked for 1-10 will occasionally return 0, 11, or a negative. Those
// must never reach the difficulty ladder, which assumes the documented range.
func TestScoresAreClampedToRange(t *testing.T) {
	e := testEvaluator(0.75)

	raw := rawEvaluation{}
	raw.Scores.TechnicalAccuracy = 0
	raw.Scores.CommunicationClarity = 11
	raw.Scores.Depth = -5
	raw.Scores.Structure = 7

	got := e.validate(testInput(), raw)
	if got.Scores.TechnicalAccuracy != 1 {
		t.Errorf("0 clamped to %d, want 1", got.Scores.TechnicalAccuracy)
	}
	if got.Scores.CommunicationClarity != 10 {
		t.Errorf("11 clamped to %d, want 10", got.Scores.CommunicationClarity)
	}
	if got.Scores.Depth != 1 {
		t.Errorf("-5 clamped to %d, want 1", got.Scores.Depth)
	}
	if got.Scores.Structure != 7 {
		t.Errorf("7 changed to %d, want it untouched", got.Scores.Structure)
	}
}

func TestUnanchorableSpansAreDroppedButConceptIsKept(t *testing.T) {
	e := testEvaluator(0.75)

	raw := rawEvaluation{}
	raw.Spans = append(raw.Spans,
		spanFixture("a Kafka topic per source", "validated", 0.9),
		// A paraphrase that appears nowhere in the transcript.
		spanFixture("the candidate discussed probabilistic data structures at length", "incomplete", 0.9),
	)
	raw.Spans[1].Concept = "probabilistic deduplication"

	got := e.validate(testInput(), raw)

	if len(got.Spans) != 1 {
		t.Fatalf("got %d spans, want 1 (the paraphrase must be dropped)", len(got.Spans))
	}
	if got.SpansDropped != 1 {
		t.Errorf("SpansDropped = %d, want 1", got.SpansDropped)
	}
	// The highlight is gone but the gap is real, so the roadmap must not lose it.
	found := false
	for _, c := range got.ConceptsMissing {
		if c == "probabilistic deduplication" {
			found = true
		}
	}
	if !found {
		t.Error("a dropped span's concept was lost; the roadmap needs it")
	}
}

// A validated span that cannot be anchored is not a gap — adding it to
// concepts_missing would tell the candidate to study something they got right.
func TestDroppedValidatedSpanDoesNotBecomeAGap(t *testing.T) {
	e := testEvaluator(0.75)

	raw := rawEvaluation{}
	raw.Spans = append(raw.Spans, spanFixture("an entirely unrelated paraphrase of something else", "validated", 0.9))
	raw.Spans[0].Concept = "message queue fan-in"

	got := e.validate(testInput(), raw)
	for _, c := range got.ConceptsMissing {
		if c == "message queue fan-in" {
			t.Error("a dropped VALIDATED span was recorded as a gap")
		}
	}
}

func TestSpanOffsetsIndexTheRealTranscript(t *testing.T) {
	e := testEvaluator(0.75)

	raw := rawEvaluation{}
	// Quoted with punctuation and capitals the transcript does not have.
	raw.Spans = append(raw.Spans, spanFixture("Deduplicated downstream using a bloom filter.", "validated", 0.9))

	got := e.validate(testInput(), raw)
	if len(got.Spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(got.Spans))
	}
	s := got.Spans[0]
	if testTranscript[s.Start:s.End] != s.Excerpt {
		t.Errorf("offsets [%d,%d) slice %q but Excerpt is %q",
			s.Start, s.End, testTranscript[s.Start:s.End], s.Excerpt)
	}
}

func TestUnknownVerdictIsDiscarded(t *testing.T) {
	e := testEvaluator(0.75)

	raw := rawEvaluation{}
	raw.Spans = append(raw.Spans,
		spanFixture("a Kafka topic per source", "probably_fine", 0.9),
		spanFixture("about 2000 requests per second", "unsupported", 0.9),
	)

	got := e.validate(testInput(), raw)
	if len(got.Spans) != 1 {
		t.Fatalf("got %d spans, want 1 — an uncolourable verdict must be discarded", len(got.Spans))
	}
	if got.Spans[0].Verdict != store.VerdictUnsupported {
		t.Errorf("surviving span verdict = %s", got.Spans[0].Verdict)
	}
}

// An unrecognised recommendation must not move the band. "hold" is the only
// safe default.
func TestDifficultyRecommendationDefaultsToHold(t *testing.T) {
	e := testEvaluator(0.75)

	for input, want := range map[string]string{
		"raise": "raise", "RAISE": "raise", "lower": "lower",
		"hold": "hold", "": "hold", "increase": "hold", "nonsense": "hold",
	} {
		raw := rawEvaluation{DifficultyRecommendation: input}
		if got := e.validate(testInput(), raw).DifficultyRecommendation; got != want {
			t.Errorf("recommendation %q became %q, want %q", input, got, want)
		}
	}
}

func TestListsAreCappedAndBlanksRemoved(t *testing.T) {
	e := testEvaluator(0.75)

	raw := rawEvaluation{}
	for i := 0; i < 30; i++ {
		raw.ConceptsMissing = append(raw.ConceptsMissing, "concept")
	}
	raw.ConceptsDemonstrated = []string{"  ", "", "queueing", "   "}

	got := e.validate(testInput(), raw)
	if len(got.ConceptsMissing) > maxConcepts {
		t.Errorf("ConceptsMissing has %d entries, want at most %d", len(got.ConceptsMissing), maxConcepts)
	}
	if len(got.ConceptsDemonstrated) != 1 || got.ConceptsDemonstrated[0] != "queueing" {
		t.Errorf("ConceptsDemonstrated = %v, want blanks removed", got.ConceptsDemonstrated)
	}
}

func TestSpansIsNeverNil(t *testing.T) {
	// A nil slice serialises to JSON null, which forces a null check on the
	// frontend for every turn that happened to produce no spans.
	got := testEvaluator(0.75).validate(testInput(), rawEvaluation{})
	if got.Spans == nil {
		t.Error("Spans is nil; want an empty slice so it renders as []")
	}
}

func spanFixture(excerpt, verdict string, confidence float64) struct {
	Excerpt     string  `json:"excerpt"`
	Verdict     string  `json:"verdict"`
	Concept     string  `json:"concept"`
	Explanation string  `json:"explanation"`
	Correction  string  `json:"correction"`
	Confidence  float64 `json:"confidence"`
} {
	return struct {
		Excerpt     string  `json:"excerpt"`
		Verdict     string  `json:"verdict"`
		Concept     string  `json:"concept"`
		Explanation string  `json:"explanation"`
		Correction  string  `json:"correction"`
		Confidence  float64 `json:"confidence"`
	}{Excerpt: excerpt, Verdict: verdict, Confidence: confidence, Concept: "test concept"}
}
