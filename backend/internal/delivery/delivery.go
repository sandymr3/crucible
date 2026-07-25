// Package delivery measures how an answer sounded.
//
// READ THIS BEFORE CHANGING ANYTHING HERE.
//
// Google's speech recognition normalises disfluencies out of the transcript.
// "Um", "uh", and false starts are removed as noise — correct behaviour for
// dictation, fatal for this feature. A filler counter built on a regex over the
// Live API transcript will ship reading permanently zero, and nobody notices
// until demo day (PRD §13.1, risk R7).
//
// So the inferred metrics come from the ANSWER AUDIO, not the transcript.
//
// The split is deliberate:
//   - Deterministic in Go: words per minute, speaking time, word count.
//     Never ask a model for arithmetic.
//   - Inferred from audio: filler instances, hesitation, pace character.
//     Things a transcript cannot carry.
package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/santh/crucible/internal/audio"
	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/prompts"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/vertexai"
)

// Analyser produces delivery metrics.
type Analyser struct {
	cfg *config.Config
	log *slog.Logger
	vx  *vertexai.Client
}

// New builds the analyser.
func New(cfg *config.Config, log *slog.Logger, vx *vertexai.Client) *Analyser {
	return &Analyser{cfg: cfg, log: log, vx: vx}
}

// Input is one turn's answer.
type Input struct {
	TurnID     string
	Transcript string
	// AudioURI is a gs:// URI to the answer WAV. Without it only the
	// deterministic metrics are available.
	AudioURI        string
	AudioDurationMs int64
}

// Pace bands (PRD §13.3). Optimal pace is context-dependent — a system-design
// walkthrough should be slower than a behavioural story — so these are guidance
// rather than a grade.
const (
	PaceHesitant = "hesitant"
	PaceOptimal  = "optimal"
	PaceRushed   = "rushed"
	PaceTooFast  = "too fast"
)

// PaceBand classifies words per minute.
func PaceBand(wpm float64) string {
	switch {
	case wpm < 110:
		return PaceHesitant
	case wpm <= 160:
		return PaceOptimal
	case wpm <= 190:
		return PaceRushed
	default:
		return PaceTooFast
	}
}

// Deterministic computes everything that can be computed rather than inferred.
//
// Always available, even with no audio and no model call. If the inferred half
// fails, the report still shows real numbers.
func Deterministic(transcript string, audioDurationMs int64) store.Delivery {
	words := len(strings.Fields(transcript))

	d := store.Delivery{
		WordCount:      words,
		SpeakingTimeMs: audioDurationMs,
	}
	if audioDurationMs > 0 {
		d.WPM = float64(words) / (float64(audioDurationMs) / 60000.0)
	}
	return d
}

// analysisTimeout bounds the call. This runs on a worker well after the turn
// has ended, so it is allowed to be slower than the grader.
const analysisTimeout = 90 * time.Second

// Analyse produces full delivery metrics, degrading to the deterministic subset
// when audio is unavailable or the model call fails.
//
// Never returns an error for a missing-audio case: delivery is enrichment, and
// a turn typed rather than spoken is a legitimate state, not a failure.
func (a *Analyser) Analyse(ctx context.Context, in Input) (store.Delivery, error) {
	base := Deterministic(in.Transcript, in.AudioDurationMs)

	if in.AudioURI == "" {
		// A typed answer, or audio we failed to upload. Deterministic metrics
		// are still meaningful for the former and honest for the latter.
		return base, nil
	}

	p, err := prompts.Get(prompts.DeliveryAnalysis)
	if err != nil {
		return base, err
	}

	// Too short to say anything useful about, and a model asked to analyse two
	// seconds of audio will invent a pattern rather than decline.
	const minAnalysableMs = 2000
	if in.AudioDurationMs > 0 && in.AudioDurationMs < minAnalysableMs {
		base.Observation = "Too short to analyse delivery."
		return base, nil
	}

	ctx, cancel := context.WithTimeout(ctx, analysisTimeout)
	defer cancel()

	temp := float32(0.2)
	raw, err := a.vx.GenerateStructured(ctx, a.cfg.ModelReasoning,
		[]*genai.Content{{Role: "user", Parts: []*genai.Part{
			// The audio part comes first so the model treats the instruction as
			// being about it rather than the other way round.
			{FileData: &genai.FileData{FileURI: in.AudioURI, MIMEType: "audio/wav"}},
			{Text: p.Text},
		}}},
		&genai.GenerateContentConfig{
			Temperature:      &temp,
			ResponseMIMEType: "application/json",
			ResponseSchema:   deliverySchema(),
		})
	if err != nil {
		// Degrade rather than fail: the deterministic numbers are still true.
		return base, fmt.Errorf("delivery: audio analysis failed: %w", err)
	}

	var parsed rawDelivery
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return base, fmt.Errorf("delivery: response was not valid JSON: %w", err)
	}

	base.FillerInstances = trimList(parsed.FillerInstances, 40)
	// Trust the list length over the model's own count: the two disagree often
	// enough, and the list is the thing we can actually show the user.
	base.FillerCount = len(base.FillerInstances)
	if base.FillerCount == 0 && parsed.FillerCount > 0 {
		base.FillerCount = parsed.FillerCount
	}
	base.HesitationScore = clamp01(parsed.HesitationScore)
	base.Observation = strings.TrimSpace(parsed.Observation)
	base.Drill = strings.TrimSpace(parsed.Drill)

	a.log.Info("delivery analysed",
		"turn_id", in.TurnID,
		"wpm", fmt.Sprintf("%.0f", base.WPM),
		"pace", PaceBand(base.WPM),
		"fillers", base.FillerCount,
		"hesitation", fmt.Sprintf("%.2f", base.HesitationScore),
		"audio_ms", in.AudioDurationMs)

	return base, nil
}

type rawDelivery struct {
	FillerInstances []string `json:"filler_instances"`
	FillerCount     int      `json:"filler_count"`
	HesitationScore float64  `json:"hesitation_score"`
	PaceNote        string   `json:"pace_note"`
	Observation     string   `json:"observation"`
	Drill           string   `json:"drill"`
}

func deliverySchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"filler_instances": {
				Type:        genai.TypeArray,
				Description: "Every filler actually heard, one entry per occurrence, not per unique word.",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
			"filler_count": {Type: genai.TypeInteger, Description: "Length of filler_instances."},
			"hesitation_score": {
				Type:        genai.TypeNumber,
				Description: "0.0 fluent to 1.0 hesitant throughout. Deliberate thinking pauses are not hesitation.",
			},
			"pace_note":   {Type: genai.TypeString, Description: "Short phrase on speaking rate."},
			"observation": {Type: genai.TypeString, Description: "One sentence. Counts and patterns only, never character judgements."},
			"drill":       {Type: genai.TypeString, Description: "One concrete thing to practise on the very next question."},
		},
		Required: []string{"filler_instances", "filler_count", "hesitation_score", "pace_note", "observation", "drill"},
	}
}

func trimList(items []string, max int) []string {
	out := make([]string, 0, len(items))
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

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ensure the audio package stays referenced for the sample-rate constants used
// by callers computing AudioDurationMs.
var _ = audio.SampleRateIn
