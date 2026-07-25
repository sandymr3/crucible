package live

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/genai"

	"github.com/santh/crucible/internal/audio"
	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/logging"
	"github.com/santh/crucible/internal/turn"
	"github.com/santh/crucible/internal/vertexai"
)

// outboundBuffer is how many frames may queue toward the client before we
// consider the connection wedged. Audio arrives in ~20 ms chunks, so this is
// roughly five seconds of slack — generous for a stalled render, short enough
// that a genuinely dead client is detected rather than buffered forever.
const outboundBuffer = 256

// writeWait bounds a single WebSocket write. A client that cannot absorb a
// frame in this long is not going to recover.
const writeWait = 10 * time.Second

// Relay owns the browser ⇄ Vertex Live bridge for one session.
type Relay struct {
	cfg *config.Config
	log *slog.Logger
	vx  *vertexai.Client

	upgrader websocket.Upgrader
	sessionRegistry
}

// NewRelay builds the relay.
func NewRelay(cfg *config.Config, log *slog.Logger, vx *vertexai.Client) *Relay {
	return &Relay{
		cfg: cfg,
		log: log,
		vx:  vx,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// Same-origin is enforced by serving the frontend from Firebase
			// Hosting with a rewrite to this service, so the browser never
			// makes a cross-origin WebSocket request. Phase 2 tightens this to
			// an explicit allowlist alongside real auth.
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// SessionOpts carries what a session needs to start. Phase 3 fills these from
// the persona config and the resume digest; Phase 1 uses defaults.
type SessionOpts struct {
	SessionID         string
	UID               string
	Voice             string
	SystemInstruction string
	// Temperature shapes the persona's register: lower is clipped and precise,
	// higher is conversational.
	Temperature float32

	// FixtureID, when set, serves a recorded session instead of connecting to
	// Vertex (AD-7). The client cannot tell the difference.
	FixtureID string
	// ReplaySpeed scales replay playback. 0 or 1 is real time.
	ReplaySpeed float64
}

// Handle upgrades the request and runs the session to completion. It returns
// only once the session is fully torn down.
func (r *Relay) Handle(w http.ResponseWriter, req *http.Request, opts SessionOpts) {
	ctx := logging.WithSession(req.Context(), opts.SessionID)
	log := logging.From(ctx, r.log)

	conn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		// Upgrade already wrote an error response.
		log.Error("websocket upgrade failed", "error", err.Error())
		return
	}
	defer conn.Close()

	// The hard session cap is enforced here, on the server, because a
	// client-enforced cap is not a cap. A forgotten open tab is a continuous
	// credit leak against the most expensive component in the system.
	ctx, cancel := context.WithTimeout(ctx, r.cfg.SessionMaxDuration)
	defer cancel()

	s := &session{
		relay:    r,
		log:      log,
		conn:     conn,
		opts:     opts,
		outbound: make(chan outboundFrame, outboundBuffer),
		upstream: make(chan upstreamMsg, upstreamBuffer),
		done:     make(chan struct{}),
	}
	s.run(ctx)
}

type outboundFrame struct {
	binary  bool
	payload []byte
}

// upstreamKind distinguishes what a queued upstream message carries.
type upstreamKind int

const (
	upAudio upstreamKind = iota
	upActivityStart
	upActivityEnd
	upText
)

// upstreamMsg is one item bound for Vertex.
//
// Audio and control signals share a single queue because their ORDER is
// load-bearing: an activity_end that overtakes the tail of the audio would
// close the turn on a truncated answer. Two channels would race.
type upstreamMsg struct {
	kind upstreamKind
	pcm  []byte
	text string
}

// upstreamBuffer holds ~2.5 s of 20 ms frames. Deep enough to absorb a network
// hiccup on either hop, shallow enough that a genuinely stalled uplink is
// detected rather than silently accumulating latency.
const upstreamBuffer = 128

type session struct {
	relay *Relay
	log   *slog.Logger
	conn  *websocket.Conn
	opts  SessionOpts

	live     *genai.Session
	outbound chan outboundFrame
	upstream chan upstreamMsg

	// ctx is the session's run context. Held on the struct so dispatch can
	// attribute Vertex usage to this session without threading a context
	// through every fan-out call.
	ctx context.Context

	// done closes exactly once, on the first teardown path to win.
	done     chan struct{}
	doneOnce atomic.Bool

	audioSeq atomic.Uint32

	// lastClientActivity backs the idle timeout — the single most important
	// credit guardrail in the system.
	lastClientActivity atomic.Int64

	audioFrames    atomic.Int64
	audioBytes     atomic.Int64
	firstAudioInAt atomic.Int64

	// turnEndedAt is when the user stopped speaking. Together with the first
	// audio byte back it produces the headline latency metric.
	turnEndedAt atomic.Int64
	awaitingAI  atomic.Bool

	// boundaryPending is set when the client signals activity_end and cleared
	// when the turn actually closes. It exists because the boundary signal and
	// the transcript that completes the turn arrive on different goroutines,
	// in that order.
	boundaryPending atomic.Bool

	// turnMu guards the active turn buffer, which is written from the client
	// reader (audio), the Vertex reader (transcripts), and the boundary.
	turnMu    sync.Mutex
	turn      *turn.Buffer
	turnIndex int
}

func (s *session) run(ctx context.Context) {
	defer s.stop()

	s.ctx = ctx
	s.touch()
	s.turn = turn.NewBuffer(0)
	s.relay.register(s.opts.SessionID, s)
	defer s.relay.unregister(s.opts.SessionID)

	s.send(ServerFrame{Type: TypeState, State: StateConnecting})

	// A replayed session never opens a Vertex connection, so it cannot be
	// broken by a network problem, a rate limit, or an outage.
	if s.opts.FixtureID != "" {
		s.runReplay(ctx, s.opts.FixtureID)
		return
	}

	connectStart := time.Now()
	liveSession, err := s.relay.vx.RawLive().Live.Connect(
		ctx, s.relay.cfg.ModelLive, s.connectConfig())
	if err != nil {
		// Live connection establishment is the highest-risk dependency in the
		// product, so its failures are logged with enough detail to diagnose
		// without a reproduction.
		s.log.Error("live connect failed",
			"model", s.relay.cfg.ModelLive,
			"location", s.relay.cfg.LiveLocation,
			"connect_ms", time.Since(connectStart).Milliseconds(),
			"error", err.Error())
		s.send(ServerFrame{
			Type: TypeError, Code: "live_connect_failed", Recoverable: false,
			Message: "Could not reach the interviewer. Please retry.",
		})
		s.drainOutboundBlocking()
		return
	}
	s.live = liveSession
	// Never rely on GC to close a billing connection. Timed, because a Close
	// that hangs would keep this handler — and therefore the session's
	// post-teardown bookkeeping — blocked indefinitely.
	defer func() {
		closeStart := time.Now()
		_ = liveSession.Close()
		s.log.Info("live session closed",
			"close_ms", time.Since(closeStart).Milliseconds())
	}()

	s.log.Info("live session connected",
		"model", s.relay.cfg.ModelLive,
		"location", s.relay.cfg.LiveLocation,
		"voice", s.opts.Voice,
		"activity_mode", string(s.relay.cfg.ActivityMode),
		"connect_ms", time.Since(connectStart).Milliseconds())

	go s.writePump(ctx)
	go s.sendPump(ctx)
	go s.readFromVertex(ctx)
	go s.watchIdle(ctx)

	s.send(ServerFrame{Type: TypeState, State: StateListening})

	// The client reader runs on this goroutine, so run returns — and the
	// deferred cleanup fires — the moment the browser disconnects.
	s.readFromClient(ctx)
}

func (s *session) connectConfig() *genai.LiveConnectConfig {
	cfg := s.relay.cfg

	instruction := s.opts.SystemInstruction
	if instruction == "" {
		instruction = "You are a technical interviewer. Ask exactly one question at a time. Keep every utterance under 60 words."
	}
	voice := s.opts.Voice
	if voice == "" {
		voice = "Charon"
	}

	lc := &genai.LiveConnectConfig{
		ResponseModalities: []genai.Modality{genai.ModalityAudio},
		SpeechConfig: &genai.SpeechConfig{
			VoiceConfig: &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{VoiceName: voice},
			},
		},
		InputAudioTranscription:  &genai.AudioTranscriptionConfig{},
		OutputAudioTranscription: &genai.AudioTranscriptionConfig{},
		SessionResumption:        &genai.SessionResumptionConfig{},
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: instruction}},
		},
	}

	if s.opts.Temperature > 0 {
		lc.Temperature = &s.opts.Temperature
	}

	if cfg.ActivityMode == config.ActivityManual {
		lc.RealtimeInputConfig = &genai.RealtimeInputConfig{
			AutomaticActivityDetection: &genai.AutomaticActivityDetection{Disabled: true},
		}
	}
	return lc
}

