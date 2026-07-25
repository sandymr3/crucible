// Package anchor locates evaluator-supplied excerpts inside a transcript.
//
// The model is never asked for character offsets. Language models are
// unreliable at them — roughly a third come back off by a few characters, and
// the resulting highlights land mid-word. Instead the evaluator returns the
// verbatim excerpt text and this package finds it, server-side, deterministically.
//
// Four tiers, in order of confidence. If all of them fail the span is dropped
// silently: a missing highlight is invisible to the user, whereas a
// mis-positioned one is a bug the judge sees on screen.
package anchor

import (
	"strings"
	"unicode"
)

// Tier records which strategy resolved a span, for the drop-rate metric and
// for debugging a heatmap that looks wrong.
type Tier string

const (
	TierExact      Tier = "exact"      // byte-for-byte substring
	TierNormalised Tier = "normalised" // case- and punctuation-insensitive
	TierFuzzy      Tier = "fuzzy"      // token-level Levenshtein over a window
	TierDropped    Tier = "dropped"    // could not be located
)

// Match is a resolved span position.
type Match struct {
	// Start and End are byte offsets into the original transcript, forming a
	// half-open range suitable for slicing.
	Start int
	End   int
	Tier  Tier
	// Text is the transcript's own wording at that range, which may differ
	// from the requested excerpt when a fuzzy tier matched. The UI must
	// highlight the transcript's text, not the model's paraphrase of it.
	Text string
}

// Found reports whether the excerpt was located.
func (m Match) Found() bool { return m.Tier != TierDropped }

// maxFuzzyDistanceRatio is the edit-distance budget for tier 3, as a fraction
// of the excerpt length.
//
// 0.15 accommodates a dropped article or a singular/plural slip while still
// rejecting a genuine paraphrase. Set it much higher and the anchor lands on
// text that does not support the verdict attached to it.
const maxFuzzyDistanceRatio = 0.15

// minFuzzyLen is the shortest excerpt eligible for fuzzy matching. Below this,
// an edit-distance budget is large enough relative to the string that matches
// become arbitrary — a 6-character excerpt with 1 edit allowed will find
// something almost anywhere.
const minFuzzyLen = 12

// Find locates excerpt within transcript.
func Find(transcript, excerpt string) Match {
	excerpt = strings.TrimSpace(excerpt)
	if excerpt == "" || transcript == "" {
		return Match{Tier: TierDropped}
	}

	// Tier 1: exact. Covers the large majority when the prompt insists on
	// character-for-character quoting.
	if i := strings.Index(transcript, excerpt); i >= 0 {
		return Match{Start: i, End: i + len(excerpt), Tier: TierExact, Text: excerpt}
	}

	// Tier 2: normalised. Catches differences in case, punctuation, and
	// whitespace — overwhelmingly the model tidying up a transcript that had
	// none of that to begin with.
	if m, ok := findNormalised(transcript, excerpt); ok {
		return m
	}

	// Tier 3: fuzzy, over a sliding window of tokens.
	if len([]rune(excerpt)) >= minFuzzyLen {
		if m, ok := findFuzzy(transcript, excerpt); ok {
			return m
		}
	}

	return Match{Tier: TierDropped}
}

// --- Tier 2: normalised ---------------------------------------------------

// findNormalised searches a lowercased, punctuation-stripped projection of the
// transcript while keeping a map back to original byte offsets, so the returned
// range still indexes the untouched transcript.
func findNormalised(transcript, excerpt string) (Match, bool) {
	normT, offsets := normalise(transcript)
	normE, _ := normalise(excerpt)
	if normE == "" {
		return Match{}, false
	}

	i := strings.Index(normT, normE)
	if i < 0 {
		return Match{}, false
	}

	start := offsets[i]
	// The end offset is the start of the character after the match; when the
	// match runs to the end of the normalised string there is no such
	// character, so fall back to the transcript length.
	end := len(transcript)
	if i+len(normE) < len(offsets) {
		end = offsets[i+len(normE)]
	}

	// Trim trailing whitespace that the normalised projection may have pulled
	// in, so the highlight does not extend past the words it covers.
	for end > start && isSpace(transcript[end-1]) {
		end--
	}

	return Match{Start: start, End: end, Tier: TierNormalised, Text: transcript[start:end]}, true
}

