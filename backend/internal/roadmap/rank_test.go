package roadmap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santh/crucible/internal/store"
)

func turnMissing(score float64, missing ...string) *store.Turn {
	return &store.Turn{
		Evaluation: &store.Evaluation{TurnScore: score, ConceptsMissing: missing},
	}
}

func digest(t *testing.T, must, nice []string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"role": map[string]any{"must_haves": must, "nice_to_haves": nice},
	})
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func labels(cs []Cluster) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Label
	}
	return out
}

// Presenting one gap as three days of work is both wrong and demoralising.
func TestNearDuplicateConceptsCluster(t *testing.T) {
	got := Rank(Input{
		Turns: []*store.Turn{
			turnMissing(4, "flow control signalling"),
			turnMissing(4, "signalling for flow control"),
			turnMissing(4, "Flow Control Signalling"),
		},
		HorizonDays: 7,
	})

	if len(got) != 1 {
		t.Fatalf("got %d clusters %v, want 1", len(got), labels(got))
	}
	if got[0].Frequency != 3 {
		t.Errorf("Frequency = %d, want 3", got[0].Frequency)
	}
}

func TestParentheticalAsidesDoNotSplitAConcept(t *testing.T) {
	got := Rank(Input{
		Turns: []*store.Turn{
			turnMissing(4, "load shedding"),
			turnMissing(4, "load shedding (e.g. token bucket, leaky bucket)"),
		},
		HorizonDays: 7,
	})

	if len(got) != 1 {
		t.Fatalf("got %d clusters %v, want 1", len(got), labels(got))
	}
	// The shorter spelling reads better in a plan.
	if got[0].Label != "load shedding" {
		t.Errorf("Label = %q, want the shorter spelling", got[0].Label)
	}
}

// A gap the JD names as required outranks one it never mentions, even at equal
// frequency.
func TestJobDescriptionRelevanceDominatesRanking(t *testing.T) {
	got := Rank(Input{
		Turns: []*store.Turn{
			turnMissing(4, "obscure trivia"),
			turnMissing(4, "streaming pipelines"),
		},
		Digest:      digest(t, []string{"streaming pipelines"}, nil),
		HorizonDays: 7,
	})

	var streaming, trivia Cluster
	for _, c := range got {
		switch c.Label {
		case "streaming pipelines":
			streaming = c
		case "obscure trivia":
			trivia = c
		}
	}
	if streaming.Relevance != RelevanceMustHave {
		t.Errorf("must-have relevance = %v, want %v", streaming.Relevance, RelevanceMustHave)
	}
	if trivia.Relevance != RelevanceUnmentioned {
		t.Errorf("unmentioned relevance = %v, want %v", trivia.Relevance, RelevanceUnmentioned)
	}
	if streaming.Score <= trivia.Score {
		t.Errorf("JD-required concept scored %v, unmentioned scored %v; required must win",
			streaming.Score, trivia.Score)
	}
}

func TestNiceToHaveRanksBetweenRequiredAndUnmentioned(t *testing.T) {
	got := Rank(Input{
		Turns:       []*store.Turn{turnMissing(4, "kafka tuning")},
		Digest:      digest(t, nil, []string{"kafka"}),
		HorizonDays: 7,
	})
	if len(got) == 0 || got[0].Relevance != RelevanceNiceToHave {
		t.Errorf("relevance = %v, want %v", got, RelevanceNiceToHave)
	}
}

// A gap in an answer that scored 2 is more urgent than the same gap in one that
// scored 8.
func TestSeverityTracksTurnScore(t *testing.T) {
	bad := Rank(Input{Turns: []*store.Turn{turnMissing(2.0, "backpressure")}, HorizonDays: 7})
	good := Rank(Input{Turns: []*store.Turn{turnMissing(8.0, "backpressure")}, HorizonDays: 7})

	if bad[0].Severity <= good[0].Severity {
		t.Errorf("severity from a 2/10 answer (%v) should exceed that from an 8/10 answer (%v)",
			bad[0].Severity, good[0].Severity)
	}
}

