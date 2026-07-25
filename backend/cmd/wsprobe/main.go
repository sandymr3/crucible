// Command wsprobe is a command-line stand-in for the browser.
//
// It speaks the exact WebSocket protocol the frontend will speak: streams a WAV
// upstream at wall-clock pace as PCM16 frames, brackets it with the manual
// activity signals, and pretty-prints every frame that comes back. Received
// audio is reassembled into a WAV you can listen to.
//
// This exists because the backend has to be testable for the rest of the build
// without a frontend existing, and because when something breaks it answers
// "is it the relay or the browser?" in one command.
//
// Usage:
//
//	go run ./cmd/wsprobe -url ws://localhost:8080 -wav testdata/out/spike.16k.wav
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/santh/crucible/internal/audio"
	"github.com/santh/crucible/internal/live"
	"github.com/santh/crucible/internal/replay"
)

func main() {
	var (
		base      = flag.String("url", "ws://localhost:8080", "server base URL (ws:// or wss://)")
		sessionID = flag.String("session", "probe-1", "session id")
		wavPath   = flag.String("wav", "", "WAV to stream as the user's answer (PCM16 mono); empty = listen only")
		textAns   = flag.String("text", "", "send this as a typed answer instead of audio")
		textSeq   = flag.String("texts", "", "several answers separated by || , driving a multi-turn session")
		turnGap   = flag.Duration("gap", 14*time.Second, "how long to wait between answers in -texts mode")
		askHint   = flag.Bool("hint", false, "request a Socratic hint after the first question")
		recordTo  = flag.String("record", "", "capture this session as a replay fixture (AD-7)")
		voice     = flag.String("voice", "", "voice name override")
		outPath   = flag.String("out", "testdata/out/wsprobe-received.wav", "where to write received audio")
		token     = flag.String("token", "", "Firebase ID token (Phase 2+)")
		wait      = flag.Duration("wait", 25*time.Second, "how long to keep listening after the answer")
		realtime  = flag.Bool("realtime", true, "pace upload at wall-clock speed, as a browser would")
	)
	flag.Parse()

	p := &probe{out: *outPath, ready: make(chan struct{})}
	p.textSeq, p.turnGap, p.askHint = *textSeq, *turnGap, *askHint
	p.recordTo = *recordTo
	if err := p.run(*base, *sessionID, *wavPath, *textAns, *voice, *token, *wait, *realtime); err != nil {
		fmt.Fprintf(os.Stderr, "\nwsprobe: %v\n", err)
		os.Exit(1)
	}
}

type probe struct {
	out string

	// ready closes when the server first reports LISTENING.
	ready     chan struct{}
	readyOnce sync.Once

	mu             sync.Mutex
	received       []byte
	lastSeq        uint32
	gaps           int
	transcriptUser strings.Builder
	transcriptAI   strings.Builder
	usage          live.ServerFrame

	answerDoneAt time.Time
	firstAudioAt time.Time

	textSeq  string
	turnGap  time.Duration
	askHint  bool
	recordTo string

	// recording captures every received frame with its offset, so a real
	// session can be replayed later frame-for-frame.
	recStart time.Time
	events   []replay.Event
	bands   []string
}

func (p *probe) run(base, sessionID, wavPath, textAns, voice, token string, wait time.Duration, realtime bool) error {
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("parsing -url: %w", err)
	}
	u.Path = fmt.Sprintf("/v1/sessions/%s/live", sessionID)
	q := u.Query()
	if token != "" {
		q.Set("token", token)
	}
	if voice != "" {
		q.Set("voice", voice)
	}
	u.RawQuery = q.Encode()

	fmt.Printf("connecting to %s://%s%s\n", u.Scheme, u.Host, u.Path)

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial failed: %w (HTTP %s)", err, resp.Status)
		}
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()
	fmt.Print("connected\n\n")

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		p.readLoop(conn)
	}()

	// Wait for the server to report LISTENING before sending anything.
	//
	// The upgrade completes long before the Vertex session behind it does.
	// Streaming immediately would pile audio into the socket buffer during that
	// window, and the burst the relay then forwards leaves Vertex still
	// ingesting when the turn closes — which shows up as turn-boundary latency
	// that has nothing to do with the model. A real client must honour this too.
	select {
	case <-p.ready:
		fmt.Print("server is LISTENING\n\n")
	case <-time.After(20 * time.Second):
		return fmt.Errorf("server never reported LISTENING")
	case <-readerDone:
		return fmt.Errorf("connection closed before the session was ready")
	}

	// Ask the interviewer to open. A real client sends this once its audio
	// playback pipeline is ready.
	if err := writeJSON(conn, live.ClientFrame{Type: live.TypeBegin}); err != nil {
		return err
	}
	fmt.Print("→ begin\n\n")

	// Keepalive. Cloud Run drops idle connections, and a session that dies
	// during a thoughtful pause is exactly the failure this prevents.
	stopPing := make(chan struct{})
	defer close(stopPing)
	go p.pingLoop(conn, stopPing)

	if p.askHint {
		// Give the interviewer a moment to finish asking before we admit defeat.
		time.Sleep(9 * time.Second)
		fmt.Println("→ request_hint")
		if err := writeJSON(conn, live.ClientFrame{Type: live.TypeRequestHint}); err != nil {
			return err
		}
	}

	switch {
	case p.textSeq != "":
		for i, answer := range strings.Split(p.textSeq, "||") {
			answer = strings.TrimSpace(answer)
			if answer == "" {
				continue
			}
			fmt.Printf("\n--- turn %d ---\n", i+1)
			if err := writeJSON(conn, live.ClientFrame{Type: live.TypeTextAnswer, Text: answer}); err != nil {
				return err
			}
			p.markAnswerDone()
			time.Sleep(p.turnGap)
		}

	case textAns != "":
		fmt.Printf("→ text answer: %q\n", textAns)
		if err := writeJSON(conn, live.ClientFrame{Type: live.TypeTextAnswer, Text: textAns}); err != nil {
			return err
		}
		p.markAnswerDone()

	case wavPath != "":
		if err := p.streamWAV(conn, wavPath, realtime); err != nil {
			return err
		}

	default:
		fmt.Println("no -wav or -text given; listening only")
	}

	// Stop early on Ctrl-C so a long -wait is not a trap.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	select {
	case <-time.After(wait):
	case <-interrupt:
		fmt.Println("\ninterrupted")
	case <-readerDone:
	}

	_ = writeJSON(conn, live.ClientFrame{Type: live.TypeEndSession})
	_ = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

	if err := p.writeFixture(""); err != nil {
		fmt.Fprintf(os.Stderr, "could not write fixture: %v\n", err)
	}
	return p.report()
}

