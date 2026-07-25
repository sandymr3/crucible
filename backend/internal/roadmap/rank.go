// Package roadmap turns a session's gaps into a day-by-day study plan with
// real, checked resource links.
//
// The ranking half is pure and deterministic. The generation half makes exactly
// one grounded model call for the whole plan — not one per day — and then
// verifies every URL it returns over HTTP before showing it to anyone.
package roadmap

import (
	"sort"
	"strings"

	"github.com/santh/crucible/internal/store"
)

// Relevance multipliers (PRD §14.1). A gap the job description names as
// required matters far more than one it never mentions.
const (
	RelevanceMustHave    = 2.0
	RelevanceNiceToHave  = 1.3
	RelevanceUnmentioned = 1.0
)

// Cluster is one deduplicated concept gap.
type Cluster struct {
	// Label is the clearest spelling seen, shown to the user.
	Label string
	// Members are the raw concept strings folded into this cluster, kept so a
	// surprising ranking can be explained.
	Members []string
	// Frequency is how many turns raised it. A gap that recurred matters more
	// than one that appeared once.
	Frequency int
	// Severity is how badly it went where it appeared, 0..1.
	Severity float64
	// Relevance is the JD multiplier.
	Relevance float64
	// Score is frequency x severity x relevance.
	Score float64
}

// Input is everything the ranker needs.
type Input struct {
	Turns    []*store.Turn
	Coverage store.Coverage
	Digest   map[string]any
	// HorizonDays is how long the candidate has. Drives how many concepts make
	// the cut.
	HorizonDays int
}

// Rank clusters the session's missing concepts and orders them for study.
//
// Ordering is by PREREQUISITE, not by score (PRD §14.1). Learning is ordered
// even when priorities are not: there is no point studying KV caching on day one
// if attention mechanics land on day four.
func Rank(in Input) []Cluster {
	clusters := cluster(in)
	if len(clusters) == 0 {
		return nil
	}

	mustHaves, niceToHaves := jdRequirements(in.Digest)
	for i := range clusters {
		clusters[i].Relevance = relevanceFor(clusters[i], mustHaves, niceToHaves)
		clusters[i].Score = float64(clusters[i].Frequency) *
			clusters[i].Severity * clusters[i].Relevance
	}

	// Rank by score to decide WHICH concepts make the cut...
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].Score != clusters[j].Score {
			return clusters[i].Score > clusters[j].Score
		}
		return clusters[i].Label < clusters[j].Label
	})

	if n := maxConcepts(in.HorizonDays); len(clusters) > n {
		clusters = clusters[:n]
	}

	// ...then reorder the survivors by dependency for the actual plan.
	return orderByPrerequisite(clusters)
}

// maxConcepts is roughly 1.5 per available day, bounded so a two-week horizon
// does not produce a plan nobody will read.
func maxConcepts(horizonDays int) int {
	if horizonDays <= 0 {
		horizonDays = 7
	}
	n := horizonDays * 3 / 2
	if n < 3 {
		n = 3
	}
	if n > 12 {
		n = 12
	}
	return n
}

// cluster folds near-duplicate concepts together.
//
// "backpressure", "flow control", and "producer throttling" are one thing to
// study, and presenting them as three days of work is both wrong and
// demoralising.
func cluster(in Input) []Cluster {
	type acc struct {
		label    string
		members  []string
		freq     int
		sevSum   float64
		sevCount int
	}
	byKey := map[string]*acc{}
	var order []string

	add := func(concept string, severity float64) {
		concept = strings.TrimSpace(concept)
		if concept == "" {
			return
		}
		key := clusterKey(concept)
		a, ok := byKey[key]
		if !ok {
			a = &acc{label: concept}
			byKey[key] = a
			order = append(order, key)
		}
		a.freq++
		a.sevSum += severity
		a.sevCount++
		if !containsFold(a.members, concept) {
			a.members = append(a.members, concept)
		}
		// Prefer the shortest spelling as the label: it is usually the
		// canonical term rather than a sentence describing it.
		if len(concept) < len(a.label) {
			a.label = concept
		}
	}

	for _, t := range in.Turns {
		if t.Evaluation == nil {
			continue
		}
		// Severity from the turn's score: a gap in an answer that scored 2 is
		// more urgent than the same gap in one that scored 8.
		severity := 1.0 - (t.Evaluation.TurnScore / 10.0)
		if severity < 0.2 {
			severity = 0.2
		}
		if severity > 1.0 {
			severity = 1.0
		}
		for _, c := range t.Evaluation.ConceptsMissing {
			add(c, severity)
		}
		// An 'incomplete' span is a partial gap, and worth less than a full miss.
		for _, s := range t.Evaluation.Spans {
			if s.Verdict == store.VerdictIncomplete && s.Concept != "" {
				add(s.Concept, severity*0.6)
			}
		}
	}

	// Session-level coverage catches anything the per-turn lists missed.
	for _, c := range in.Coverage.Missing {
		add(c, 0.5)
	}

	// Never study something already demonstrated.
	proven := map[string]bool{}
	for _, p := range in.Coverage.Proven {
		proven[clusterKey(p)] = true
	}

	out := make([]Cluster, 0, len(order))
	for _, key := range order {
		if proven[key] {
			continue
		}
		a := byKey[key]
		sev := 0.5
		if a.sevCount > 0 {
			sev = a.sevSum / float64(a.sevCount)
		}
		out = append(out, Cluster{
			Label: a.label, Members: a.members,
			Frequency: a.freq, Severity: sev,
		})
	}
	return out
}

