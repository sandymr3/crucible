// Package evaluator grades one answer and produces the span-level heatmap.
//
// Two rules govern everything here:
//
//  1. The conversation never waits on this. Evaluation runs on a worker, and
//     every failure path degrades to "the interview keeps working".
//  2. A false red is the worst output this system can produce. It destroys the
//     candidate's trust in every other judgement, including the correct ones.
//     Prompt calibration plus a server-side confidence gate defend against it.
package evaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/santh/crucible/internal/anchor"
	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/persona"
	"github.com/santh/crucible/internal/prompts"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/vertexai"
)

// Evaluator grades turns.
type Evaluator struct {
	cfg *config.Config
	log *slog.Logger
	vx  *vertexai.Client
}

// New builds the evaluator.
func New(cfg *config.Config, log *slog.Logger, vx *vertexai.Client) *Evaluator {
	return &Evaluator{cfg: cfg, log: log, vx: vx}
}

// Input is everything needed to grade one answer.
type Input struct {
	TurnID     string
	Question   string
	Transcript string
	RoleTitle  string
	Seniority  string
	Band       int
	Persona    store.Persona
	// DomainVocab is the resume's primary stack plus the JD's technical nouns.
	// It is what lets the evaluator read "blue filter" as "bloom filter"
	// instead of marking a correct answer wrong.
	DomainVocab []string
}

// evalTimeout bounds the call. PRD §4.4 budgets under 4 s for evaluation and
// says beyond 6 s feels stuck; this is the hard ceiling before we give up and
// mark the turn ungraded.
const evalTimeout = 45 * time.Second

// Evaluate grades an answer and returns a validated, anchored evaluation.
func (e *Evaluator) Evaluate(ctx context.Context, in Input) (*store.Evaluation, error) {
	p, err := prompts.Get(prompts.EvaluateTurn)
	if err != nil {
		return nil, err
	}

	vocab := strings.Join(in.DomainVocab, ", ")
	if vocab == "" {
		vocab = "(none supplied)"
	}
	question := in.Question
	if strings.TrimSpace(question) == "" {
		question = "(the question was not captured; grade the answer on its own terms)"
	}

	instruction := p.Render(map[string]string{
		"QUESTION":         question,
		"TRANSCRIPT":       in.Transcript,
		"ROLE_TITLE":       orDefault(in.RoleTitle, "technical"),
		"SENIORITY":        orDefault(in.Seniority, "mid"),
		"BAND":             fmt.Sprint(in.Band),
		"BAND_DESCRIPTION": persona.BandDescription(in.Band),
		"DOMAIN_VOCAB":     vocab,
	})

	ctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()

	// Low temperature: grading must be as reproducible as the model allows.
	// A rubric that returns a different score for the same answer on a re-run
	// is not a rubric.
	temp := float32(0.2)

	genCfg := &genai.GenerateContentConfig{
		Temperature:      &temp,
		ResponseMIMEType: "application/json",
		ResponseSchema:   evaluationSchema(),
	}
	// Bound the grader's reasoning. Unbounded thinking measured ~3s slower per
	// evaluation on this workload, and the heatmap reveal is a moment the user
	// watches. A negative budget means "leave it to the model".
	if e.cfg.EvalThinkingBudget >= 0 {
		budget := int32(e.cfg.EvalThinkingBudget)
		genCfg.ThinkingConfig = &genai.ThinkingConfig{ThinkingBudget: &budget}
	}

	started := time.Now()
	raw, err := e.vx.GenerateStructured(ctx, e.cfg.ModelReasoning, genai.Text(instruction), genCfg)
	if err != nil {
		return nil, fmt.Errorf("evaluator: grading call failed: %w", err)
	}
	duration := time.Since(started)

	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("evaluator: model returned no content")
	}

	var parsed rawEvaluation
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("evaluator: response was not valid JSON despite the schema: %w", err)
	}

	eval := e.validate(in, parsed)
	eval.TurnID = in.TurnID
	eval.Model = e.cfg.ModelReasoning
	eval.PromptVersion = p.Version
	eval.GradedAt = time.Now()
	eval.DurationMs = duration.Milliseconds()

	// The persona's rubric weighting is what makes the same answer score
	// differently for a Tech Lead than for a PM.
	eval.TurnScore = persona.MustGet(in.Persona).Weights.Score(eval.Scores)

	e.log.Info("turn evaluated",
		"metric", "evaluation_duration_ms",
		"turn_id", in.TurnID,
		"duration_ms", duration.Milliseconds(),
		"turn_score", fmt.Sprintf("%.2f", eval.TurnScore),
		"spans", len(eval.Spans),
		"spans_dropped", eval.SpansDropped,
		"reds_downgraded", eval.RedsDowngraded,
		"prompt_version", p.Version)

	return eval, nil
}

// rawEvaluation mirrors the response schema before validation.
type rawEvaluation struct {
	QuestionIntent string `json:"question_intent"`
	Scores         struct {
		TechnicalAccuracy    int `json:"technical_accuracy"`
		CommunicationClarity int `json:"communication_clarity"`
		Depth                int `json:"depth"`
		Structure            int `json:"structure"`
	} `json:"scores"`
	VerdictSummary string `json:"verdict_summary"`
	Spans          []struct {
		Excerpt     string  `json:"excerpt"`
		Verdict     string  `json:"verdict"`
		Concept     string  `json:"concept"`
		Explanation string  `json:"explanation"`
		Correction  string  `json:"correction"`
		Confidence  float64 `json:"confidence"`
	} `json:"spans"`
	ConceptsDemonstrated     []string `json:"concepts_demonstrated"`
	ConceptsMissing          []string `json:"concepts_missing"`
	IdealAnswerOutline       []string `json:"ideal_answer_outline"`
	FollowupProbe            string   `json:"followup_probe"`
	DifficultyRecommendation string   `json:"difficulty_recommendation"`
}