// streamWAV sends the file bracketed by explicit activity signals, pacing
// frames at wall-clock speed. Blasting the whole file instantly would measure
// throughput rather than the conversational latency that actually matters.
func (p *probe) streamWAV(conn *websocket.Conn, path string, realtime bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	pcm, rate, err := audio.ReadWAV(raw)
	if err != nil {
		return err
	}
	if rate != audio.SampleRateIn {
		fmt.Printf("resampling %d Hz → %d Hz\n", rate, audio.SampleRateIn)
		pcm = audio.Resample(pcm, rate, audio.SampleRateIn)
	}

	frames := audio.SplitFrames(pcm, audio.FrameSize(audio.SampleRateIn))
	fmt.Printf("→ streaming %s: %d frames, %d ms of audio\n",
		path, len(frames), audio.Duration(pcm, audio.SampleRateIn))

	if err := writeJSON(conn, live.ClientFrame{Type: live.TypeActivityStart}); err != nil {
		return err
	}

	ticker := time.NewTicker(audio.FrameDurationMs * time.Millisecond)
	defer ticker.Stop()

	for _, f := range frames {
		if err := conn.WriteMessage(websocket.BinaryMessage, f); err != nil {
			return fmt.Errorf("sending audio frame: %w", err)
		}
		if realtime {
			<-ticker.C
		}
	}

	if err := writeJSON(conn, live.ClientFrame{Type: live.TypeActivityEnd}); err != nil {
		return err
	}
	p.markAnswerDone()
	fmt.Print("→ activity_end (turn boundary)\n\n")
	return nil
}

func (p *probe) markAnswerDone() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answerDoneAt = time.Now()
}

func (p *probe) readLoop(conn *websocket.Conn) {
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				fmt.Printf("← read error: %v\n", err)
			}
			return
		}

		if msgType == websocket.BinaryMessage {
			p.record(replay.KindAudio, data)
			p.onAudio(data)
			continue
		}
		p.record(replay.KindJSON, data)

		var f live.ServerFrame
		if err := json.Unmarshal(data, &f); err != nil {
			fmt.Printf("← undecodable frame: %s\n", truncate(string(data), 120))
			continue
		}
		p.onFrame(f)
	}
}

