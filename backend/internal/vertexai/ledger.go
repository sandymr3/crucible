package vertexai

import (
	"context"
	"log/slog"

	"github.com/santh/crucible/internal/store"
)

// sessionKey is how a session ID rides along in a context so the ledger can
// attribute a call without every call site growing a parameter.
type sessionKey struct{}

// WithSession tags a context so any Vertex usage on it is attributed to a
// session. Calls made without this still land in the daily ledger; they just
// are not attributable to one interview.
func WithSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionKey{}, sessionID)
}

func sessionFrom(ctx context.Context) string {
	if v, ok := ctx.Value(sessionKey{}).(string); ok {
		return v
	}
	return ""
}

// FirestoreLedger persists token usage.
//
// PRD §21.3 asks for this from day one, for two reasons: the real numbers are
// needed to tune the guardrails, and "here is our actual per-session unit
// economics" is a genuinely strong answer to a judge asking about viability.
// Neither is available retroactively.
type FirestoreLedger struct {
	store *store.Store
	log   *slog.Logger
}

// NewFirestoreLedger builds the ledger.
func NewFirestoreLedger(st *store.Store, log *slog.Logger) *FirestoreLedger {
	return &FirestoreLedger{store: st, log: log}
}

// Record persists one call's usage.
//
// Failures are logged and swallowed. Accounting must never break inference: a
// Firestore hiccup should cost a data point, not an interview.
func (l *FirestoreLedger) Record(ctx context.Context, u *Usage) {
	if u == nil || u.TotalTokens == 0 {
		return
	}

	// Detach from the caller's context. The common case is a live session
	// ending, which cancels its context immediately — writing on it would
	// discard exactly the usage record for the turn we most want counted.
	writeCtx := context.WithoutCancel(ctx)

	err := l.store.RecordUsage(writeCtx, sessionFrom(ctx), u.Model, store.CostEstimate{
		TotalTokens:         u.TotalTokens,
		PromptAudioTokens:   u.PromptAudioTokens,
		ResponseAudioTokens: u.ResponseAudioTokens,
		PromptTextTokens:    u.PromptTextTokens,
		ResponseTextTokens:  u.ResponseTextTokens,
		Calls:               1,
	})
	if err != nil {
		l.log.Warn("usage not recorded", "model", u.Model, "error", err.Error())
	}
}
