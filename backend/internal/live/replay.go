package live

import (
	"context"
	"time"

	"github.com/santh/crucible/internal/replay"
)

// runReplay serves a recorded session instead of opening a Vertex connection.
//
// The client cannot tell the difference: same frame types, same sequence
// numbers, same timings. That is the whole point — a replayed demo is
// indistinguishable from a live one, and it cannot be broken by venue wifi, a
// rate limit, or a Vertex outage.
func (s *session) runReplay(ctx context.Context, fixtureID string) {
	f, err := replay.Get(fixtureID)
	if err != nil {
		s.log.Error("replay fixture unavailable", "fixture", fixtureID, "error", err.Error())
		s.send(ServerFrame{
			Type: TypeError, Code: "fixture_not_found", Recoverable: false,
			Message: "That recorded session is not available.",
		})
		s.drainOutboundBlocking()
		return
	}

	s.log.Info("replaying recorded session",
		"fixture", f.ID, "events", len(f.Events), "duration_ms", f.DurationMs())

	go s.writePump(ctx)
	go s.watchIdle(ctx)

	s.send(ServerFrame{Type: TypeState, State: StateListening})

	// A replay still honours the client's begin signal, so the recorded
	// opening question is not spoken into a page that cannot yet play it.
	ready := make(chan struct{})
	go s.awaitBegin(ctx, ready)

	select {
	case <-ready:
	case <-ctx.Done():
		return
	case <-s.done:
		return
	case <-time.After(30 * time.Second):
		// Begin never arrived. Play anyway rather than sitting silent — on
		// stage, a demo that does nothing is worse than one that starts early.
		s.log.Warn("replay starting without a begin signal")
	}

	if err := replay.Play(f, s, s.replaySpeed(), s.done); err != nil {
		s.log.Error("replay failed", "fixture", f.ID, "error", err.Error())
	}

	s.send(ServerFrame{Type: TypeState, State: StateSettled})
	s.drainOutboundBlocking()
	s.log.Info("replay complete", "fixture", f.ID)
}

// awaitBegin reads client frames until the begin signal arrives, then keeps
// draining so the connection stays responsive to pings and an end request.
func (s *session) awaitBegin(ctx context.Context, ready chan<- struct{}) {
	signalled := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		_, data, err := s.conn.ReadMessage()
		if err != nil {
			s.stop()
			return
		}
		s.touch()

		var f ClientFrame
		if err := decodeJSON(data, &f); err != nil {
			continue
		}
		switch f.Type {
		case TypeBegin:
			if !signalled {
				signalled = true
				close(ready)
			}
		case TypePing:
			s.send(ServerFrame{Type: TypePong, T: f.T})
		case TypeEndSession:
			s.stop()
			return
		}
	}
}

// replaySpeed allows a rehearsal to run faster than real time.
func (s *session) replaySpeed() float64 {
	if s.opts.ReplaySpeed > 0 {
		return s.opts.ReplaySpeed
	}
	return 1.0
}

// EmitJSON implements replay.Emitter.
func (s *session) EmitJSON(payload []byte) {
	s.enqueue(outboundFrame{payload: payload})
}

// EmitAudio implements replay.Emitter, re-prefixing each chunk with a fresh
// sequence number so the client's gap detection works exactly as it would live.
func (s *session) EmitAudio(pcm []byte) {
	s.sendBinary(encodeAudioFrame(s.audioSeq.Add(1), pcm))
}
