// Package report aggregates a finished session into the screen the candidate
// actually came for.
//
// Everything here is deterministic arithmetic over turns already graded. There
// is no model call: the judgements were made per-turn by the evaluator, and
// re-asking a model to summarise its own summaries would add latency, cost, and
// a fresh opportunity to contradict itself.
package report

import (
	"sort"
	"strings"
	"time"

	"github.com/santh/crucible/internal/delivery"
	"github.com/santh/crucible/internal/store"
)

// Status drives the 202-polling contract on GET /report.
type Status string

const (
	StatusGenerating Status = "generating"
	StatusReady      Status = "ready"
	StatusFailed     Status = "failed"
)

// DomainScore is one axis of the radar chart.
type DomainScore struct {
	Domain    string  `firestore:"domain" json:"domain"`
	Score     float64 `firestore:"score" json:"score"`
	TurnCount int     `firestore:"turnCount" json:"turnCount"`
}

// TurnSummary is one row of the per-turn accordion.
type TurnSummary struct {
	TurnID     string         `firestore:"turnId" json:"turnId"`
	Index      int            `firestore:"index" json:"index"`
	Question   string         `firestore:"question" json:"question"`
	Score      float64        `firestore:"score" json:"score"`
	HintsUsed  int            `firestore:"hintsUsed" json:"hintsUsed"`
	Band       int            `firestore:"band" json:"band"`
	Graded     bool           `firestore:"graded" json:"graded"`
	SpanCounts map[string]int `firestore:"spanCounts" json:"spanCounts"`
}

// DeliveryAggregate rolls delivery metrics across the whole session.
type DeliveryAggregate struct {
	WPM             float64 `firestore:"wpm" json:"wpm"`
	PaceBand        string  `firestore:"paceBand" json:"paceBand"`
	FillerTotal     int     `firestore:"fillerTotal" json:"fillerTotal"`
	FillerPerMinute float64 `firestore:"fillerPerMinute" json:"fillerPerMinute"`
	SpeakingTimeMs  int64   `firestore:"speakingTimeMs" json:"speakingTimeMs"`
	HesitationScore float64 `firestore:"hesitationScore" json:"hesitationScore"`
	Observation     string  `firestore:"observation,omitempty" json:"observation,omitempty"`
	Drill           string  `firestore:"drill,omitempty" json:"drill,omitempty"`
	TurnsWithAudio  int     `firestore:"turnsWithAudio" json:"turnsWithAudio"`
}

// Report is the finalized session summary.
type Report struct {
	SessionID string `firestore:"sessionId" json:"sessionId"`
	Status    Status `firestore:"status" json:"status"`

	AggregateScores store.Scores  `firestore:"aggregateScores" json:"aggregateScores"`
	OverallScore    float64       `firestore:"overallScore" json:"overallScore"`
	DomainScores    []DomainScore `firestore:"domainScores" json:"domainScores"`

	// BandTrajectory is the sparkline. Read straight off the session's
	// denormalised bandHistory, so no aggregation query is needed.
	BandTrajectory []int `firestore:"bandTrajectory" json:"bandTrajectory"`
	StartBand      int   `firestore:"startBand" json:"startBand"`
	EndBand        int   `firestore:"endBand" json:"endBand"`

	Strengths []string `firestore:"strengths" json:"strengths"`
	Gaps      []string `firestore:"gaps" json:"gaps"`

	Turns    []TurnSummary     `firestore:"turns" json:"turns"`
	Delivery DeliveryAggregate `firestore:"delivery" json:"delivery"`

	TurnsGraded int       `firestore:"turnsGraded" json:"turnsGraded"`
	DurationMs  int64     `firestore:"durationMs" json:"durationMs"`
	GeneratedAt time.Time `firestore:"generatedAt" json:"generatedAt"`
}

// maxGaps caps the "you need to close" list.
//
// A list of nineteen weaknesses is not actionable, it is discouraging. Five is
// what someone can actually hold in their head walking out of the room.
const maxGaps = 5

// maxStrengths keeps the two columns visually balanced.
const maxStrengths = 6

// Build assembles the report from a session and its turns.
func Build(sess *store.Session, turns []*store.Turn) *Report {
	r := &Report{
		SessionID:      sess.ID,
		Status:         StatusReady,
		DomainScores:   []DomainScore{},
		BandTrajectory: []int{},
		Strengths:      []string{},
		Gaps:           []string{},
		Turns:          []TurnSummary{},
		GeneratedAt:    time.Now(),
		DurationMs:     sess.DurationMs,
	}

	r.buildBandTrajectory(sess)
	r.buildTurnSummaries(turns)
	r.buildAggregateScores(turns)
	r.buildDomainScores(sess, turns)
	r.buildStrengthsAndGaps(sess, turns)
	r.buildDelivery(turns)

	return r
}