// --- Client → Vertex ------------------------------------------------------

func (s *session) readFromClient(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		msgType, data, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				s.log.Warn("client connection lost", "error", err.Error())
			} else {
				s.log.Info("client disconnected")
			}
			return
		}
		s.touch()

		switch msgType {
		case websocket.BinaryMessage:
			// Copy: gorilla reuses its read buffer, so handing the slice to
			// another goroutine without copying would let the next frame
			// overwrite audio still queued for send.
			pcm := make([]byte, len(data))
			copy(pcm, data)
			s.audioFrames.Add(1)
			s.audioBytes.Add(int64(len(pcm)))
			s.firstAudioInAt.CompareAndSwap(0, time.Now().UnixMilli())
			s.captureAudio(pcm)
			s.queueUpstream(upstreamMsg{kind: upAudio, pcm: pcm})

		case websocket.TextMessage:
			if stop := s.handleControl(data); stop {
				return
			}
		}
	}
}

// upstreamChunkMs is how much audio the relay coalesces before forwarding.
//
// The browser sends 20 ms frames because that is the right capture
// granularity, but forwarding each one as its own API call was measurably
// expensive: per-call overhead accumulated across ~260 frames into roughly
// 700 ms of upstream backlog, and because activity_end queues behind that
// backlog, all of it landed on the turn-boundary latency.
//
// Vertex paces its own ingestion at approximately real time — verified by
// blasting a file with no pacing, which made latency worse, not better — so
// there is nothing to gain from more frequent sends. Coalescing to 100 ms cuts
// the call count fivefold and costs at most 100 ms of input buffering.
const upstreamChunkMs = 100