// Caps on list lengths. Schema compliance is not semantic sanity, and an
// unbounded list would bloat the system instruction on every subsequent turn.
const (
	maxSpans    = 12
	maxConcepts = 8
	maxOutline  = 6
)

// validate turns a schema-compliant response into one we are willing to show a
// user: scores clamped, verdicts calibrated, spans anchored or dropped.
func (e *Evaluator) validate(in Input, raw rawEvaluation) *store.Evaluation {
	eval := &store.Evaluation{
		QuestionIntent: strings.TrimSpace(raw.QuestionIntent),
		Scores: store.Scores{
			TechnicalAccuracy:    clamp(raw.Scores.TechnicalAccuracy, 1, 10),
			CommunicationClarity: clamp(raw.Scores.CommunicationClarity, 1, 10),
			Depth:                clamp(raw.Scores.Depth, 1, 10),
			Structure:            clamp(raw.Scores.Structure, 1, 10),
		},
		VerdictSummary:           strings.TrimSpace(raw.VerdictSummary),
		ConceptsDemonstrated:     trimList(raw.ConceptsDemonstrated, maxConcepts),
		ConceptsMissing:          trimList(raw.ConceptsMissing, maxConcepts),
		IdealAnswerOutline:       trimList(raw.IdealAnswerOutline, maxOutline),
		FollowupProbe:            strings.TrimSpace(raw.FollowupProbe),
		DifficultyRecommendation: normaliseRecommendation(raw.DifficultyRecommendation),
		Spans:                    []store.Span{},
	}

	var stats anchor.Stats

	for _, s := range raw.Spans {
		if len(eval.Spans) >= maxSpans {
			break
		}
		excerpt := strings.TrimSpace(s.Excerpt)
		if excerpt == "" {
			continue
		}

		verdict := store.Verdict(strings.ToLower(strings.TrimSpace(s.Verdict)))
		if !validVerdict(verdict) {
			// An unrecognised verdict cannot be coloured, and guessing would
			// mean inventing a judgement the model did not make.
			continue
		}

		confidence := s.Confidence
		if confidence < 0 {
			confidence = 0
		} else if confidence > 1 {
			confidence = 1
		}

		// AD-4: the server-side gate on red spans.
		//
		// PRD §5.1 names "an excellent answer produces zero red spans" as the
		// most important test in the document. A prompt instruction alone is
		// not a reliable calibration mechanism, so a low-confidence
		// "incorrect" is rewritten to "unsupported" — which is both more
		// honest and far less damaging when wrong.
		if verdict == store.VerdictIncorrect && confidence < e.cfg.EvalRedConfidenceMin {
			e.log.Info("downgraded low-confidence red span",
				"turn_id", in.TurnID,
				"concept", s.Concept,
				"confidence", confidence,
				"threshold", e.cfg.EvalRedConfidenceMin)
			verdict = store.VerdictUnsupported
			eval.RedsDowngraded++
		}

		// Anchor against the real transcript. A span that cannot be located is
		// dropped: a missing highlight is invisible, a misplaced one attaches a
		// verdict to words that never made the claim.
		m := anchor.Find(in.Transcript, excerpt)
		stats.Record(m.Tier)
		if !m.Found() {
			// The concept still counts as missing even though the highlight is
			// gone, so the roadmap does not lose it.
			if s.Concept != "" && verdict != store.VerdictValidated {
				eval.ConceptsMissing = appendUnique(eval.ConceptsMissing, s.Concept, maxConcepts)
			}
			continue
		}

		eval.Spans = append(eval.Spans, store.Span{
			// The transcript's own wording, not the model's quotation of it:
			// the UI highlights a range of the real transcript.
			Excerpt:     m.Text,
			Verdict:     verdict,
			Concept:     strings.TrimSpace(s.Concept),
			Explanation: strings.TrimSpace(s.Explanation),
			Correction:  strings.TrimSpace(s.Correction),
			Confidence:  confidence,
			Start:       m.Start,
			End:         m.End,
		})
	}

	eval.SpansDropped = stats.Dropped

	// Above roughly 20% the evaluator is paraphrasing rather than quoting, and
	// the fix is a tighter prompt rather than a looser matcher.
	if rate := stats.DropRate(); stats.Total > 0 && rate > 0.20 {
		e.log.Warn("span anchoring drop rate is high",
			"metric", "anchor_drop_rate",
			"value", rate,
			"turn_id", in.TurnID,
			"total", stats.Total,
			"dropped", stats.Dropped,
			"hint", "the evaluator is paraphrasing rather than quoting verbatim")
	}

	return eval
}

func validVerdict(v store.Verdict) bool {
	switch v {
	case store.VerdictValidated, store.VerdictIncomplete,
		store.VerdictUnsupported, store.VerdictIncorrect:
		return true
	}
	return false
}

func normaliseRecommendation(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "raise":
		return "raise"
	case "lower":
		return "lower"
	default:
		// "hold" is the safe default: an unrecognised value must not move the
		// difficulty band.
		return "hold"
	}
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

func trimList(items []string, max int) []string {
	out := make([]string, 0, min(len(items), max))
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
		if len(out) >= max {
			break
		}
	}
	return out
}

func appendUnique(items []string, s string, max int) []string {
	if len(items) >= max {
		return items
	}
	for _, existing := range items {
		if strings.EqualFold(existing, s) {
			return items
		}
	}
	return append(items, s)
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