func (r *Report) buildBandTrajectory(sess *store.Session) {
	start := sess.DifficultyBand
	if len(sess.BandHistory) > 0 {
		// The first recorded change tells us where the band was before it.
		start = sess.BandHistory[0].Band
		if sess.BandHistory[0].Band > 2 {
			start = sess.BandHistory[0].Band - 1
		}
	}
	r.StartBand = start
	r.BandTrajectory = append(r.BandTrajectory, start)

	for _, change := range sess.BandHistory {
		r.BandTrajectory = append(r.BandTrajectory, change.Band)
	}
	r.EndBand = sess.DifficultyBand
}

func (r *Report) buildTurnSummaries(turns []*store.Turn) {
	for _, t := range turns {
		s := TurnSummary{
			TurnID:     t.ID,
			Index:      t.Index,
			Question:   t.QuestionText,
			HintsUsed:  t.HintsUsed,
			Band:       t.QuestionBand,
			SpanCounts: map[string]int{},
		}
		if t.Evaluation != nil {
			s.Graded = true
			s.Score = t.Evaluation.TurnScore
			for _, span := range t.Evaluation.Spans {
				s.SpanCounts[string(span.Verdict)]++
			}
		}
		r.Turns = append(r.Turns, s)
	}
}

func (r *Report) buildAggregateScores(turns []*store.Turn) {
	var acc struct{ ta, cc, d, st, overall float64 }
	graded := 0

	for _, t := range turns {
		if t.Evaluation == nil {
			continue
		}
		graded++
		acc.ta += float64(t.Evaluation.Scores.TechnicalAccuracy)
		acc.cc += float64(t.Evaluation.Scores.CommunicationClarity)
		acc.d += float64(t.Evaluation.Scores.Depth)
		acc.st += float64(t.Evaluation.Scores.Structure)
		acc.overall += t.Evaluation.TurnScore
	}

	r.TurnsGraded = graded
	if graded == 0 {
		return
	}

	n := float64(graded)
	r.AggregateScores = store.Scores{
		TechnicalAccuracy:    roundToInt(acc.ta / n),
		CommunicationClarity: roundToInt(acc.cc / n),
		Depth:                roundToInt(acc.d / n),
		Structure:            roundToInt(acc.st / n),
	}
	r.OverallScore = round1(acc.overall / n)
}

// buildDomainScores produces the radar chart axes.
//
// Each turn's score is attributed to whichever of the role's domain areas its
// concepts mention. A turn touching two domains contributes to both — an
// interview answer rarely belongs to exactly one area, and forcing a single
// assignment would make the chart arbitrary.
func (r *Report) buildDomainScores(sess *store.Session, turns []*store.Turn) {
	domains := roleDomains(sess.Digest)
	if len(domains) == 0 {
		return
	}

	type acc struct {
		total float64
		count int
	}
	byDomain := make(map[string]*acc, len(domains))
	for _, d := range domains {
		byDomain[d] = &acc{}
	}

	for _, t := range turns {
		if t.Evaluation == nil {
			continue
		}
		concepts := strings.ToLower(strings.Join(append(
			append([]string{}, t.Evaluation.ConceptsDemonstrated...),
			t.Evaluation.ConceptsMissing...), " ") + " " + strings.ToLower(t.QuestionText))

		matched := false
		for _, d := range domains {
			if domainMatches(concepts, d) {
				byDomain[d].total += t.Evaluation.TurnScore
				byDomain[d].count++
				matched = true
			}
		}
		// An unattributable turn still says something about the candidate, so
		// spread it across every axis rather than discarding it. Without this a
		// session whose concepts never name a domain produces an empty chart.
		if !matched {
			for _, d := range domains {
				byDomain[d].total += t.Evaluation.TurnScore
				byDomain[d].count++
			}
		}
	}

	for _, d := range domains {
		a := byDomain[d]
		score := 0.0
		if a.count > 0 {
			score = round1(a.total / float64(a.count))
		}
		r.DomainScores = append(r.DomainScores, DomainScore{
			Domain: d, Score: score, TurnCount: a.count,
		})
	}
}

