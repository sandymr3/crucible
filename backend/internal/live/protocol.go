// Package live is the WebSocket relay between the browser and the Vertex Live
// API.
//
// The relay is structurally mandatory, not a performance optimisation: Vertex
// authenticates with an OAuth2 bearer token minted from a service account, and
// there is no safe way to put a service account key in frontend code. So every
// audio frame passes through here.
package live

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// --- Client → Server ------------------------------------------------------

// Binary frames from the client are raw PCM16 @16 kHz mono, 20 ms per frame.
// Text frames are JSON control messages of the shapes below.

const (
	// TypeBegin asks the interviewer to open the session.
	//
	// Required because manual activity detection means the model never speaks
	// unprompted — it waits for a turn boundary that, at session start, has not
	// happened yet. The client sends this once its audio playback pipeline is
	// ready, so the first question is never spoken into a page that cannot play
	// it. That first question is the strongest moment in the product; it must
	// not be missed because the client was still initialising.
	TypeBegin = "begin"

	// TypeActivityStart marks the microphone going hot. With manual activity
	// detection (AD-2) this and ActivityEnd are the entire turn-boundary
	// protocol — the model will not begin speaking until it sees the end.
	TypeActivityStart = "activity_start"
	// TypeActivityEnd marks the user clicking Done. This is the moment the
	// turn-boundary latency clock starts.
	TypeActivityEnd = "activity_end"
	// TypeTextAnswer submits a typed answer instead of speech. Routed through
	// the identical downstream path, which is what makes the accessibility
	// fallback and Study Mode reuse rather than reimplement.
	TypeTextAnswer = "text_answer"
	// TypeRequestHint asks for a Socratic hint. Wired in Phase 5.
	TypeRequestHint = "request_hint"
	// TypeEndSession is a client-initiated close.
	TypeEndSession = "end_session"
	// TypePing keeps the connection alive. Cloud Run closes idle connections,
	// and a demo that dies during a thoughtful pause is a demo that dies.
	TypePing = "ping"
)

// ClientFrame is any JSON control message from the browser.
type ClientFrame struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	T    int64  `json:"t,omitempty"`
}

// --- Server → Client ------------------------------------------------------

const (
	TypeTranscript   = "transcript"
	TypeState        = "state"
	TypeQuestion     = "question"
	TypeTurnComplete = "turn_complete"
	TypeInterrupted  = "interrupted"
	TypeUsage        = "usage"
	TypeError        = "error"
	TypePong         = "pong"
	// TypeEvaluation carries a graded turn, arriving after the answer rather
	// than with it. This is what reveals the heatmap.
	TypeEvaluation = "evaluation"
	// TypeUngraded tells the client a turn could not be graded, so it can say
	// so inline instead of leaving a shimmer running forever.
	TypeUngraded = "ungraded"
	// TypeBand announces a difficulty change. Adaptation the user cannot
	// perceive is worthless, so this drives a visible indicator and a toast.
	TypeBand = "band"
	// TypeHint carries a Socratic nudge and its score penalty.
	TypeHint = "hint"
)

// Turn lifecycle states (PRD §6.3). The most common failure in a voice UI is
// that the user cannot tell whether the system is listening, thinking, or
// dead, so every one of these is surfaced explicitly rather than inferred.
//
// PROTOCOL REQUIREMENT: the client MUST NOT send audio or activity signals
// until it has received StateListening.
//
// Upgrading the WebSocket is fast; establishing the Vertex Live session behind
// it takes roughly two seconds. A client that starts streaming on connect fills
// the socket buffer during that window, the relay then drains it in a burst,
// and Vertex receives several seconds of audio compressed into an instant. It
// is still ingesting that backlog when the turn closes, so the delay lands
// entirely on turn-boundary latency — measured at ~1.8x real-time upload and
// roughly 800 ms of added latency before this rule existed.
const (
	StateConnecting = "CONNECTING"
	StateAsking     = "ASKING"
	StateListening  = "LISTENING"
	StateClosing    = "CLOSING"
	StateEvaluating = "EVALUATING"
	StateSettled    = "SETTLED"
	StateError      = "ERROR"
)

// Transcript sides.
const (
	SideUser = "user"
	SideAI   = "ai"
)

// ServerFrame is any JSON message sent down to the browser.
//
// One flat struct with omitempty rather than a union type: the frontend
// switches on Type, and a flat shape keeps the TypeScript definition trivial.
type ServerFrame struct {
	Type string `json:"type"`

	// Transcript
	Side  string `json:"side,omitempty"`
	Text  string `json:"text,omitempty"`
	Final bool   `json:"final,omitempty"`

	// State
	State     string `json:"state,omitempty"`
	TurnIndex int    `json:"turnIndex,omitempty"`

	// Error
	Code        string `json:"code,omitempty"`
	Recoverable bool   `json:"recoverable,omitempty"`
	Message     string `json:"message,omitempty"`

	// Usage — surfaced live so the per-session unit economics are
	// demonstrable rather than estimated.
	TotalTokens    int64 `json:"totalTokens,omitempty"`
	AudioTokensIn  int64 `json:"audioTokensIn,omitempty"`
	AudioTokensOut int64 `json:"audioTokensOut,omitempty"`

	// TurnID ties a frame to the turn it describes.
	TurnID string `json:"turnId,omitempty"`

	// Band change
	From    int     `json:"from,omitempty"`
	To      int     `json:"to,omitempty"`
	Penalty float64 `json:"penalty,omitempty"`

	// Payload carries structured bodies (an evaluation, a delivery report)
	// that do not fit the flat fields above.
	Payload any `json:"payload,omitempty"`

	// Pong
	T int64 `json:"t,omitempty"`
}

func (f ServerFrame) encode() ([]byte, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("live: encoding %s frame: %w", f.Type, err)
	}
	return b, nil
}

// --- Audio framing --------------------------------------------------------

// AudioSeqPrefixLen is the 4-byte big-endian sequence number prefixed to every
// downstream audio frame.
//
// It exists so the client can detect gaps and reordering, and so buffer
// underruns are measurable rather than merely audible. Underrun count is the
// best early warning of a network problem before the demo.
const AudioSeqPrefixLen = 4

// encodeAudioFrame prefixes a PCM chunk with its sequence number.
func encodeAudioFrame(seq uint32, pcm []byte) []byte {
	out := make([]byte, AudioSeqPrefixLen+len(pcm))
	binary.BigEndian.PutUint32(out[:AudioSeqPrefixLen], seq)
	copy(out[AudioSeqPrefixLen:], pcm)
	return out
}

// DecodeAudioFrame splits a downstream binary frame into its sequence number
// and PCM payload. Exported for cmd/wsprobe and the frontend's test harness.
func DecodeAudioFrame(frame []byte) (seq uint32, pcm []byte, err error) {
	if len(frame) < AudioSeqPrefixLen {
		return 0, nil, fmt.Errorf("live: audio frame too short (%d bytes)", len(frame))
	}
	return binary.BigEndian.Uint32(frame[:AudioSeqPrefixLen]), frame[AudioSeqPrefixLen:], nil
}

// beginDirective triggers the interviewer's opening question.
//
// The bracketed do-not-read-aloud marker is the same convention every injected
// turn uses. Without it the model will happily narrate the instruction.
const beginDirective = "[SESSION START — this is a system directive, not the candidate speaking. " +
	"Do not read this aloud and do not acknowledge it.] " +
	"Begin the interview now. Greet the candidate in one short sentence, then ask your first question."
