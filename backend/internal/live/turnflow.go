package live

import (
	"context"
	"time"

	"github.com/santh/crucible/internal/audio"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/turn"
)

// captureAudio retains a frame of the candidate's speech for the current turn.
//
// Kept even though the same bytes are forwarded to Vertex: delivery metrics
// need the raw audio, because speech recognition normalises disfluencies out of
// the transcript. A filler counter built on the transcript reads zero forever.
func (s *session) captureAudio(pcm []byte) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.turn != nil {
		s.turn.AppendAudio(pcm)
	}
}

// captureUserTranscript accumulates finalized candidate speech.
func (s *session) captureUserTranscript(text string) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.turn != nil {
		s.turn.AppendTranscript(text)
	}
}

// captureQuestion accumulates the interviewer's spoken question.
//
// Sourced from the output transcription stream rather than a separate
// generation, so the text on screen is exactly what was said aloud.
func (s *session) captureQuestion(text string) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.turn != nil {
		s.turn.AppendQuestion(text)
	}
}

// captureTextAnswer records a typed answer.
func (s *session) captureTextAnswer(text string) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.turn != nil {
		s.turn.SetTextAnswer(text)
	}
}

// closeTurn snapshots the finished turn, hands it to the sink, and starts the
// next one.
//
// This is the whole point of the design: the snapshot is taken synchronously so
// the data is consistent, then everything expensive — persisting, uploading
// audio, grading — happens elsewhere. The conversation moves on immediately.
// Nothing here awaits an evaluation.
func (s *session) closeTurn() {
	s.turnMu.Lock()
	current := s.turn
	if current == nil || !current.HasAnswer() {
		// A boundary with nothing in it: the user clicked Done without
		// speaking, or the session ended between questions. Persisting an
		// empty turn would put a blank row in their report.
		s.turnMu.Unlock()
		return
	}
	s.turnIndex++
	s.turn = turn.NewBuffer(s.turnIndex)
	s.turnMu.Unlock()

	audioBytes := s.audioBytes.Swap(0)
	s.audioFrames.Store(0)
	s.firstAudioInAt.Store(0)

	durationMs := audio.Duration(make([]byte, audioBytes), audio.SampleRateIn)
	record, pcm := current.Snapshot(durationMs)

	sink := s.relay.sink
	if sink == nil {
		return
	}

	// Detached context. The common case for a final turn is the user ending
	// the session, which cancels the request context immediately — writing on
	// it would discard exactly the turn we most want graded.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 2*time.Minute)

	go func() {
		defer cancel()
		sink.TurnClosed(ctx, s.opts.SessionID, record, pcm)
	}()

	s.send(ServerFrame{Type: TypeState, State: StateEvaluating, TurnIndex: record.Index})
}

// hintPenalty is the score cost of one hint (PRD §10.2).
const hintPenalty = 0.5

// handleHint generates a Socratic nudge for the turn in progress.
//
// Runs on its own goroutine: the candidate is mid-answer and the read loop must
// keep accepting their audio while the hint is being generated.
func (s *session) handleHint() {
	if s.relay.hints == nil {
		return
	}

	s.turnMu.Lock()
	current := s.turn
	s.turnMu.Unlock()
	if current == nil {
		return
	}

	// A hard cap, because hints are score-bearing. Without it a candidate could
	// walk the interviewer to the answer one nudge at a time.
	if current.HintCount() >= s.relay.cfg.MaxHintsPerTurn {
		s.send(ServerFrame{
			Type: TypeError, Code: "hint_limit", Recoverable: true,
			Message: "You've used all your hints for this question.",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 25*time.Second)
	defer cancel()

	text, err := s.relay.hints.Hint(ctx, s.opts.SessionID, current.Question(), current.Transcript())
	if err != nil || text == "" {
		s.log.Warn("hint generation failed", "error", errString(err))
		s.send(ServerFrame{
			Type: TypeError, Code: "hint_failed", Recoverable: true,
			Message: "Couldn't fetch a hint — keep going.",
		})
		return
	}

	current.AddHint(store.Hint{Text: text, RequestedAt: time.Now(), Penalty: hintPenalty})

	// The hint is delivered as TEXT, never injected into the model's context.
	// Injecting it would have the interviewer read the hint aloud, which both
	// gives the game away and breaks the persona's "never supply the answer"
	// rule.
	s.send(ServerFrame{Type: TypeHint, Text: text, Penalty: hintPenalty})
	s.log.Info("hint delivered", "hints_used", current.HintCount())
}

func errString(err error) string {
	if err == nil {
		return "empty hint"
	}
	return err.Error()
}
