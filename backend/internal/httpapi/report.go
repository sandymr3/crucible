package httpapi

import (
	"errors"
	"net/http"

	"github.com/santh/crucible/internal/authn"
	"github.com/santh/crucible/internal/report"
	"github.com/santh/crucible/internal/store"
)

// registerReport mounts the post-session routes.
func (a *API) registerReport(mux *http.ServeMux, authed func(http.Handler) http.Handler) {
	h := func(fn http.HandlerFunc) http.Handler { return authed(fn) }
	mux.Handle("GET /v1/sessions/{id}/report", h(a.getReport))
	mux.Handle("GET /v1/sessions/{id}/turns", h(a.listTurns))
}

// getReport returns the report, or 202 while it is still being generated.
//
// The 202 contract exists because finalization waits on outstanding grades and
// can legitimately take tens of seconds. A client that gets 202 knows to keep
// polling rather than to show an error.
func (a *API) getReport(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	sess, err := a.store.GetSession(r.Context(), sessionID, uid)
	if err != nil {
		a.renderStoreError(w, err, "reading report")
		return
	}

	var rep report.Report
	if err := a.store.GetReport(r.Context(), sessionID, &rep); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Distinguish "still working" from "never asked for". A session
			// that was never ended has no report coming, and telling the
			// client to poll forever would be a lie.
			status := "generating"
			if sess.Status != store.StatusComplete && sess.Status != store.StatusEvaluating {
				status = "not_started"
			}
			writeJSON(w, http.StatusAccepted, map[string]any{
				"status":        status,
				"sessionStatus": string(sess.Status),
			})
			return
		}
		internalError(w, a.log, "reading report", err)
		return
	}

	writeJSON(w, http.StatusOK, rep)
}

// listTurns returns every turn with its evaluation embedded, which is what the
// per-turn accordion renders from.
func (a *API) listTurns(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	if _, err := a.store.GetSession(r.Context(), sessionID, uid); err != nil {
		a.renderStoreError(w, err, "listing turns")
		return
	}

	turns, err := a.store.ListTurns(r.Context(), sessionID)
	if err != nil {
		internalError(w, a.log, "listing turns", err)
		return
	}
	if turns == nil {
		turns = []*store.Turn{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"turns": turns})
}
