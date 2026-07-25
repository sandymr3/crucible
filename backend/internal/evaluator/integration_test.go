//go:build integration

// Golden tests against live Vertex.
//
// Excluded from the default build because they cost credits and take real
// time. Run with:
//
//	go test ./internal/evaluator/ -tags=integration -v -timeout=10m
//
// Re-run these after EVERY edit to evaluate_turn.md. The zero-red-spans case is
// PRD §5.1's most important acceptance criterion, and prompt changes are
// exactly what silently breaks it.
package evaluator

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/vertexai"
)

// TestMain moves to the module root ONCE.
//
// Doing this per-test silently broke every test after the first: the second
// chdir was relative to the already-moved directory, so the key file path no
// longer resolved and the tests skipped rather than failed — which looks like
// a pass at a glance.
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic("chdir to module root: " + err.Error())
	}
	_ = config.LoadDotEnv(".env")
	os.Exit(m.Run())
}

var (
	sharedOnce sync.Once
	sharedEval *Evaluator
	sharedErr  error
)

func liveEvaluator(t *testing.T) *Evaluator {
	t.Helper()

	sharedOnce.Do(func() {
		cfg, err := config.Load()
		if err != nil {
			sharedErr = err
			return
		}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		vx, err := vertexai.New(context.Background(), cfg, log, nil)
		if err != nil {
			sharedErr = err
			return
		}
		sharedEval = New(cfg, log, vx)
	})

	if sharedErr != nil {
		t.Skipf("no live Vertex access, skipping: %v", sharedErr)
	}
	return sharedEval
}

const (
	// An answer that is genuinely strong: mechanism, measurement, and a named
	// tradeoff. This must not attract a red span.
	answerExcellent = "We used a bounded queue between the ingestion workers and the feature " +
		"store writer. When the queue hit its high-water mark the producer blocked, which " +
		"propagated backpressure to the upstream HTTP handler and let it shed load with a 429. " +
		"We monitored consumer lag as the primary signal, alerting when lag exceeded thirty " +
		"seconds, because lag rises before throughput drops. Load shedding was preferred over " +
		"unbounded buffering because latency degrades gracefully but memory does not."

	// Directionally right, no mechanism. Should attract amber, not red.
	answerVague = "Yeah so for backpressure we basically just handled it at the queue level. " +
		"We had some monitoring set up and it worked pretty well overall. The system scaled " +
		"fine and we didn't really have issues with consumers falling behind that much."

	// Specific numbers with no basis. Should attract blue, not red.
	answerFabricated = "Our pipeline sustained forty thousand requests per second with p99 " +
		"latency under two milliseconds. We reduced infrastructure cost by sixty percent and " +
		"the deduplication was one hundred percent accurate across every workload we tested."

	testQuestion = "How did you handle backpressure in that streaming pipeline, " +
		"particularly when the consumer stalled?"
)

func evaluate(t *testing.T, e *Evaluator, transcript string) *store.Evaluation {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	started := time.Now()
	eval, err := e.Evaluate(ctx, Input{
		TurnID:      "integration",
		Question:    testQuestion,
		Transcript:  transcript,
		RoleTitle:   "ML Engineer",
		Seniority:   "mid",
		Band:        3,
		Persona:     store.PersonaTechLead,
		DomainVocab: []string{"Kafka", "Python", "feature store", "bloom filter", "backpressure"},
	})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	t.Logf("latency=%dms score=%.2f spans=%d dropped=%d downgraded=%d",
		time.Since(started).Milliseconds(), eval.TurnScore,
		len(eval.Spans), eval.SpansDropped, eval.RedsDowngraded)
	for _, s := range eval.Spans {
		t.Logf("  [%-11s conf=%.2f] %s", s.Verdict, s.Confidence, truncate(s.Excerpt, 70))
	}
	return eval
}