// clusterKey normalises a concept for deduplication.
//
// Strips punctuation, lowercases, drops stopwords and parenthetical asides, and
// sorts the remaining significant words — so "flow control signalling" and
// "signalling for flow control" collapse to one key.
func clusterKey(s string) string {
	s = strings.ToLower(s)
	if i := strings.Index(s, "("); i > 0 {
		s = s[:i] // drop "(e.g. token bucket, leaky bucket)"
	}

	words := strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})

	significant := make([]string, 0, len(words))
	for _, w := range words {
		// Only single characters are dropped. Two-character tokens carry real
		// meaning in this domain — IO, ML, GC, OS, TS — and discarding them
		// merges genuinely different concepts into one study day.
		if stopwords[w] || len(w) < 2 {
			continue
		}
		significant = append(significant, singular(w))
	}
	if len(significant) == 0 {
		return strings.TrimSpace(s)
	}
	sort.Strings(significant)
	return strings.Join(significant, " ")
}

var stopwords = map[string]bool{
	"and": true, "the": true, "for": true, "with": true, "from": true,
	"into": true, "over": true, "under": true, "using": true, "via": true,
	"vs": true, "versus": true, "its": true, "their": true, "how": true,
}

func singular(w string) string {
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "es") && len(w) > 4:
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && len(w) > 3:
		return w[:len(w)-1]
	}
	return w
}

// relevanceFor applies the JD multiplier.
func relevanceFor(c Cluster, mustHaves, niceToHaves []string) float64 {
	for _, m := range mustHaves {
		if conceptsOverlap(c.Label, m) {
			return RelevanceMustHave
		}
	}
	for _, n := range niceToHaves {
		if conceptsOverlap(c.Label, n) {
			return RelevanceNiceToHave
		}
	}
	return RelevanceUnmentioned
}

// conceptsOverlap reports whether two phrases share a significant word.
func conceptsOverlap(a, b string) bool {
	setA := map[string]bool{}
	for _, w := range strings.Fields(clusterKey(a)) {
		setA[w] = true
	}
	for _, w := range strings.Fields(clusterKey(b)) {
		if setA[w] {
			return true
		}
	}
	return false
}

// orderByPrerequisite sorts concepts into a teachable sequence.
//
// A dependency graph would be better, but nothing in the pipeline produces one.
// This uses a foundational-terms heuristic instead: concepts naming primitives
// come before concepts naming systems built from them. It is approximate, and
// deliberately stable so the same session always produces the same plan.
func orderByPrerequisite(clusters []Cluster) []Cluster {
	depth := func(c Cluster) int {
		l := strings.ToLower(c.Label)
		for _, term := range foundational {
			if strings.Contains(l, term) {
				return 0
			}
		}
		for _, term := range advanced {
			if strings.Contains(l, term) {
				return 2
			}
		}
		return 1
	}

	sort.SliceStable(clusters, func(i, j int) bool {
		di, dj := depth(clusters[i]), depth(clusters[j])
		if di != dj {
			return di < dj
		}
		// Within a tier, highest priority first.
		return clusters[i].Score > clusters[j].Score
	})
	return clusters
}

var foundational = []string{
	"basic", "fundamental", "definition", "primitive", "data structure",
	"algorithm", "complexity", "syntax", "type", "memory", "buffer", "queue",
}

var advanced = []string{
	"distributed", "consensus", "scal", "shard", "partition", "failover",
	"consistency", "tradeoff", "architecture", "optimis", "optimiz", "tuning",
}

// jdRequirements pulls must-haves and nice-to-haves out of the digest.
func jdRequirements(digest map[string]any) (must, nice []string) {
	role, ok := digest["role"].(map[string]any)
	if !ok {
		return nil, nil
	}
	return stringSlice(role["must_haves"]), stringSlice(role["nice_to_haves"])
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
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

func containsFold(items []string, s string) bool {
	for _, item := range items {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}