// Telling a candidate to study something they demonstrated is the fastest way
// to lose their trust in the whole report.
func TestProvenConceptsAreExcluded(t *testing.T) {
	got := Rank(Input{
		Turns:       []*store.Turn{turnMissing(5, "backpressure"), turnMissing(5, "consumer lag")},
		Coverage:    store.Coverage{Proven: []string{"Backpressure"}},
		HorizonDays: 7,
	})

	for _, c := range got {
		if strings.EqualFold(c.Label, "backpressure") {
			t.Error("a proven concept appeared in the study plan")
		}
	}
	if len(got) != 1 {
		t.Errorf("got %v, want only the unproven concept", labels(got))
	}
}

// Roughly 1.5 concepts per available day, bounded at both ends.
func TestConceptCountScalesWithHorizon(t *testing.T) {
	// Genuinely distinct concept names. An earlier version of this test used
	// single-letter suffixes, which the clusterer correctly folded into one
	// cluster — the test was wrong, not the code.
	names := []string{
		"backpressure", "consumer lag", "bloom filters", "raft consensus",
		"shard rebalancing", "feature stores", "vector indexing", "batch inference",
		"model quantisation", "gradient accumulation", "cache invalidation",
		"connection pooling", "circuit breakers", "idempotency keys",
		"exponential backoff", "leader election", "write ahead logging",
		"columnar storage", "query planning", "index selectivity",
	}
	var turns []*store.Turn
	for _, n := range names {
		turns = append(turns, turnMissing(4, n))
	}

	short := Rank(Input{Turns: turns, HorizonDays: 2})
	long := Rank(Input{Turns: turns, HorizonDays: 10})

	if len(short) >= len(long) {
		t.Errorf("2-day plan has %d concepts, 10-day has %d; longer horizons should carry more",
			len(short), len(long))
	}
	if len(long) > 12 {
		t.Errorf("10-day plan has %d concepts; the cap should bound it", len(long))
	}
	if len(short) < 3 {
		t.Errorf("2-day plan has %d concepts; the floor should keep it useful", len(short))
	}
}

// PRD §14.1: order by prerequisite, not by score. Learning is ordered even when
// priorities are not.
func TestFoundationalConceptsComeFirst(t *testing.T) {
	got := Rank(Input{
		Turns: []*store.Turn{
			// Highest priority, but advanced.
			turnMissing(1, "distributed consensus"),
			turnMissing(1, "distributed consensus"),
			turnMissing(1, "distributed consensus"),
			// Lower priority, but foundational.
			turnMissing(7, "queue data structure"),
		},
		HorizonDays: 7,
	})

	if len(got) < 2 {
		t.Fatalf("got %v", labels(got))
	}
	if !strings.Contains(strings.ToLower(got[0].Label), "queue") {
		t.Errorf("plan order = %v; the foundational concept should come first "+
			"despite the advanced one scoring higher", labels(got))
	}
}

func TestNoGapsProducesNoClusters(t *testing.T) {
	if got := Rank(Input{HorizonDays: 7}); len(got) != 0 {
		t.Errorf("got %v, want no clusters", labels(got))
	}
}

// An 'incomplete' span is a partial gap and should weigh less than a full miss.
func TestIncompleteSpansContributeLessThanFullMisses(t *testing.T) {
	full := Rank(Input{
		Turns:       []*store.Turn{turnMissing(5, "backpressure")},
		HorizonDays: 7,
	})
	partial := Rank(Input{
		Turns: []*store.Turn{{Evaluation: &store.Evaluation{
			TurnScore: 5,
			Spans:     []store.Span{{Verdict: store.VerdictIncomplete, Concept: "backpressure"}},
		}}},
		HorizonDays: 7,
	})

	if len(full) == 0 || len(partial) == 0 {
		t.Fatal("expected both to produce a cluster")
	}
	if partial[0].Severity >= full[0].Severity {
		t.Errorf("partial gap severity %v should be below a full miss %v",
			partial[0].Severity, full[0].Severity)
	}
}

func TestClusterKeyIsOrderInsensitive(t *testing.T) {
	if clusterKey("flow control signalling") != clusterKey("signalling flow control") {
		t.Error("word order changed the cluster key")
	}
	if clusterKey("consumer lag") == clusterKey("producer throughput") {
		t.Error("unrelated concepts collapsed to the same key")
	}
}