// sendPump is the only goroutine that writes to the Vertex session.
//
// Two jobs: keep the client read loop from ever blocking on a network write,
// and coalesce 20 ms frames into larger chunks. Pending audio is always flushed
// before any control signal, so an activity_end can never close a turn on a
// truncated answer.
func (s *session) sendPump(ctx context.Context) {
	defer s.stop()

	mime := "audio/pcm;rate=" + itoa(audio.SampleRateIn)
	chunkBytes := audio.SampleRateIn * upstreamChunkMs / 1000 * audio.BytesPerSample

	pending := make([]byte, 0, chunkBytes*2)

	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		err := s.live.SendRealtimeInput(genai.LiveRealtimeInput{
			Audio: &genai.Blob{Data: pending, MIMEType: mime},
		})
		pending = pending[:0]
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case m := <-s.upstream:
			var err error
			switch m.kind {
			case upAudio:
				pending = append(pending, m.pcm...)
				if len(pending) >= chunkBytes {
					err = flush()
				}

			case upActivityStart:
				// Nothing buffered can belong to a turn that has not begun,
				// but flush anyway so state is never carried across a boundary.
				if err = flush(); err == nil {
					err = s.live.SendRealtimeInput(genai.LiveRealtimeInput{
						ActivityStart: &genai.ActivityStart{},
					})
				}

			case upActivityEnd:
				// The tail of the answer must reach Vertex before the turn is
				// closed, or the model grades a sentence that stops mid-word.
				if err = flush(); err == nil {
					err = s.live.SendRealtimeInput(genai.LiveRealtimeInput{
						ActivityEnd: &genai.ActivityEnd{},
					})
				}
				// Splits turn-boundary latency into the part we own (queue
				// drain) and the part Vertex owns (model response). Without
				// this split, tuning the relay is guesswork.
				if endedAt := s.turnEndedAt.Load(); endedAt != 0 {
					s.log.Info("upstream drained",
						"metric", "activity_end_queue_lag_ms",
						"value_ms", time.Now().UnixMilli()-endedAt)
				}

			case upText:
				if err = flush(); err == nil {
					turnComplete := true
					err = s.live.SendClientContent(genai.LiveClientContentInput{
						Turns:        []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: m.text}}}},
						TurnComplete: &turnComplete,
					})
				}
			}

			if err != nil {
				s.log.Error("upstream send failed", "kind", m.kind, "error", err.Error())
				return
			}
		}
	}
}

