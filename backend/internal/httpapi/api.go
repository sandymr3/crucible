// Package httpapi is the REST surface (PRD §19.1).
//
// Everything under /v1 requires a verified Firebase ID token. Handlers stay
// thin: validate, delegate, render. Business logic belongs in the packages that
// own it.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/santh/crucible/internal/authn"
	"github.com/santh/crucible/internal/blob"
	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/guardrails"
	"github.com/santh/crucible/internal/ingest"
	"github.com/santh/crucible/internal/store"
)

// API holds the handler dependencies.
type API struct {
	cfg    *config.Config
	log    *slog.Logger
	store  *store.Store
	guard  *guardrails.Guard
	blob   *blob.Store
	ingest *Ingester
	// finalizer queues report generation. An interface so httpapi does not
	// import the grading service and create an import cycle.
	finalizer Finalizer
	study     StudyEngine
}

// Finalizer queues report generation for a completed session.
type Finalizer interface {
	Finalize(sessionID string) error
}

// Ingester is the digest builder this package depends on.
type Ingester = ingest.Ingester

// New builds the API.
func New(cfg *config.Config, log *slog.Logger, st *store.Store, guard *guardrails.Guard,
	bl *blob.Store, ing *Ingester, fin Finalizer, se StudyEngine) *API {
	return &API{cfg: cfg, log: log, store: st, guard: guard, blob: bl,
		ingest: ing, finalizer: fin, study: se}
}

// Register mounts the routes onto mux. The caller wraps them with auth.
func (a *API) Register(mux *http.ServeMux, authed func(http.Handler) http.Handler) {
	h := func(fn http.HandlerFunc) http.Handler { return authed(fn) }

	mux.Handle("POST /v1/sessions", h(a.createSession))
	mux.Handle("GET /v1/sessions", h(a.listSessions))
	mux.Handle("GET /v1/sessions/{id}", h(a.getSession))
	mux.Handle("POST /v1/sessions/{id}/jd", h(a.attachJD))
	mux.Handle("POST /v1/sessions/{id}/end", h(a.endSession))
	mux.Handle("GET /v1/sessions/{id}/usage", h(a.sessionUsage))
	mux.Handle("GET /v1/me", h(a.me))

	a.registerIngest(mux, authed)
	a.registerReport(mux, authed)
	a.registerRoadmap(mux, authed)
	a.registerStudy(mux, authed)
}

// --- Handlers -------------------------------------------------------------

type createSessionRequest struct {
	Mode      store.Mode    `json:"mode"`
	Persona   store.Persona `json:"persona,omitempty"`
	Topic     string        `json:"topic,omitempty"`
	FixtureID string        `json:"fixtureId,omitempty"`
}

func (a *API) createSession(w http.ResponseWriter, r *http.Request) {
	user, _ := authn.FromContext(r.Context())

	var req createSessionRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid_body", err.Error())
		return
	}

	if req.Mode == "" {
		req.Mode = store.ModeInterview
	}
	switch req.Mode {
	case store.ModeInterview:
		// Persona is optional at creation; the user picks it on the persona
		// screen before entering the room. But a value that IS supplied must
		// be valid, or a typo silently becomes the wrong rubric.
		if req.Persona != "" && !req.Persona.Valid() {
			badRequest(w, "invalid_persona", "persona must be tech_lead, architect, or pm")
			return
		}
	case store.ModeStudy:
		if req.Topic == "" {
			badRequest(w, "missing_topic", "study mode requires a topic")
			return
		}
	case store.ModeReplay:
		if req.FixtureID == "" {
			badRequest(w, "missing_fixture", "replay mode requires a fixtureId")
			return
		}
	default:
		badRequest(w, "invalid_mode", "mode must be interview, study, or replay")
		return
	}

	// Replay sessions serve a recording and cost nothing, so they must not
	// consume a daily allocation — otherwise rehearsing the demo burns the
	// budget meant for the demo.
	if req.Mode != store.ModeReplay {
		if err := a.guard.CheckDailyCap(r.Context(), user.UID); err != nil {
			if errors.Is(err, guardrails.ErrDailyCapReached) {
				writeJSON(w, http.StatusTooManyRequests, errorBody{
					Error:   "daily_cap_reached",
					Message: err.Error(),
				})
				return
			}
			internalError(w, a.log, "daily cap check", err)
			return
		}
	}

	// Entry band 3 is the mid-level default. Never 1 for a candidate with a
	// real resume: it is insulting, and it wastes the opening of a short demo.
	band := 3
	if req.Mode == store.ModeStudy {
		band = 2
	}

	sess, err := a.store.CreateSession(r.Context(), &store.Session{
		UID:            user.UID,
		Mode:           req.Mode,
		Status:         store.StatusConfiguring,
		Persona:        req.Persona,
		Topic:          req.Topic,
		FixtureID:      req.FixtureID,
		DifficultyBand: band,
	})
	if err != nil {
		internalError(w, a.log, "creating session", err)
		return
	}

	// Best-effort profile upsert; a failure here must not fail the request.
	_ = a.store.UpsertUser(r.Context(), store.User{
		UID: user.UID, Email: user.Email, DisplayName: user.DisplayName,
	})

	a.log.Info("session created",
		"session_id", sess.ID, "uid", user.UID, "mode", req.Mode, "band", band)
	writeJSON(w, http.StatusCreated, sess)
}