func (r *Report) buildStrengthsAndGaps(sess *store.Session, turns []*store.Turn) {
	// Proven concepts are the strengths, in the order they were demonstrated.
	for _, c := range sess.Coverage.Proven {
		if len(r.Strengths) >= maxStrengths {
			break
		}
		r.Strengths = append(r.Strengths, c)
	}

	// Gaps are ranked by how often they came up: a concept missed in three
	// turns matters more than one missed once.
	freq := map[string]int{}
	display := map[string]string{}
	for _, t := range turns {
		if t.Evaluation == nil {
			continue
		}
		for _, c := range t.Evaluation.ConceptsMissing {
			key := strings.ToLower(strings.TrimSpace(c))
			if key == "" {
				continue
			}
			freq[key]++
			if _, seen := display[key]; !seen {
				display[key] = c
			}
		}
	}
	for _, c := range sess.Coverage.Missing {
		key := strings.ToLower(strings.TrimSpace(c))
		if key == "" {
			continue
		}
		if _, seen := display[key]; !seen {
			display[key] = c
			freq[key]++
		}
	}

	keys := make([]string, 0, len(freq))
	for k := range freq {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool { return freq[keys[i]] > freq[keys[j]] })

	for _, k := range keys {
		if len(r.Gaps) >= maxGaps {
			break
		}
		r.Gaps = append(r.Gaps, display[k])
	}
}

func (r *Report) buildDelivery(turns []*store.Turn) {
	var totalWords int
	var totalMs int64
	var fillers int
	var hesitationSum float64
	var withAudio int

	for _, t := range turns {
		if t.Delivery == nil {
			continue
		}
		totalWords += t.Delivery.WordCount
		totalMs += t.Delivery.SpeakingTimeMs
		fillers += t.Delivery.FillerCount
		if t.Delivery.SpeakingTimeMs > 0 {
			withAudio++
			hesitationSum += t.Delivery.HesitationScore
		}
		// Carry the most recent turn's coaching forward: it is the most
		// relevant, and stacking six observations is noise.
		if t.Delivery.Observation != "" {
			r.Delivery.Observation = t.Delivery.Observation
		}
		if t.Delivery.Drill != "" {
			r.Delivery.Drill = t.Delivery.Drill
		}
	}

	r.Delivery.FillerTotal = fillers
	r.Delivery.SpeakingTimeMs = totalMs
	r.Delivery.TurnsWithAudio = withAudio

	if totalMs > 0 {
		minutes := float64(totalMs) / 60000.0
		r.Delivery.WPM = round1(float64(totalWords) / minutes)
		r.Delivery.FillerPerMinute = round1(float64(fillers) / minutes)
	}
	if withAudio > 0 {
		r.Delivery.HesitationScore = round1(hesitationSum / float64(withAudio))
	}
	r.Delivery.PaceBand = delivery.PaceBand(r.Delivery.WPM)
}

// --- helpers --------------------------------------------------------------

func roleDomains(digest map[string]any) []string {
	role, ok := digest["role"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := role["domain_areas"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// domainMatches reports whether a turn's concepts relate to a domain area.
//
// Word-level overlap rather than substring containment: "serving" must not
// match "observing", and a domain like "model serving" should match a turn
// about "low-latency serving".
func domainMatches(concepts, domain string) bool {
	// Tokenise the haystack once so comparison is genuinely word-level.
	// strings.Contains would match "serving" inside "observing", attributing a
	// turn about metrics to a domain about model serving.
	haystack := make(map[string]struct{})
	for _, w := range strings.FieldsFunc(strings.ToLower(concepts), isNotLetterOrDigit) {
		haystack[w] = struct{}{}
	}

	for _, word := range strings.FieldsFunc(strings.ToLower(domain), isNotLetterOrDigit) {
		if len(word) < 4 {
			continue // skip "the", "and", "ML" — too common to carry signal
		}
		if _, ok := haystack[word]; ok {
			return true
		}
		// Accept a simple plural or gerund difference, so a domain of "feature
		// pipelines" still matches a concept of "feature pipeline".
		for candidate := range haystack {
			if stemEqual(candidate, word) {
				return true
			}
		}
	}
	return false
}

func isNotLetterOrDigit(r rune) bool {
	return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9')
}

// stemEqual compares two words ignoring a trailing "s" or "es".
func stemEqual(a, b string) bool {
	return trimPlural(a) == trimPlural(b)
}

func trimPlural(s string) string {
	switch {
	case strings.HasSuffix(s, "es") && len(s) > 4:
		return s[:len(s)-2]
	case strings.HasSuffix(s, "s") && len(s) > 3:
		return s[:len(s)-1]
	}
	return s
}

func roundToInt(v float64) int {
	return int(v + 0.5)
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