// queueUpstream enqueues without blocking the reader.
//
// A full queue means we are receiving audio faster than Vertex will accept it.
// Dropping the oldest frame is deliberately preferred to blocking: blocking
// would stall the read loop, which delays the activity_end behind it and turns
// a throughput problem into a latency problem on the metric that matters.
func (s *session) queueUpstream(m upstreamMsg) {
	select {
	case s.upstream <- m:
		return
	case <-s.done:
		return
	default:
	}

	// Control signals must never be dropped — losing an activity_end leaves
	// the turn open forever. Block for them, briefly.
	if m.kind != upAudio {
		select {
		case s.upstream <- m:
		case <-s.done:
		case <-time.After(2 * time.Second):
			s.log.Error("upstream queue blocked; dropping control signal", "kind", m.kind)
		}
		return
	}

	select {
	case dropped := <-s.upstream:
		s.log.Warn("upstream queue full, dropped a frame",
			"dropped_kind", dropped.kind, "queued", len(s.upstream))
	default:
	}
	select {
	case s.upstream <- m:
	case <-s.done:
	default:
	}
}

// handleControl processes a JSON control frame. It reports whether the session
// should terminate.
func (s *session) handleControl(data []byte) (stop bool) {
	var f ClientFrame
	if err := decodeJSON(data, &f); err != nil {
		s.log.Warn("undecodable control frame", "error", err.Error())
		return false
	}

	switch f.Type {
	case TypeBegin:
		// Kick the interviewer into asking its opening question.
		//
		// Bracketed and marked do-not-read for the same reason every injected
		// turn is: the failure mode is the interviewer reading our internal
		// instructions aloud, which is funny once and fatal on stage.
		s.turnEndedAt.Store(time.Now().UnixMilli())
		s.awaitingAI.Store(true)
		s.send(ServerFrame{Type: TypeState, State: StateAsking})
		s.queueUpstream(upstreamMsg{kind: upText, text: beginDirective})

	case TypeActivityStart:
		s.awaitingAI.Store(false)
		s.send(ServerFrame{Type: TypeState, State: StateListening})
		s.queueUpstream(upstreamMsg{kind: upActivityStart})

	case TypeActivityEnd:
		// The turn boundary. Start the latency clock here.
		s.turnEndedAt.Store(time.Now().UnixMilli())
		s.awaitingAI.Store(true)
		s.send(ServerFrame{Type: TypeState, State: StateClosing})
		// Do NOT close the turn here.
		//
		// The user's transcript arrives from Vertex asynchronously, AFTER this
		// signal — it is produced by the Vertex reader goroutine once the audio
		// has been processed. Snapshotting now captures an empty transcript,
		// and the turn is then skipped as "too short" with zero words despite
		// the user having spoken for fifteen seconds.
		//
		// Instead mark the boundary as pending; the turn closes as soon as the
		// transcription lands, with TurnComplete as a backstop.
		s.boundaryPending.Store(true)
		// Compares the wall-clock upload window against the true duration of
		// the audio. If uploadMs materially exceeds audioMs, the client is
		// feeding us slower than real time and Vertex is still ingesting when
		// the turn closes — which would show up as turn-boundary latency that
		// is not actually the model's fault.
		if first := s.firstAudioInAt.Load(); first != 0 {
			audioMs := s.audioBytes.Load() * 1000 / int64(audio.SampleRateIn*audio.BytesPerSample)
			uploadMs := time.Now().UnixMilli() - first
			s.log.Info("upload window",
				"metric", "client_upload_ms",
				"upload_ms", uploadMs,
				"audio_ms", audioMs,
				"drift_ms", uploadMs-audioMs,
				"frames", s.audioFrames.Load())
		}
		s.queueUpstream(upstreamMsg{kind: upActivityEnd})

	case TypeTextAnswer:
		// Typed answers travel the same path as speech, which is what lets the
		// accessibility fallback, the demo safety net, and Study Mode share one
		// implementation instead of three.
		s.turnEndedAt.Store(time.Now().UnixMilli())
		s.awaitingAI.Store(true)
		// A typed answer needs no transcription round trip — the text is
		// already in hand, so the turn can close immediately.
		s.captureTextAnswer(f.Text)
		s.send(ServerFrame{Type: TypeTranscript, Side: SideUser, Text: f.Text, Final: true})
		s.closeTurn()
		s.queueUpstream(upstreamMsg{kind: upText, text: f.Text})

	case TypeRequestHint:
		go s.handleHint()

	case TypePing:
		s.send(ServerFrame{Type: TypePong, T: f.T})

	case TypeEndSession:
		s.log.Info("client ended session")
		return true

	default:
		s.log.Warn("unknown control frame type", "type", f.Type)
	}
	return false
}