func (p *probe) onAudio(frame []byte) {
	seq, pcm, err := live.DecodeAudioFrame(frame)
	if err != nil {
		fmt.Printf("← bad audio frame: %v\n", err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.firstAudioAt.IsZero() {
		p.firstAudioAt = time.Now()
		if !p.answerDoneAt.IsZero() {
			// The headline metric: user stops speaking → interviewer starts.
			fmt.Printf("← FIRST AUDIO after %d ms  (PRD target < 1200 ms)\n",
				p.firstAudioAt.Sub(p.answerDoneAt).Milliseconds())
		}
	}
	// Sequence numbers exist so gaps are measurable rather than merely
	// audible; a gap here is the earliest warning of a network problem.
	if p.lastSeq != 0 && seq != p.lastSeq+1 {
		p.gaps++
	}
	p.lastSeq = seq
	p.received = append(p.received, pcm...)
}

func (p *probe) onFrame(f live.ServerFrame) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch f.Type {
	case live.TypeTranscript:
		marker := "interim"
		if f.Final {
			marker = "final"
		}
		who := "USER"
		target := &p.transcriptUser
		if f.Side == live.SideAI {
			who, target = "AI  ", &p.transcriptAI
		}
		if f.Final {
			target.WriteString(f.Text)
		}
		fmt.Printf("← [%s %-7s] %s\n", who, marker, f.Text)

	case live.TypeState:
		fmt.Printf("← [state] %s\n", f.State)
		// LISTENING is the server's signal that the Vertex session behind the
		// socket is live and it is safe to start streaming.
		if f.State == live.StateListening {
			p.readyOnce.Do(func() { close(p.ready) })
		}

	case live.TypeTurnComplete:
		fmt.Println("← [turn complete]")

	case live.TypeInterrupted:
		fmt.Println("← [interrupted] discard queued playback")

	case live.TypeUsage:
		p.usage = f
		fmt.Printf("← [usage] total=%d audio_in=%d audio_out=%d\n",
			f.TotalTokens, f.AudioTokensIn, f.AudioTokensOut)

	case live.TypeBand:
		line := fmt.Sprintf("BAND %d -> %d  (%s)", f.From, f.To, f.Message)
		p.bands = append(p.bands, line)
		fmt.Printf("← [BAND CHANGE] %s\n", line)

	case live.TypeHint:
		fmt.Printf("← [hint -%.1f] %s\n", f.Penalty, f.Text)

	case live.TypeError:
		fmt.Printf("← [ERROR] %s recoverable=%v: %s\n", f.Code, f.Recoverable, f.Message)

	case live.TypePong:
		// Keepalive round-trip; too noisy to print.

	default:
		fmt.Printf("← [%s] %+v\n", f.Type, f)
	}
}

func (p *probe) pingLoop(conn *websocket.Conn, stop <-chan struct{}) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = writeJSON(conn, live.ClientFrame{Type: live.TypePing, T: time.Now().UnixMilli()})
		}
	}
}

func (p *probe) report() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	fmt.Println("\n────────── summary ──────────")
	fmt.Printf("audio received     %d bytes (%d ms @ %d Hz)\n",
		len(p.received), audio.Duration(p.received, audio.SampleRateOut), audio.SampleRateOut)
	fmt.Printf("sequence gaps      %d\n", p.gaps)
	if !p.firstAudioAt.IsZero() && !p.answerDoneAt.IsZero() {
		fmt.Printf("turn boundary      %d ms\n", p.firstAudioAt.Sub(p.answerDoneAt).Milliseconds())
	}
	fmt.Printf("user transcript    %s\n", orDash(p.transcriptUser.String()))
	fmt.Printf("ai transcript      %s\n", orDash(p.transcriptAI.String()))
	if p.usage.TotalTokens > 0 {
		fmt.Printf("tokens             total=%d audio_in=%d audio_out=%d\n",
			p.usage.TotalTokens, p.usage.AudioTokensIn, p.usage.AudioTokensOut)
	}

	if len(p.received) == 0 {
		fmt.Println("\nNO AUDIO RECEIVED")
		return nil
	}

	if err := os.MkdirAll(dirOf(p.out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p.out)
	if err != nil {
		return err
	}
	defer f.Close()
	// Model output is 24 kHz. Writing a 16 kHz header here would produce a file
	// that plays slow rather than one that fails.
	if err := audio.WriteWAV(f, p.received, audio.SampleRateOut); err != nil {
		return err
	}
	fmt.Printf("\nwrote %s — play it to confirm the interviewer is audible\n", p.out)
	return nil
}

func writeJSON(conn *websocket.Conn, f live.ClientFrame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func dirOf(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[:i]
	}
	return "."
}

// record captures one received frame for the replay fixture.
//
// Recording client-side is deliberate: what the client received is exactly what
// a replay needs to emit, so a fixture built this way is faithful by
// construction rather than by a separate serialisation that could drift.
func (p *probe) record(kind replay.EventKind, data []byte) {
	if p.recordTo == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.recStart.IsZero() {
		p.recStart = time.Now()
	}
	e := replay.Event{
		OffsetMs: time.Since(p.recStart).Milliseconds(),
		Kind:     kind,
	}
	if kind == replay.KindAudio {
		// Strip the sequence prefix: the replayer re-numbers on the way out,
		// so storing the original numbers would double-prefix them.
		if _, pcm, err := live.DecodeAudioFrame(data); err == nil {
			e.Audio = base64.StdEncoding.EncodeToString(pcm)
		}
	} else {
		e.Frame = append(json.RawMessage(nil), data...)
	}
	p.events = append(p.events, e)
}

// writeFixture saves the recorded session.
func (p *probe) writeFixture(persona string) error {
	p.mu.Lock()
	events := p.events
	p.mu.Unlock()

	if p.recordTo == "" || len(events) == 0 {
		return nil
	}

	f := replay.Fixture{
		ID:          strings.TrimSuffix(filepath.Base(p.recordTo), ".json"),
		Description: "Recorded session captured by wsprobe.",
		Persona:     persona,
		RecordedAt:  time.Now().UTC().Format(time.RFC3339),
		Events:      events,
	}
	b, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.recordTo), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p.recordTo, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("\nrecorded %d events (%d ms) to %s\n",
		len(events), f.DurationMs(), p.recordTo)
	return nil
}