// THE most important test in this codebase.
//
// PRD §5.1: "A deliberately excellent answer produces zero red spans." A false
// red destroys the candidate's trust in every other judgement the system makes,
// including the correct ones.
func TestExcellentAnswerProducesNoRedSpans(t *testing.T) {
	e := liveEvaluator(t)
	eval := evaluate(t, e, answerExcellent)

	for _, s := range eval.Spans {
		if s.Verdict == store.VerdictIncorrect {
			t.Errorf("FALSE RED on an excellent answer (confidence %.2f): %q — %s",
				s.Confidence, s.Excerpt, s.Explanation)
		}
	}
	if eval.Scores.TechnicalAccuracy < 7 {
		t.Errorf("technical_accuracy = %d on a strong answer, want >= 7", eval.Scores.TechnicalAccuracy)
	}
	if eval.TurnScore < 6.5 {
		t.Errorf("turn score = %.2f on a strong answer, want >= 6.5", eval.TurnScore)
	}
}

// A vague answer must be marked thin, not wrong. Amber is the honest verdict
// for "correct as far as it goes".
func TestVagueAnswerIsMarkedThinNotWrong(t *testing.T) {
	e := liveEvaluator(t)
	eval := evaluate(t, e, answerVague)

	if eval.Scores.Depth > 5 {
		t.Errorf("depth = %d on a vague answer, want <= 5", eval.Scores.Depth)
	}

	soft := 0
	for _, s := range eval.Spans {
		switch s.Verdict {
		case store.VerdictIncomplete, store.VerdictUnsupported:
			soft++
		}
	}
	if len(eval.Spans) > 0 && soft == 0 {
		t.Error("a vague answer produced no incomplete or unsupported spans")
	}
	if len(eval.ConceptsMissing) == 0 {
		t.Error("a vague answer produced no missing concepts; the roadmap would have nothing to work with")
	}
}

// Unbacked numbers are 'unsupported', never 'incorrect'. We cannot know they
// are false — only that nothing supports them. Getting this wrong is the
// difference between a coaching tool and an accusatory one.
func TestFabricatedClaimsAreUnsupportedNotIncorrect(t *testing.T) {
	e := liveEvaluator(t)
	eval := evaluate(t, e, answerFabricated)

	unsupported := 0
	for _, s := range eval.Spans {
		if s.Verdict == store.VerdictUnsupported {
			unsupported++
		}
		if s.Verdict == store.VerdictIncorrect {
			t.Errorf("unverifiable claim marked INCORRECT rather than unsupported: %q", s.Excerpt)
		}
	}
	if unsupported == 0 {
		t.Error("no unsupported spans on an answer made entirely of unbacked numbers")
	}
}

// Every span must anchor to the real transcript. A high drop rate means the
// evaluator is paraphrasing instead of quoting, which is a prompt regression.
func TestSpansAnchorToTheTranscript(t *testing.T) {
	e := liveEvaluator(t)

	for name, transcript := range map[string]string{
		"excellent":  answerExcellent,
		"vague":      answerVague,
		"fabricated": answerFabricated,
	} {
		t.Run(name, func(t *testing.T) {
			eval := evaluate(t, e, transcript)

			for _, s := range eval.Spans {
				if s.Start < 0 || s.End > len(transcript) || s.Start >= s.End {
					t.Errorf("span has invalid bounds [%d,%d): %q", s.Start, s.End, s.Excerpt)
					continue
				}
				if transcript[s.Start:s.End] != s.Excerpt {
					t.Errorf("span offsets slice %q but Excerpt is %q",
						transcript[s.Start:s.End], s.Excerpt)
				}
			}

			total := len(eval.Spans) + eval.SpansDropped
			if total > 0 {
				rate := float64(eval.SpansDropped) / float64(total)
				t.Logf("anchor drop rate: %.0f%% (%d of %d)", rate*100, eval.SpansDropped, total)
				if rate > 0.20 {
					t.Errorf("drop rate %.0f%% exceeds 20%%; tighten the verbatim-quoting instruction", rate*100)
				}
			}
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