// --- Vertex → Client ------------------------------------------------------

func (s *session) readFromVertex(ctx context.Context) {
	defer s.stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		msg, err := s.live.Receive()
		if err != nil {
			if !errors.Is(err, context.Canceled) && !isClosed(s.done) {
				s.log.Error("live receive failed", "error", err.Error())
				s.send(ServerFrame{
					Type: TypeError, Code: "live_stream_lost", Recoverable: true,
					Message: "Connection to the interviewer dropped.",
				})
			}
			return
		}
		if msg == nil {
			continue
		}
		s.dispatch(msg)
	}
}

// dispatch fans one server message out to the client.
//
// Every branch is an `if`, deliberately. A single Live server event can carry
// audio chunks AND an input transcript AND an output transcript at once, so
// this must be treated as a bag of parts rather than a tagged union. Writing it
// as a switch silently drops data, and the symptom — occasionally missing
// transcript text — is miserable to track down later.
func (s *session) dispatch(msg *genai.LiveServerMessage) {
	if msg.UsageMetadata != nil {
		if u := vertexai.UsageFromLive(s.relay.cfg.ModelLive, msg.UsageMetadata); u != nil {
			// Persist as well as display. The live connection is the dominant
			// cost in this system, so a ledger that omitted it would be worse
			// than no ledger — it would look authoritative while missing
			// nearly all the spend.
			s.relay.vx.RecordLiveUsage(s.ctx, msg.UsageMetadata)

			s.send(ServerFrame{
				Type:           TypeUsage,
				TotalTokens:    u.TotalTokens,
				AudioTokensIn:  u.PromptAudioTokens,
				AudioTokensOut: u.ResponseAudioTokens,
			})
		}
	}
	if msg.GoAway != nil {
		// The server is about to hang up. Phase 5 uses the resumption handle
		// to reconnect transparently; for now, tell the client honestly.
		s.log.Warn("live server sent GoAway")
		s.send(ServerFrame{
			Type: TypeError, Code: "live_going_away", Recoverable: true,
			Message: "Reconnecting…",
		})
	}

	sc := msg.ServerContent
	if sc == nil {
		return
	}

	if sc.ModelTurn != nil {
		for _, part := range sc.ModelTurn.Parts {
			if part.InlineData == nil || len(part.InlineData.Data) == 0 {
				continue
			}
			s.recordFirstAudio()
			s.sendBinary(encodeAudioFrame(s.audioSeq.Add(1), part.InlineData.Data))
		}
	}

	// Interim transcription updates while the user is still speaking. This is
	// what the UI renders at reduced opacity before finalising — the effect
	// that makes the transcript feel alive.
	if sc.InterimInputTranscription != nil && sc.InterimInputTranscription.Text != "" {
		s.send(ServerFrame{
			Type: TypeTranscript, Side: SideUser,
			Text: sc.InterimInputTranscription.Text, Final: false,
		})
	}
	if sc.InputTranscription != nil && sc.InputTranscription.Text != "" {
		s.captureUserTranscript(sc.InputTranscription.Text)
		s.send(ServerFrame{
			Type: TypeTranscript, Side: SideUser,
			Text: sc.InputTranscription.Text, Final: true,
		})
		// The transcript the boundary was waiting for. Close now rather than at
		// TurnComplete, so grading starts while the interviewer is still
		// acknowledging rather than after it has finished speaking.
		if s.boundaryPending.CompareAndSwap(true, false) {
			s.closeTurn()
		}
	}
	if sc.OutputTranscription != nil && sc.OutputTranscription.Text != "" {
		s.captureQuestion(sc.OutputTranscription.Text)
		s.send(ServerFrame{
			Type: TypeTranscript, Side: SideAI,
			Text: sc.OutputTranscription.Text, Final: true,
		})
	}

	if sc.Interrupted {
		// The client must discard queued playback immediately rather than
		// letting the buffer drain: a model that keeps talking for two seconds
		// after being interrupted feels broken.
		s.send(ServerFrame{Type: TypeInterrupted})
		s.send(ServerFrame{Type: TypeState, State: StateListening})
	}
	if sc.TurnComplete {
		// Backstop: if no transcription ever arrived — silence, or a
		// recognition failure — the turn still has to close, or its audio is
		// never persisted and the next turn inherits it.
		if s.boundaryPending.CompareAndSwap(true, false) {
			s.closeTurn()
		}
		s.awaitingAI.Store(false)
		s.send(ServerFrame{Type: TypeTurnComplete})
		s.send(ServerFrame{Type: TypeState, State: StateListening})
	}
}

