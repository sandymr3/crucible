package report

import (
	"encoding/json"
	"testing"

	"github.com/santh/crucible/internal/store"
)

func digestWithDomains(t *testing.T, domains ...string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"role": map[string]any{"domain_areas": domains},
	})
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func gradedTurn(index int, score float64, s store.Scores, demonstrated, missing []string) *store.Turn {
	return &store.Turn{
		ID:    "t" + string(rune('0'+index)),
		Index: index,
		Evaluation: &store.Evaluation{
			TurnScore:            score,
			Scores:               s,
			ConceptsDemonstrated: demonstrated,
			ConceptsMissing:      missing,
		},
	}
}

func TestAggregateScoresAverageOnlyGradedTurns(t *testing.T) {
	turns := []*store.Turn{
		gradedTurn(0, 8.0, store.Scores{TechnicalAccuracy: 8, CommunicationClarity: 6, Depth: 8, Structure: 6}, nil, nil),
		gradedTurn(1, 6.0, store.Scores{TechnicalAccuracy: 6, CommunicationClarity: 8, Depth: 4, Structure: 8}, nil, nil),
		// Ungraded: a grader outage must not drag the average down.
		{ID: "t2", Index: 2, GradingStatus: store.GradingFailed},
	}

	r := Build(&store.Session{ID: "s1"}, turns)

	if r.TurnsGraded != 2 {
		t.Errorf("TurnsGraded = %d, want 2", r.TurnsGraded)
	}
	if r.AggregateScores.TechnicalAccuracy != 7 {
		t.Errorf("technical accuracy = %d, want 7", r.AggregateScores.TechnicalAccuracy)
	}
	if r.OverallScore != 7.0 {
		t.Errorf("OverallScore = %v, want 7.0", r.OverallScore)
	}
}

func TestNoGradedTurnsProducesAnEmptyButValidReport(t *testing.T) {
	// A session that ended before anything was graded must still render.
	r := Build(&store.Session{ID: "s1"}, []*store.Turn{{ID: "t0", GradingStatus: store.GradingFailed}})

	if r.TurnsGraded != 0 || r.OverallScore != 0 {
		t.Errorf("expected an empty aggregate, got graded=%d overall=%v", r.TurnsGraded, r.OverallScore)
	}
	if r.Strengths == nil || r.Gaps == nil || r.DomainScores == nil || r.Turns == nil {
		t.Error("nil slice in report; JSON would render null and break the client")
	}
}

// The radar chart is the visual PRD §11.4 says the problem statement is
// implicitly asking for, so an empty one is a visible failure.
func TestDomainScoresAttributeTurnsToMatchingAreas(t *testing.T) {
	sess := &store.Session{
		ID:     "s1",
		Digest: digestWithDomains(t, "feature engineering", "model serving"),
	}
	turns := []*store.Turn{
		gradedTurn(0, 9.0, store.Scores{}, []string{"feature pipelines"}, nil),
		gradedTurn(1, 5.0, store.Scores{}, []string{"low latency serving"}, nil),
	}

	r := Build(sess, turns)

	if len(r.DomainScores) != 2 {
		t.Fatalf("got %d domain scores, want 2", len(r.DomainScores))
	}
	byName := map[string]DomainScore{}
	for _, d := range r.DomainScores {
		byName[d.Domain] = d
	}
	if got := byName["feature engineering"].Score; got != 9.0 {
		t.Errorf("feature engineering scored %v, want 9.0", got)
	}
	if got := byName["model serving"].Score; got != 5.0 {
		t.Errorf("model serving scored %v, want 5.0", got)
	}
}

// A session whose concepts never name a domain would otherwise render a blank
// chart, which looks broken rather than empty.
func TestUnattributableTurnsStillPopulateTheChart(t *testing.T) {
	sess := &store.Session{ID: "s1", Digest: digestWithDomains(t, "distributed training", "MLOps tooling")}
	turns := []*store.Turn{gradedTurn(0, 7.0, store.Scores{}, []string{"queueing"}, nil)}

	r := Build(sess, turns)

	for _, d := range r.DomainScores {
		if d.Score == 0 {
			t.Errorf("domain %q scored 0; an unattributable turn should spread across axes", d.Domain)
		}
	}
}

