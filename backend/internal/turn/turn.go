// Package turn owns the lifecycle of one question-and-answer exchange.
//
// Deliberately transport-agnostic (AD-6): it knows nothing about WebSockets or
// audio codecs. A spoken answer and a typed answer both produce the same Turn
// and dispatch the same evaluation job, which is the only reason Study Mode,
// the "type instead" accessibility path, and the demo safety net are three uses
// of one code path rather than three implementations.
package turn

import (
	"strings"
	"sync"
	"time"

	"github.com/santh/crucible/internal/store"
)

// State is the turn lifecycle from PRD §6.3.
type State string

const (
	StateQueued     State = "QUEUED"
	StateAsking     State = "ASKING"
	StateListening  State = "LISTENING"
	StateClosing    State = "CLOSING"
	StateEvaluating State = "EVALUATING"
	StateSettled    State = "SETTLED"
)

// Buffer accumulates one turn's material as it arrives.
//
// Concurrent by construction: transcript deltas arrive on the Vertex reader
// goroutine while audio frames arrive on the client reader, and the boundary
// snapshot is taken from a third.
type Buffer struct {
	mu sync.Mutex

	index      int
	state      State
	question   strings.Builder
	transcript strings.Builder

	// audio holds the raw PCM the user spoke, at the Live API's input rate.
	// Retained so the turn can be flushed to a WAV for delivery analysis —
	// filler counts cannot come from the transcript, because speech
	// recognition normalises disfluencies out of it.
	audio []byte

	askedAt         time.Time
	answerStartedAt time.Time
	answerEndedAt   time.Time

	hints     []store.Hint
	inputMode store.InputMode
}

// NewBuffer starts a turn.
func NewBuffer(index int) *Buffer {
	return &Buffer{
		index:     index,
		state:     StateQueued,
		askedAt:   time.Now(),
		inputMode: store.InputVoice,
	}
}

// Index returns the turn's position in the session.
func (b *Buffer) Index() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.index
}

// State returns the current lifecycle state.
func (b *Buffer) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// SetState advances the lifecycle.
func (b *Buffer) SetState(s State) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = s
}

// AppendQuestion accumulates the interviewer's spoken question, sourced from
// the output transcription stream rather than a separate generation — the text
// on screen must be exactly what was said aloud.
func (b *Buffer) AppendQuestion(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.question.WriteString(text)
}

// AppendTranscript accumulates the candidate's finalized speech.
func (b *Buffer) AppendTranscript(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.answerStartedAt.IsZero() {
		b.answerStartedAt = time.Now()
	}
	b.transcript.WriteString(text)
}

// AppendAudio retains a frame of the candidate's speech.
func (b *Buffer) AppendAudio(pcm []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.answerStartedAt.IsZero() {
		b.answerStartedAt = time.Now()
	}
	b.audio = append(b.audio, pcm...)
}

// SetTextAnswer records a typed answer, which takes the same downstream path as
// speech.
func (b *Buffer) SetTextAnswer(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transcript.Reset()
	b.transcript.WriteString(text)
	b.inputMode = store.InputText
	if b.answerStartedAt.IsZero() {
		b.answerStartedAt = time.Now()
	}
}

// AddHint records a Socratic hint and its score penalty.
func (b *Buffer) AddHint(h store.Hint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hints = append(b.hints, h)
}

// HintCount returns how many hints this turn has used.
func (b *Buffer) HintCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.hints)
}

// Transcript returns the answer so far.
func (b *Buffer) Transcript() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.transcript.String())
}

// Question returns the interviewer's question text.
func (b *Buffer) Question() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.question.String())
}

// HasAnswer reports whether anything was said or typed. Used to avoid
// persisting empty turns when a session ends mid-question.
func (b *Buffer) HasAnswer() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.transcript.String()) != "" || len(b.audio) > 0
}

// Snapshot freezes the turn at its boundary and returns the persistable
// document plus the raw audio.
//
// Taking a copy here — rather than reading the live buffer later — is what lets
// the conversation move on to the next question immediately while evaluation
// runs in the background.
func (b *Buffer) Snapshot(audioDurationMs int64) (*store.Turn, []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.answerEndedAt.IsZero() {
		b.answerEndedAt = time.Now()
	}

	t := &store.Turn{
		Index:               b.index,
		QuestionText:        strings.TrimSpace(b.question.String()),
		AskedAt:             b.askedAt,
		UserTranscript:      strings.TrimSpace(b.transcript.String()),
		UserTranscriptFinal: true,
		InputMode:           b.inputMode,
		AudioDurationMs:     audioDurationMs,
		HintsUsed:           len(b.hints),
		Hints:               b.hints,
		GradingStatus:       store.GradingPending,
	}
	if !b.answerStartedAt.IsZero() {
		started := b.answerStartedAt
		t.AnswerStartedAt = &started
	}
	ended := b.answerEndedAt
	t.AnswerEndedAt = &ended

	audio := b.audio
	b.audio = nil // hand ownership to the caller; do not retain a second copy
	return t, audio
}

// WordCount returns the number of words in the answer so far.
func (b *Buffer) WordCount() int {
	return len(strings.Fields(b.Transcript()))
}