// recordFirstAudio logs the turn-boundary latency once per turn: the interval
// between the user finishing and the interviewer starting to speak. PRD §4.4
// targets under 1.2 s, and this is the headline metric for the product.
func (s *session) recordFirstAudio() {
	if !s.awaitingAI.CompareAndSwap(true, false) {
		return
	}
	endedAt := s.turnEndedAt.Load()
	if endedAt == 0 {
		return
	}
	s.log.Info("turn boundary latency",
		"metric", "turn_boundary_latency_ms",
		"value_ms", time.Now().UnixMilli()-endedAt)
	s.send(ServerFrame{Type: TypeState, State: StateAsking})
}

// --- Plumbing -------------------------------------------------------------

// writePump is the ONLY goroutine that writes to the WebSocket. gorilla permits
// exactly one concurrent writer, and several goroutines produce frames, so
// everything funnels through this channel.
func (s *session) writePump(ctx context.Context) {
	defer s.stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case f := <-s.outbound:
			kind := websocket.TextMessage
			if f.binary {
				kind = websocket.BinaryMessage
			}
			_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.conn.WriteMessage(kind, f.payload); err != nil {
				s.log.Warn("client write failed", "error", err.Error())
				return
			}
		}
	}
}

// watchIdle closes the session after a stretch with no client activity.
//
// This is the guardrail that matters most for credits: live audio is the
// dominant cost in the system, and a forgotten open tab bleeds it continuously.
func (s *session) watchIdle(ctx context.Context) {
	idle := s.relay.cfg.SessionIdleTimeout
	ticker := time.NewTicker(idle / 3)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			last := time.UnixMilli(s.lastClientActivity.Load())
			if time.Since(last) < idle {
				continue
			}
			s.log.Info("closing idle session",
				"idle_for_sec", int(time.Since(last).Seconds()),
				"limit_sec", int(idle.Seconds()))
			s.send(ServerFrame{
				Type: TypeError, Code: "idle_timeout", Recoverable: true,
				Message: "Session paused after inactivity.",
			})
			s.drainOutboundBlocking()
			s.stop()
			return
		}
	}
}

func (s *session) touch() {
	s.lastClientActivity.Store(time.Now().UnixMilli())
}

func (s *session) send(f ServerFrame) {
	b, err := f.encode()
	if err != nil {
		s.log.Error("encoding server frame", "type", f.Type, "error", err.Error())
		return
	}
	s.enqueue(outboundFrame{payload: b})
}

func (s *session) sendBinary(b []byte) {
	s.enqueue(outboundFrame{binary: true, payload: b})
}

// enqueue never blocks. A client too slow to drain audio would otherwise stall
// the Vertex reader, which in turn stalls the billing connection — so a wedged
// client is dropped rather than allowed to apply backpressure all the way to
// the model.
func (s *session) enqueue(f outboundFrame) {
	select {
	case s.outbound <- f:
	case <-s.done:
	default:
		s.log.Warn("outbound buffer full, dropping client", "buffered", len(s.outbound))
		s.stop()
	}
}

// drainOutboundBlocking gives the write pump a brief window to flush queued
// frames before teardown, so a final error message actually reaches the client.
func (s *session) drainOutboundBlocking() {
	deadline := time.After(time.Second)
	for {
		select {
		case <-deadline:
			return
		default:
			if len(s.outbound) == 0 {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// stop is idempotent; whichever teardown path fires first wins.
//
// Closing the client connection is not optional cleanup — it is what makes
// teardown actually happen. readFromClient blocks in conn.ReadMessage(), which
// does not observe the done channel, so without this close the session
// goroutine survives until the browser happens to disconnect. run() would never
// return, its deferred liveSession.Close() would never fire, and the Vertex
// connection would keep billing for a session we already decided to end.
//
// That defeats the idle timeout entirely, which is the single guardrail
// standing between a forgotten open tab and the credit budget.
func (s *session) stop() {
	if s.doneOnce.CompareAndSwap(false, true) {
		close(s.done)
		if s.conn != nil {
			_ = s.conn.Close()
		}
	}
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