// normalise lowercases, drops punctuation, and collapses whitespace runs to a
// single space. offsets[i] is the byte offset in the original string of the
// character that produced normalised byte i.
func normalise(s string) (string, []int) {
	var b strings.Builder
	offsets := make([]int, 0, len(s))
	lastWasSpace := true // leading whitespace is dropped

	for i, r := range s {
		switch {
		case unicode.IsSpace(r):
			if !lastWasSpace {
				b.WriteByte(' ')
				offsets = append(offsets, i)
				lastWasSpace = true
			}
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			// Dropped entirely: the model routinely adds or removes commas and
			// periods when quoting a speech transcript.
		default:
			lower := unicode.ToLower(r)
			n := b.Len()
			b.WriteRune(lower)
			for range b.Len() - n {
				offsets = append(offsets, i)
			}
			lastWasSpace = false
		}
	}

	out := b.String()
	// Drop a trailing space so an excerpt ending mid-sentence still matches.
	// offsets must shrink in step, or every later lookup is off by one.
	for len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
		offsets = offsets[:len(offsets)-1]
	}
	return out, offsets
}

// --- Tier 3: fuzzy --------------------------------------------------------

type token struct {
	text       string // normalised
	start, end int    // byte offsets in the original transcript
}

// findFuzzy slides a window of transcript tokens the same length as the
// excerpt's token count and keeps the best Levenshtein match within budget.
func findFuzzy(transcript, excerpt string) (Match, bool) {
	tTokens := tokenise(transcript)
	eTokens := tokenise(excerpt)
	if len(eTokens) == 0 || len(tTokens) < len(eTokens) {
		return Match{}, false
	}

	target := joinTokens(eTokens)
	budget := int(float64(len([]rune(target))) * maxFuzzyDistanceRatio)
	if budget < 1 {
		budget = 1
	}

	best, bestWidth := -1, 0
	bestDist := budget + 1

	// Try windows slightly shorter and longer than the excerpt too: speech
	// transcripts routinely gain or lose a filler word inside a quoted span.
	for width := max(1, len(eTokens)-1); width <= len(eTokens)+1; width++ {
		for i := 0; i+width <= len(tTokens); i++ {
			candidate := joinTokens(tTokens[i : i+width])
			d := levenshtein([]rune(candidate), []rune(target), bestDist)
			if d < bestDist {
				bestDist, best, bestWidth = d, i, width
			}
		}
	}

	if best < 0 {
		return Match{}, false
	}
	start := tTokens[best].start
	end := tTokens[best+bestWidth-1].end
	return Match{Start: start, End: end, Tier: TierFuzzy, Text: transcript[start:end]}, true
}

func tokenise(s string) []token {
	var out []token
	start := -1
	for i := 0; i <= len(s); i++ {
		atEnd := i == len(s)
		if !atEnd && !isSpace(s[i]) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			word := strings.Map(func(r rune) rune {
				if unicode.IsPunct(r) || unicode.IsSymbol(r) {
					return -1
				}
				return unicode.ToLower(r)
			}, s[start:i])
			if word != "" {
				out = append(out, token{text: word, start: start, end: i})
			}
			start = -1
		}
	}
	return out
}

func joinTokens(ts []token) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.text
	}
	return strings.Join(parts, " ")
}

// levenshtein computes edit distance, abandoning early once every value in a
// row exceeds the cutoff. The early exit matters: this runs across every
// window of a transcript for every span, on the post-turn path that has a
// four-second budget.
func levenshtein(a, b []rune, cutoff int) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	if abs(len(a)-len(b)) > cutoff {
		return cutoff + 1
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		rowMin := curr[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(min(curr[j-1]+1, prev[j]+1), prev[j-1]+cost)
			if curr[j] < rowMin {
				rowMin = curr[j]
			}
		}
		if rowMin > cutoff {
			return cutoff + 1
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// --- Batch ----------------------------------------------------------------

// Stats summarises a batch anchoring pass.
//
// The drop rate is a quality signal worth watching: above roughly 20% the
// evaluator is paraphrasing rather than quoting, and the fix is to tighten the
// prompt rather than to loosen the matcher.
type Stats struct {
	Total      int
	Exact      int
	Normalised int
	Fuzzy      int
	Dropped    int
}

// DropRate returns the fraction of spans that could not be anchored.
func (s Stats) DropRate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Dropped) / float64(s.Total)
}

// Record folds one match into the stats.
func (s *Stats) Record(t Tier) {
	s.Total++
	switch t {
	case TierExact:
		s.Exact++
	case TierNormalised:
		s.Normalised++
	case TierFuzzy:
		s.Fuzzy++
	default:
		s.Dropped++
	}
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
