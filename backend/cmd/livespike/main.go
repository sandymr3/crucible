// Command livespike proves the Vertex Live bidirectional audio loop end to end
// with nothing else in the way — no HTTP server, no Firestore, no auth.
//
// This is Phase 1 of the build plan and it decides the project: everything
// downstream assumes speech-to-speech works through our own stack. It stays in
// the tree after the spike because it remains the fastest way to answer "is the
// Live API itself broken, or is it our relay?"
//
// Two modes:
//
//	-mode=speak   send text, receive audio. Proves the downstream path and
//	              writes a WAV you can use as a test fixture.
//	-mode=listen  send a WAV as user speech, receive audio + transcripts.
//	              Proves the full duplex loop.
//
// The bootstrap trick: -mode=speak generates real speech audio, which
// -mode=listen then feeds back as the user's voice. That gives us a speech
// fixture without a microphone, a TTS dependency, or a checked-in binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/santh/crucible/internal/audio"
	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/logging"
	"github.com/santh/crucible/internal/vertexai"
)

func main() {
	var (
		mode     = flag.String("mode", "speak", "speak | listen")
		text     = flag.String("text", "Ask me one short question about backpressure in streaming systems.", "text to send in speak mode")
		inFile   = flag.String("in", "testdata/answer.wav", "input WAV (listen mode), PCM16 mono")
		outFile  = flag.String("out", "testdata/out/spike.wav", "where to write received audio")
		voice    = flag.String("voice", "Charon", "prebuilt voice name")
		timeout  = flag.Duration("timeout", 90*time.Second, "overall deadline")
		realtime = flag.Bool("realtime", true, "pace audio upload at wall-clock speed, as a browser would")
	)
	flag.Parse()

	_ = config.LoadDotEnv(".env")
	log := logging.New(os.Getenv("LOG_LEVEL"))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "error", err.Error())
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	vx, err := vertexai.New(ctx, cfg, log, nil)
	if err != nil {
		log.Error("vertex client", "error", err.Error())
		os.Exit(1)
	}

	if err := run(ctx, log, cfg, vx, spikeOpts{
		mode: *mode, text: *text, in: *inFile, out: *outFile,
		voice: *voice, realtime: *realtime,
	}); err != nil {
		log.Error("spike failed", "error", err.Error())
		os.Exit(1)
	}
}

type spikeOpts struct {
	mode, text, in, out, voice string
	realtime                   bool
}

func run(ctx context.Context, log *slog.Logger, cfg *config.Config, vx *vertexai.Client, o spikeOpts) error {
	connectStart := time.Now()

	session, err := vx.RawLive().Live.Connect(ctx, cfg.ModelLive, liveConfig(cfg, o.voice))
	if err != nil {
		return fmt.Errorf("live connect (%s in %s): %w", cfg.ModelLive, cfg.LiveLocation, err)
	}
	defer session.Close()

	log.Info("live session connected",
		"model", cfg.ModelLive,
		"location", cfg.LiveLocation,
		"voice", o.voice,
		"activity_mode", string(cfg.ActivityMode),
		"connect_ms", time.Since(connectStart).Milliseconds())

	// Receive runs concurrently: the API streams audio back while we are still
	// uploading, and a send-then-receive shape would serialise that away and
	// make the measured latency meaningless.
	results := make(chan collected, 1)
	go func() { results <- receiveLoop(ctx, log, session) }()

	sendStart := time.Now()
	switch o.mode {
	case "speak":
		if err := sendText(session, o.text); err != nil {
			return err
		}
		log.Info("sent text prompt", "text", o.text)

	case "listen":
		n, err := sendAudioFile(log, session, o.in, o.realtime)
		if err != nil {
			return err
		}
		log.Info("sent audio", "file", o.in, "frames", n)

	default:
		return fmt.Errorf("unknown mode %q (want speak or listen)", o.mode)
	}
	// The moment the user stops talking. Everything after this is the number
	// PRD §4.4 caps at 1.2 s and the headline metric for the whole product.
	turnBoundary := time.Now()

	var res collected
	select {
	case res = <-results:
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for model turn: %w", ctx.Err())
	}
	if res.err != nil {
		return res.err
	}

	if res.firstAudioAt.IsZero() {
		log.Warn("no audio received from model")
	} else {
		log.Info("TURN BOUNDARY LATENCY",
			"stopped_speaking_to_first_audio_ms", res.firstAudioAt.Sub(turnBoundary).Milliseconds(),
			"note", "PRD §4.4 target is under 1200ms")
	}

	log.Info("turn complete",
		"upload_ms", turnBoundary.Sub(sendStart).Milliseconds(),
		"audio_bytes", len(res.audio),
		"audio_duration_ms", audio.Duration(res.audio, audio.SampleRateOut),
		"user_transcript", res.inputTranscript.String(),
		"ai_transcript", res.outputTranscript.String(),
		"interim_updates", res.interimCount,
		"total_tokens", res.totalTokens,
		"audio_tokens_in", res.audioTokensIn,
		"audio_tokens_out", res.audioTokensOut)

	if len(res.audio) == 0 {
		return nil
	}
	return writeOutputs(log, o.out, res.audio)
}

