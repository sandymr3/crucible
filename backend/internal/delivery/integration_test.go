//go:build integration

// Live delivery-analysis tests.
//
//	go test ./internal/delivery/ -tags=integration -v -timeout=10m
//
// The filler-count test guards PRD risk R7, which is rated "High if
// unaddressed" for a reason: the failure mode is a counter that reads zero
// forever and looks like a working feature.
package delivery

import (
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/santh/crucible/internal/blob"
	"github.com/santh/crucible/internal/config"
)

func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic("chdir to module root: " + err.Error())
	}
	_ = config.LoadDotEnv(".env")
	os.Exit(m.Run())
}

var (
	setupOnce sync.Once
	analyser  *Analyser
	blobStore *blob.Store
	setupErr  error
)

func liveAnalyser(t *testing.T) (*Analyser, *blob.Store) {
	t.Helper()

	setupOnce.Do(func() {
		cfg, err := config.Load()
		if err != nil {
			setupErr = err
			return
		}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))

		vx, err := newVertex(cfg, log)
		if err != nil {
			setupErr = err
			return
		}
		bl, err := blob.New(context.Background(), cfg.GCSBucket)
		if err != nil {
			setupErr = err
			return
		}
		analyser, blobStore = New(cfg, log, vx), bl
	})

	if setupErr != nil {
		t.Skipf("no live access, skipping: %v", setupErr)
	}
	return analyser, blobStore
}

// uploadFixture puts a local WAV in GCS and returns its gs:// URI.
func uploadFixture(t *testing.T, bl *blob.Store, path string) (string, int64) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Skipf("fixture %s missing; generate it with cmd/livespike: %v", path, err)
	}
	defer f.Close()

	info, _ := f.Stat()
	uri, err := bl.Upload(context.Background(), "testdata/"+strings.TrimPrefix(path, "testdata/out/"),
		"audio/wav", f, info.Size()+1)
	if err != nil {
		t.Fatalf("uploading fixture: %v", err)
	}

	// WAV header is 44 bytes; the rest is PCM16 at 24 kHz as written by
	// livespike.
	durationMs := (info.Size() - 44) * 1000 / (24000 * 2)
	return uri, durationMs
}

// transcriptFillerRegex is the approach PRD §13.1 warns against. It exists here
// only so the test can demonstrate the trap rather than merely describe it.
var transcriptFillerRegex = regexp.MustCompile(`(?i)\b(um+|uh+|er+|hmm+)\b`)

// R7. The counter must be non-zero on genuinely disfluent speech.
func TestDisfluentAudioProducesNonZeroFillerCount(t *testing.T) {
	a, bl := liveAnalyser(t)
	uri, durationMs := uploadFixture(t, bl, "testdata/out/disfluent.wav")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// The REAL transcript the Live API produced for this exact audio, captured
	// verbatim from a session. Do not replace this with a tidied version — the
	// whole point is to compare against what the transcript actually contains.
	//
	// PRD §13.1 asserts that Google's speech recognition normalises
	// disfluencies out, so a transcript regex "will ship a counter that always
	// reads zero". MEASURED HERE, THAT IS NOT TRUE: the Live API's input
	// transcription preserves fillers. See the assertions below for what this
	// means for the feature.
	realTranscript := "So um, back pressure, uh, we'd sort of like handled it at the um Q level, " +
		"I think and uh you know, we had like some monitoring um set up. It uh basically worked. " +
		"I mean mostly, I'm fine."

	got, err := a.Analyse(ctx, Input{
		TurnID:          "r7",
		Transcript:      realTranscript,
		AudioURI:        uri,
		AudioDurationMs: durationMs,
	})
	if err != nil {
		t.Fatalf("Analyse failed: %v", err)
	}

	t.Logf("audio=%dms wpm=%.0f pace=%s fillers=%d hesitation=%.2f",
		durationMs, got.WPM, PaceBand(got.WPM), got.FillerCount, got.HesitationScore)
	t.Logf("  instances: %v", got.FillerInstances)
	t.Logf("  observation: %s", got.Observation)
	t.Logf("  drill: %s", got.Drill)

	fromTranscript := len(transcriptFillerRegex.FindAllString(realTranscript, -1))
	t.Logf("  a transcript regex over the REAL transcript would count: %d hard fillers", fromTranscript)

	// R7 proper: the counter must not read zero on disfluent speech.
	if got.FillerCount == 0 {
		t.Error("filler count is ZERO on deliberately disfluent audio — this is exactly R7, " +
			"the counter that ships reading zero forever")
	}
	// The audio must be at least as informative as the transcript. It is
	// allowed to be close on the raw COUNT — the PRD's premise that the
	// transcript is stripped did not hold — but it must not be worse.
	if got.FillerCount < fromTranscript {
		t.Errorf("audio analysis found %d fillers but the transcript alone shows %d; "+
			"the audio call is not earning its cost", got.FillerCount, fromTranscript)
	}
	// This is what actually justifies the audio call: prosodic signal that no
	// transcript can carry, at any level of quality.
	if got.HesitationScore == 0 {
		t.Error("hesitation score is 0 on audibly hesitant speech; " +
			"this is the signal a transcript genuinely cannot provide")
	}
	if got.Observation == "" {
		t.Error("no observation returned")
	}
	if got.Drill == "" {
		t.Error("no drill returned; an observation without a drill is not coaching")
	}
}

// Delivery feedback is where a coaching tool most easily becomes cruel.
func TestObservationReportsBehaviourNotCharacter(t *testing.T) {
	a, bl := liveAnalyser(t)
	uri, durationMs := uploadFixture(t, bl, "testdata/out/disfluent.wav")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	got, err := a.Analyse(ctx, Input{
		TurnID: "tone", Transcript: "placeholder", AudioURI: uri, AudioDurationMs: durationMs,
	})
	if err != nil {
		t.Fatalf("Analyse failed: %v", err)
	}

	text := strings.ToLower(got.Observation + " " + got.Drill)
	for _, banned := range []string{
		"unconfident", "not confident", "lack confidence", "nervous",
		"insecure", "weak speaker", "poor communicator", "you seem",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("delivery feedback made a character judgement (%q): %s", banned, got.Observation)
		}
	}
}

// A typed answer has no audio. That is a legitimate state, not a failure, and
// the deterministic metrics must still be right.
func TestNoAudioDegradesToDeterministicMetrics(t *testing.T) {
	a, _ := liveAnalyser(t)

	got, err := a.Analyse(context.Background(), Input{
		TurnID:          "typed",
		Transcript:      "one two three four five six seven eight nine ten",
		AudioDurationMs: 30000, // 30s
	})
	if err != nil {
		t.Fatalf("Analyse with no audio returned an error: %v", err)
	}
	if got.WordCount != 10 {
		t.Errorf("WordCount = %d, want 10", got.WordCount)
	}
	// 10 words in 0.5 minutes = 20 wpm.
	if got.WPM < 19 || got.WPM > 21 {
		t.Errorf("WPM = %v, want ~20", got.WPM)
	}
	if got.FillerCount != 0 {
		t.Errorf("FillerCount = %d with no audio, want 0 rather than a guess", got.FillerCount)
	}
}
