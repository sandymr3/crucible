package anchor

import (
	"strings"
	"testing"
)

// A realistic speech transcript: no punctuation to speak of, run-on phrasing,
// which is exactly what the Live API produces and what the evaluator will be
// quoting back at us.
const transcript = "So the ingestion layer used a Kafka topic per source and we deduplicated " +
	"downstream using a bloom filter before the feature store write backpressure was just a " +
	"bigger buffer and we were handling about 2000 requests per second"

func TestExactMatch(t *testing.T) {
	m := Find(transcript, "deduplicated downstream using a bloom filter")

	if m.Tier != TierExact {
		t.Fatalf("tier = %s, want exact", m.Tier)
	}
	if got := transcript[m.Start:m.End]; got != "deduplicated downstream using a bloom filter" {
		t.Errorf("anchored text = %q", got)
	}
}

// The evaluator routinely tidies a transcript when quoting it: adds a capital,
// adds a comma, adds a period. The offsets must still land on the original.
func TestNormalisedMatchSurvivesPunctuationAndCase(t *testing.T) {
	cases := []string{
		"Deduplicated downstream using a bloom filter.",
		"deduplicated, downstream, using a bloom filter",
		"DEDUPLICATED DOWNSTREAM USING A BLOOM FILTER",
		"deduplicated  downstream   using a bloom filter",
	}

	for _, excerpt := range cases {
		m := Find(transcript, excerpt)
		if !m.Found() {
			t.Errorf("%q was dropped", excerpt)
			continue
		}
		got := strings.ToLower(transcript[m.Start:m.End])
		if !strings.Contains(got, "bloom filter") {
			t.Errorf("%q anchored to %q, which does not contain the quoted phrase", excerpt, got)
		}
		if m.Tier == TierDropped {
			t.Errorf("%q resolved to a dropped tier", excerpt)
		}
	}
}

// Near-miss wording — a dropped article, a pluralisation — should still anchor,
// because the alternative is silently losing a legitimate highlight.
func TestFuzzyMatchToleratesSmallDrift(t *testing.T) {
	cases := []string{
		"deduplicated downstream using bloom filter",    // dropped article
		"deduplicated downstream using a bloom filters", // pluralised
	}

	for _, excerpt := range cases {
		m := Find(transcript, excerpt)
		if !m.Found() {
			t.Errorf("%q was dropped, expected a fuzzy match", excerpt)
			continue
		}
		if !strings.Contains(strings.ToLower(m.Text), "bloom") {
			t.Errorf("%q anchored to %q", excerpt, m.Text)
		}
	}
}

// The most important property in this package. Anchoring a paraphrase onto
// unrelated text attaches a verdict to words that do not support it — the user
// sees "incorrect" highlighted over a sentence that never made that claim.
// Dropping is always the safer failure.
func TestParaphraseIsDroppedRatherThanMisplaced(t *testing.T) {
	paraphrases := []string{
		"the candidate used probabilistic deduplication techniques",
		"they employed a data structure for set membership testing",
		"something about message queues and consumer groups",
		"the system was highly available and fault tolerant",
	}

	for _, p := range paraphrases {
		if m := Find(transcript, p); m.Found() {
			t.Errorf("paraphrase %q was anchored to %q (tier %s); it should have been dropped",
				p, m.Text, m.Tier)
		}
	}
}

func TestEmptyInputsAreDropped(t *testing.T) {
	for _, tc := range []struct{ transcript, excerpt string }{
		{transcript, ""},
		{transcript, "   "},
		{"", "bloom filter"},
		{"", ""},
	} {
		if m := Find(tc.transcript, tc.excerpt); m.Found() {
			t.Errorf("Find(%q, %q) matched, want dropped", tc.transcript, tc.excerpt)
		}
	}
}

// Short excerpts must not go through fuzzy matching: with a minimum edit
// budget of 1, a six-character string finds a "match" almost anywhere.
func TestShortExcerptsDoNotFuzzyMatch(t *testing.T) {
	if m := Find(transcript, "xyzab"); m.Found() {
		t.Errorf("short nonsense excerpt anchored to %q", m.Text)
	}
}

// Offsets must be valid slice bounds on the ORIGINAL transcript. An off-by-one
// here produces a highlight that starts mid-word, which is the exact failure
// this package exists to prevent.
func TestOffsetsAreValidSliceBounds(t *testing.T) {
	excerpts := []string{
		"Kafka topic per source",
		"backpressure was just a bigger buffer.",
		"about 2000 requests per second",
		"So the ingestion layer",
	}

	for _, e := range excerpts {
		m := Find(transcript, e)
		if !m.Found() {
			continue
		}
		if m.Start < 0 || m.End > len(transcript) || m.Start >= m.End {
			t.Errorf("%q produced invalid bounds [%d,%d) for a %d-byte transcript",
				e, m.Start, m.End, len(transcript))
			continue
		}
		if transcript[m.Start:m.End] != m.Text {
			t.Errorf("%q: Text %q disagrees with the slice at its own offsets %q",
				e, m.Text, transcript[m.Start:m.End])
		}
		// A highlight must not begin or end mid-word.
		if m.Start > 0 && !isSpace(transcript[m.Start-1]) {
			t.Errorf("%q starts mid-word: ...%q", e, transcript[max(0, m.Start-6):m.End])
		}
	}
}

func TestMatchAtTranscriptEnd(t *testing.T) {
	// Off-by-one errors concentrate at the boundary.
	m := Find(transcript, "2000 requests per second")
	if !m.Found() {
		t.Fatal("trailing excerpt was dropped")
	}
	if m.End != len(transcript) {
		t.Errorf("End = %d, want %d (the transcript's length)", m.End, len(transcript))
	}
}

func TestMatchAtTranscriptStart(t *testing.T) {
	m := Find(transcript, "So the ingestion layer")
	if !m.Found() {
		t.Fatal("leading excerpt was dropped")
	}
	if m.Start != 0 {
		t.Errorf("Start = %d, want 0", m.Start)
	}
}

func TestStatsTrackDropRate(t *testing.T) {
	var s Stats
	s.Record(TierExact)
	s.Record(TierNormalised)
	s.Record(TierFuzzy)
	s.Record(TierDropped)

	if s.Total != 4 {
		t.Errorf("Total = %d, want 4", s.Total)
	}
	if got := s.DropRate(); got != 0.25 {
		t.Errorf("DropRate = %v, want 0.25", got)
	}
	// A drop rate above ~20% means the evaluator is paraphrasing rather than
	// quoting, which is a prompt problem, not a matcher problem.
	if s.DropRate() <= 0.20 {
		t.Error("expected this sample to exceed the 20% investigate threshold")
	}
}

func TestEmptyStatsDoNotDivideByZero(t *testing.T) {
	var s Stats
	if got := s.DropRate(); got != 0 {
		t.Errorf("DropRate on empty stats = %v, want 0", got)
	}
}

func TestUnicodeTranscriptDoesNotCorruptOffsets(t *testing.T) {
	// Byte offsets over multi-byte runes are a classic source of mid-character
	// slicing, which panics or renders mojibake.
	tr := "we used a naïve approach then switched to a bloom filter for dedup"
	m := Find(tr, "bloom filter")
	if !m.Found() {
		t.Fatal("excerpt after a multi-byte rune was dropped")
	}
	if tr[m.Start:m.End] != "bloom filter" {
		t.Errorf("anchored to %q, want %q", tr[m.Start:m.End], "bloom filter")
	}
}