// liveConfig assembles the connect-time configuration.
func liveConfig(cfg *config.Config, voice string) *genai.LiveConnectConfig {
	lc := &genai.LiveConnectConfig{
		ResponseModalities: []genai.Modality{genai.ModalityAudio},
		SpeechConfig: &genai.SpeechConfig{
			VoiceConfig: &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{VoiceName: voice},
			},
		},
		// Both transcriptions on. Input drives the evaluator; output sources
		// the on-screen question text, because in a noisy demo hall nobody can
		// hear the audio.
		InputAudioTranscription:  &genai.AudioTranscriptionConfig{},
		OutputAudioTranscription: &genai.AudioTranscriptionConfig{},
		// Enabled now even though nothing consumes the handle yet: it costs
		// nothing and Phase 5's reconnect path needs the server to have been
		// emitting SessionResumptionUpdate all along.
		SessionResumption: &genai.SessionResumptionConfig{},
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: "You are a technical interviewer. Keep every utterance under 40 words. Ask exactly one question at a time."}},
		},
	}

	if cfg.ActivityMode == config.ActivityManual {
		// AD-2: the client owns the turn boundary. Deterministic, echo-proof,
		// and it lets the upstream skip silent frames.
		lc.RealtimeInputConfig = &genai.RealtimeInputConfig{
			AutomaticActivityDetection: &genai.AutomaticActivityDetection{Disabled: true},
		}
	}
	return lc
}

func sendText(session *genai.Session, text string) error {
	turnComplete := true
	return session.SendClientContent(genai.LiveClientContentInput{
		Turns:        []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: text}}}},
		TurnComplete: &turnComplete,
	})
}

// sendAudioFile streams a WAV as user speech, bracketed by explicit activity
// signals.
func sendAudioFile(log *slog.Logger, session *genai.Session, path string, realtime bool) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", path, err)
	}
	pcm, rate, err := audio.ReadWAV(raw)
	if err != nil {
		return 0, err
	}
	if rate != audio.SampleRateIn {
		log.Info("resampling input to Live input rate", "from", rate, "to", audio.SampleRateIn)
		pcm = audio.Resample(pcm, rate, audio.SampleRateIn)
	}

	// In manual mode the model will not begin its turn until ActivityEnd, so
	// these two signals are the entire turn boundary protocol.
	if err := session.SendRealtimeInput(genai.LiveRealtimeInput{
		ActivityStart: &genai.ActivityStart{},
	}); err != nil {
		return 0, fmt.Errorf("activity start: %w", err)
	}

	frames := audio.SplitFrames(pcm, audio.FrameSize(audio.SampleRateIn))
	tick := time.NewTicker(audio.FrameDurationMs * time.Millisecond)
	defer tick.Stop()

	for _, f := range frames {
		if err := session.SendRealtimeInput(genai.LiveRealtimeInput{
			Audio: &genai.Blob{
				Data:     f,
				MIMEType: fmt.Sprintf("audio/pcm;rate=%d", audio.SampleRateIn),
			},
		}); err != nil {
			return 0, fmt.Errorf("sending audio frame: %w", err)
		}
		// Pacing at wall-clock speed matters: blasting ten seconds of audio in
		// fifty milliseconds measures throughput, not the conversational
		// latency we actually care about.
		if realtime {
			<-tick.C
		}
	}

	if err := session.SendRealtimeInput(genai.LiveRealtimeInput{
		ActivityEnd: &genai.ActivityEnd{},
	}); err != nil {
		return 0, fmt.Errorf("activity end: %w", err)
	}
	return len(frames), nil
}

