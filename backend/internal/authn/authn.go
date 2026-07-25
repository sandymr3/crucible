// Package authn verifies Firebase ID tokens.
//
// Two entry points because the two transports differ: REST reads the
// Authorization header, and the WebSocket reads a query parameter — browsers
// cannot set headers on a WebSocket handshake, so there is no alternative.
// Both run the same verification.
package authn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"

	"github.com/santh/crucible/internal/logging"
)

// ErrUnauthenticated covers every rejection reason.
//
// One error rather than several: distinguishing "malformed token" from
// "expired" from "wrong audience" in the response tells an attacker which
// direction to iterate, and tells a legitimate user nothing they can act on.
// The specific cause is logged, not returned.
var ErrUnauthenticated = errors.New("authn: unauthenticated")

type ctxKey int

const ctxKeyUser ctxKey = iota

// User is the authenticated caller.
type User struct {
	UID         string
	Email       string
	DisplayName string
	Anonymous   bool
}

// Verifier checks ID tokens against a Firebase project.
type Verifier struct {
	client *fbauth.Client
	log    *slog.Logger

	// devAllowAnon bypasses verification entirely for local development.
	// Gated behind an explicit env var and refused on Cloud Run, because an
	// unauthenticated live socket in front of a billing API is a credit leak
	// waiting to be discovered.
	devAllowAnon bool
}

// NewVerifier builds a verifier for the given project.
func NewVerifier(ctx context.Context, projectID string, log *slog.Logger, devAllowAnon bool) (*Verifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("authn: initialising firebase app: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("authn: initialising firebase auth: %w", err)
	}

	if devAllowAnon {
		log.Warn("DEV_ALLOW_ANON is set: authentication is BYPASSED. Never enable this on a deployed service.")
	}
	return &Verifier{client: client, log: log, devAllowAnon: devAllowAnon}, nil
}

// Verify checks a raw ID token and returns the caller.
func (v *Verifier) Verify(ctx context.Context, idToken string) (*User, error) {
	if v.devAllowAnon && idToken == "" {
		return &User{UID: "dev-user", DisplayName: "Local Dev", Anonymous: true}, nil
	}
	if idToken == "" {
		return nil, ErrUnauthenticated
	}

	tok, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		// Logged, not returned: the caller gets a bare 401.
		v.log.Warn("id token rejected", "reason", err.Error())
		return nil, ErrUnauthenticated
	}

	u := &User{UID: tok.UID}
	if email, ok := tok.Claims["email"].(string); ok {
		u.Email = email
	}
	if name, ok := tok.Claims["name"].(string); ok {
		u.DisplayName = name
	}
	if provider, ok := tok.Firebase.SignInProvider, true; ok {
		u.Anonymous = provider == "anonymous"
	}
	return u, nil
}

// Middleware enforces authentication on REST routes.
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := v.Verify(r.Context(), bearerToken(r))
		if err != nil {
			unauthorized(w)
			return
		}

		ctx := WithUser(r.Context(), user)
		ctx = logging.WithUser(ctx, user.UID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// VerifyRequest authenticates a request that is about to be upgraded to a
// WebSocket, where the token arrives as a query parameter.
//
// The exposure that creates — tokens in URLs get logged by proxies — is
// mitigated by Firebase ID tokens being short-lived, and by this codebase never
// logging a full URL. Do not add a request logger that prints r.URL.
func (v *Verifier) VerifyRequest(r *http.Request) (*User, error) {
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	return v.Verify(r.Context(), token)
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	if after, found := strings.CutPrefix(h, "Bearer "); found {
		return strings.TrimSpace(after)
	}
	return ""
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="crucible"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthenticated"}`))
}

// WithUser attaches an authenticated user to a context.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxKeyUser, u)
}

// FromContext returns the authenticated user, if any.
func FromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(*User)
	return u, ok
}

// MustUID returns the caller's uid, or "" when unauthenticated. Handlers behind
// Middleware can rely on it being non-empty.
func MustUID(ctx context.Context) string {
	if u, ok := FromContext(ctx); ok {
		return u.UID
	}
	return ""
}
