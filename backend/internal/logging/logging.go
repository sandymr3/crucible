// Package logging provides structured JSON logging shaped for Cloud Logging.
//
// There is no time for a real observability stack, so this exists to make the
// four things that will actually break legible: live connection establishment,
// turn-boundary latency, evaluation duration and failure rate, and span
// anchoring drop rate (PRD §20.4). Everything goes to stdout as JSON; Cloud
// Logging picks it up for free.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// Context keys for correlation IDs. Unexported type so nothing outside this
// package can collide with them.
type ctxKey int

const (
	ctxKeySession ctxKey = iota
	ctxKeyUser
	ctxKeyRequest
)

// New builds the process logger. Cloud Logging keys off "severity" rather than
// slog's default "level", and off "message" rather than "msg", so we rename
// both at the handler rather than at every call site.
func New(level string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.LevelKey:
				a.Key = "severity"
				// Cloud Logging expects WARNING, not WARN.
				if a.Value.String() == "WARN" {
					a.Value = slog.StringValue("WARNING")
				}
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	})
	return slog.New(h)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithSession tags a context with the session ID so every downstream log line
// can be filtered to one interview. This is what makes a failed demo run
// debuggable after the fact.
func WithSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, ctxKeySession, sessionID)
}

// WithUser tags a context with the authenticated uid.
func WithUser(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, ctxKeyUser, uid)
}

// WithRequest tags a context with a request ID.
func WithRequest(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxKeyRequest, requestID)
}

// From returns a logger carrying whichever correlation IDs the context holds.
func From(ctx context.Context, base *slog.Logger) *slog.Logger {
	l := base
	if v, ok := ctx.Value(ctxKeySession).(string); ok && v != "" {
		l = l.With("session_id", v)
	}
	if v, ok := ctx.Value(ctxKeyUser).(string); ok && v != "" {
		l = l.With("uid", v)
	}
	if v, ok := ctx.Value(ctxKeyRequest).(string); ok && v != "" {
		l = l.With("request_id", v)
	}
	return l
}