type collected struct {
	audio                             []byte
	inputTranscript, outputTranscript strings.Builder
	interimCount                      int
	firstAudioAt                      time.Time
	totalTokens                       int64
	audioTokensIn, audioTokensOut     int64
	err                               error
}

// receiveLoop drains the session until the model's turn completes.
//
// The critical detail, and the one the PRD calls out: a single server message
// is a BAG OF PARTS. Audio chunks, an input transcript, and an output
// transcript can all arrive on the same message. Writing this as a switch that
// handles one field and moves on silently drops data.
func receiveLoop(ctx context.Context, log *slog.Logger, session *genai.Session) collected {
	var c collected

	for {
		if err := ctx.Err(); err != nil {
			c.err = err
			return c
		}

		msg, err := session.Receive()
		if err != nil {
			c.err = fmt.Errorf("receive: %w", err)
			return c
		}
		if msg == nil {
			continue
		}

		if msg.SetupComplete != nil {
			log.Debug("setup complete")
		}
		if msg.UsageMetadata != nil {
			u := vertexai.UsageFromLive("live", msg.UsageMetadata)
			if u != nil {
				c.totalTokens = u.TotalTokens
				c.audioTokensIn = u.PromptAudioTokens
				c.audioTokensOut = u.ResponseAudioTokens
			}
		}
		if msg.GoAway != nil {
			log.Warn("server sent GoAway; connection closing soon")
		}
		if msg.SessionResumptionUpdate != nil {
			log.Debug("session resumption handle updated")
		}

		sc := msg.ServerContent
		if sc == nil {
			continue
		}

		// Every branch below is an `if`, never a `case`: they co-occur.
		if sc.ModelTurn != nil {
			for _, part := range sc.ModelTurn.Parts {
				if part.InlineData == nil || len(part.InlineData.Data) == 0 {
					continue
				}
				if c.firstAudioAt.IsZero() {
					c.firstAudioAt = time.Now()
				}
				c.audio = append(c.audio, part.InlineData.Data...)
			}
		}
		if sc.InterimInputTranscription != nil {
			c.interimCount++
		}
		if sc.InputTranscription != nil {
			c.inputTranscript.WriteString(sc.InputTranscription.Text)
		}
		if sc.OutputTranscription != nil {
			c.outputTranscript.WriteString(sc.OutputTranscription.Text)
		}
		if sc.Interrupted {
			log.Info("model interrupted by user activity")
		}
		if sc.TurnComplete {
			return c
		}
	}
}

func writeOutputs(log *slog.Logger, outPath string, pcm []byte) error {
	if err := os.MkdirAll(dir(outPath), 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", outPath, err)
	}
	defer f.Close()

	// Model output is 24 kHz — writing this header as 16 kHz would produce a
	// file that plays slow rather than one that fails.
	if err := audio.WriteWAV(f, pcm, audio.SampleRateOut); err != nil {
		return err
	}
	log.Info("wrote model audio", "path", outPath, "sample_rate", audio.SampleRateOut)

	// Also emit a 16 kHz copy, ready to be replayed as user speech by
	// -mode=listen. This is how we bootstrap a speech fixture with no
	// microphone and nothing binary checked into the repo.
	replayPath := strings.TrimSuffix(outPath, ".wav") + ".16k.wav"
	rf, err := os.Create(replayPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", replayPath, err)
	}
	defer rf.Close()

	if err := audio.WriteWAV(rf, audio.Resample(pcm, audio.SampleRateOut, audio.SampleRateIn), audio.SampleRateIn); err != nil {
		return err
	}
	log.Info("wrote replayable fixture", "path", replayPath, "sample_rate", audio.SampleRateIn)
	return nil
}

func dir(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[:i]
	}
	return "."
}