func (a *API) getSession(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())

	sess, err := a.store.GetSession(r.Context(), r.PathValue("id"), uid)
	if err != nil {
		a.renderStoreError(w, err, "getting session")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (a *API) listSessions(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())

	sessions, err := a.store.ListSessions(r.Context(), uid, 20)
	if err != nil {
		internalError(w, a.log, "listing sessions", err)
		return
	}
	if sessions == nil {
		sessions = []*store.Session{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

type attachJDRequest struct {
	Text string `json:"text"`
}

// maxJDChars matches the PRD's ingestion limit. Enforced server-side because
// the digest call's cost scales with it.
const maxJDChars = 20000

func (a *API) attachJD(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())

	var req attachJDRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid_body", err.Error())
		return
	}
	if req.Text == "" {
		badRequest(w, "missing_text", "job description text is required")
		return
	}
	if len(req.Text) > maxJDChars {
		badRequest(w, "jd_too_long", "job description exceeds 20000 characters")
		return
	}

	if err := a.store.UpdateSession(r.Context(), r.PathValue("id"), uid, map[string]any{
		"jdText": req.Text,
	}); err != nil {
		a.renderStoreError(w, err, "attaching jd")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (a *API) endSession(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	sess, err := a.store.GetSession(r.Context(), sessionID, uid)
	if err != nil {
		a.renderStoreError(w, err, "ending session")
		return
	}

	// Idempotent: ending an already-ended session is a success, not an error.
	// The client's unload handler and an explicit End click routinely race.
	if sess.Status == store.StatusComplete {
		writeJSON(w, http.StatusOK, map[string]any{"status": "already_complete"})
		return
	}

	now := time.Now()
	fields := map[string]any{
		"status":  string(store.StatusComplete),
		"endedAt": now,
	}
	if sess.StartedAt != nil {
		fields["durationMs"] = now.Sub(*sess.StartedAt).Milliseconds()
	}

	if err := a.store.UpdateSession(r.Context(), sessionID, uid, fields); err != nil {
		a.renderStoreError(w, err, "ending session")
		return
	}

	// Queue the report and return immediately. The contract is that the client
	// polls GET /report, which answers 202 until it is ready.
	if a.finalizer != nil {
		if err := a.finalizer.Finalize(sessionID); err != nil {
			a.log.Error("could not queue finalization", "session_id", sessionID, "error", err.Error())
		}
	}

	a.log.Info("session ended", "session_id", sessionID, "uid", uid)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "ending"})
}

// sessionUsage exposes the per-session token and cost breakdown.
func (a *API) sessionUsage(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())

	sess, err := a.store.GetSession(r.Context(), r.PathValue("id"), uid)
	if err != nil {
		a.renderStoreError(w, err, "reading session usage")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sess.ID,
		"cost":      sess.Cost,
		"turnCount": sess.TurnCount,
	})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	user, _ := authn.FromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"uid":         user.UID,
		"email":       user.Email,
		"displayName": user.DisplayName,
		"anonymous":   user.Anonymous,
	})
}

// --- Helpers --------------------------------------------------------------

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// renderStoreError maps store errors onto status codes.
//
// ErrForbidden deliberately renders as 404, not 403: a 403 confirms that a
// session ID exists, which turns the endpoint into an enumeration oracle. The
// real cause is logged.
func (a *API) renderStoreError(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not_found"})
	case errors.Is(err, store.ErrForbidden):
		a.log.Warn("cross-user access attempt", "op", op)
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not_found"})
	default:
		internalError(w, a.log, op, err)
	}
}

// maxBodyBytes bounds a JSON request body.
const maxBodyBytes = 1 << 20 // 1 MiB

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func badRequest(w http.ResponseWriter, code, msg string) {
	writeJSON(w, http.StatusBadRequest, errorBody{Error: code, Message: msg})
}

// internalError logs the cause and returns an opaque body. Internal error text
// can carry document paths and project identifiers, which the client has no
// use for and an attacker does.
func internalError(w http.ResponseWriter, log *slog.Logger, op string, err error) {
	log.Error("request failed", "op", op, "error", err.Error())
	writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal_error"})
}
