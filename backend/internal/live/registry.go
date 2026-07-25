package live

import (
	"context"
	"sync"

	"github.com/santh/crucible/internal/store"
)

// TurnSink receives a completed turn at its boundary.
//
// An interface so the relay stays free of Firestore, the worker pool, and the
// evaluator. The relay's job is moving bytes between two sockets; deciding what
// a finished turn means belongs to the caller.
//
// Implementations MUST NOT block: this is called on the path that also has to
// keep the conversation moving.
type TurnSink interface {
	TurnClosed(ctx context.Context, sessionID string, t *store.Turn, audio []byte)
}

// HintProvider generates a Socratic hint for a turn in progress.
//
// An interface for the same reason as TurnSink: the relay must not import the
// evaluator or the model client.
type HintProvider interface {
	Hint(ctx context.Context, sessionID, question, partial string) (string, error)
}

// SetTurnSink installs the sink. Call before serving.
func (r *Relay) SetTurnSink(s TurnSink) { r.sink = s }

// SetHintProvider installs the hint generator.
func (r *Relay) SetHintProvider(h HintProvider) { r.hints = h }

// register makes a session addressable by ID so background work can push
// frames into it after the fact.
func (r *Relay) register(id string, s *session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		r.sessions = make(map[string]*session)
	}
	r.sessions[id] = s
}

func (r *Relay) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// Publish sends a frame to a live session, reporting whether anyone was
// listening.
//
// This is how an evaluation that finished three seconds after the answer
// reaches the browser. A false return is normal rather than an error: the user
// may have ended the session before their last turn finished grading, and the
// result is still persisted for the report.
func (r *Relay) Publish(sessionID string, f ServerFrame) bool {
	r.mu.RLock()
	s, ok := r.sessions[sessionID]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	s.send(f)
	return true
}

// sessionRegistry is embedded into Relay.
type sessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*session
	sink     TurnSink
	hints    HintProvider
}

// InjectCoachState pushes a system directive into a live session's model
// context, reporting whether a session was there to receive it.
//
// This is how a grade computed after one turn reaches the question asked in the
// next. It travels the same ordered upstream queue as audio, so it can never
// overtake the tail of an answer still being transmitted.
func (r *Relay) InjectCoachState(sessionID, directive string) bool {
	r.mu.RLock()
	s, ok := r.sessions[sessionID]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	s.queueUpstream(upstreamMsg{kind: upText, text: directive})
	return true
}