// Word-level matching, so "serving" does not match "observing".
func TestDomainMatchingDoesNotMatchSubstringsOfOtherWords(t *testing.T) {
	if domainMatches("we were observing the metrics", "model serving") {
		t.Error("'observing' matched the domain 'model serving'")
	}
	if !domainMatches("low latency serving path", "model serving") {
		t.Error("'serving' failed to match the domain 'model serving'")
	}
	// Short words must not match anything, or "ML" matches half the transcript.
	if domainMatches("the and but", "ML the") {
		t.Error("a short stopword produced a domain match")
	}
}

// A list of nineteen weaknesses is not actionable, it is discouraging.
func TestGapsAreCappedAndRankedByFrequency(t *testing.T) {
	turns := []*store.Turn{
		gradedTurn(0, 5, store.Scores{}, nil, []string{"backpressure", "consumer lag", "sharding"}),
		gradedTurn(1, 5, store.Scores{}, nil, []string{"backpressure", "consumer lag", "caching"}),
		gradedTurn(2, 5, store.Scores{}, nil, []string{"backpressure", "retries", "indexing", "batching", "quotas"}),
	}

	r := Build(&store.Session{ID: "s1"}, turns)

	if len(r.Gaps) > maxGaps {
		t.Errorf("got %d gaps, want at most %d", len(r.Gaps), maxGaps)
	}
	if len(r.Gaps) == 0 || r.Gaps[0] != "backpressure" {
		t.Errorf("Gaps = %v, want the most frequent concept first", r.Gaps)
	}
}

func TestBandTrajectoryTracksHistory(t *testing.T) {
	sess := &store.Session{
		ID:             "s1",
		DifficultyBand: 5,
		BandHistory: []store.BandChange{
			{TurnIndex: 2, Band: 4},
			{TurnIndex: 4, Band: 5},
		},
	}

	r := Build(sess, nil)

	if want := []int{3, 4, 5}; !equalInts(r.BandTrajectory, want) {
		t.Errorf("BandTrajectory = %v, want %v", r.BandTrajectory, want)
	}
	if r.StartBand != 3 || r.EndBand != 5 {
		t.Errorf("start/end = %d/%d, want 3/5", r.StartBand, r.EndBand)
	}
}

func TestDeliveryAggregatesAcrossTurns(t *testing.T) {
	turns := []*store.Turn{
		{ID: "t0", Delivery: &store.Delivery{WordCount: 100, SpeakingTimeMs: 60000, FillerCount: 8, HesitationScore: 0.4}},
		{ID: "t1", Delivery: &store.Delivery{WordCount: 140, SpeakingTimeMs: 60000, FillerCount: 6, HesitationScore: 0.6,
			Observation: "latest observation", Drill: "latest drill"}},
	}

	r := Build(&store.Session{ID: "s1"}, turns)

	if r.Delivery.FillerTotal != 14 {
		t.Errorf("FillerTotal = %d, want 14", r.Delivery.FillerTotal)
	}
	// 240 words over 2 minutes.
	if r.Delivery.WPM != 120 {
		t.Errorf("WPM = %v, want 120", r.Delivery.WPM)
	}
	if r.Delivery.FillerPerMinute != 7 {
		t.Errorf("FillerPerMinute = %v, want 7", r.Delivery.FillerPerMinute)
	}
	if r.Delivery.PaceBand != "optimal" {
		t.Errorf("PaceBand = %q, want optimal at 120 wpm", r.Delivery.PaceBand)
	}
	if r.Delivery.HesitationScore != 0.5 {
		t.Errorf("HesitationScore = %v, want 0.5", r.Delivery.HesitationScore)
	}
	// The most recent turn's coaching is the relevant one.
	if r.Delivery.Observation != "latest observation" || r.Delivery.Drill != "latest drill" {
		t.Error("expected the most recent turn's observation and drill to carry forward")
	}
}

func TestSpanCountsPerTurn(t *testing.T) {
	turns := []*store.Turn{{
		ID: "t0", Index: 0,
		Evaluation: &store.Evaluation{
			TurnScore: 7,
			Spans: []store.Span{
				{Verdict: store.VerdictValidated}, {Verdict: store.VerdictValidated},
				{Verdict: store.VerdictIncomplete}, {Verdict: store.VerdictUnsupported},
			},
		},
	}}

	r := Build(&store.Session{ID: "s1"}, turns)

	counts := r.Turns[0].SpanCounts
	if counts["validated"] != 2 || counts["incomplete"] != 1 || counts["unsupported"] != 1 {
		t.Errorf("SpanCounts = %v", counts)
	}
	if counts["incorrect"] != 0 {
		t.Errorf("expected no red spans, got %d", counts["incorrect"])
	}
}

func equalInts(a, b []int) bool {
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
