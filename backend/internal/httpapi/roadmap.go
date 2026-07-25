package httpapi

import (
	"errors"
	"net/http"

	"github.com/santh/crucible/internal/authn"
	"github.com/santh/crucible/internal/roadmap"
	"github.com/santh/crucible/internal/store"
)

// registerRoadmap mounts the roadmap and retest routes.
func (a *API) registerRoadmap(mux *http.ServeMux, authed func(http.Handler) http.Handler) {
	h := func(fn http.HandlerFunc) http.Handler { return authed(fn) }
	mux.Handle("GET /v1/sessions/{id}/roadmap", h(a.getRoadmap))
	mux.Handle("POST /v1/sessions/{id}/retest", h(a.retest))
}

// getRoadmap returns the plan, or 202 while it is still being built.
//
// Same polling contract as the report. The roadmap is queued only after the
// report is written, so it is legitimately later.
func (a *API) getRoadmap(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	sess, err := a.store.GetSession(r.Context(), sessionID, uid)
	if err != nil {
		a.renderStoreError(w, err, "reading roadmap")
		return
	}

	var plan roadmap.Plan
	if err := a.store.GetRoadmap(r.Context(), sessionID, &plan); err != nil {
		if errors.Is(err, store.ErrNotFound) {
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
		internalError(w, a.log, "reading roadmap", err)
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

// retest materialises a roadmap's retest_plan into a new, pre-configured
// session.
//
// This is what closes the loop: the roadmap ends by pointing back into the
// product rather than at a dead end. The new session inherits the original's
// digest — same resume, same JD — so the candidate does not re-upload anything,
// and starts at the recommended band rather than the default.
func (a *API) retest(w http.ResponseWriter, r *http.Request) {
	user, _ := authn.FromContext(r.Context())
	sourceID := r.PathValue("id")

	source, err := a.store.GetSession(r.Context(), sourceID, user.UID)
	if err != nil {
		a.renderStoreError(w, err, "starting retest")
		return
	}

	var plan roadmap.Plan
	if err := a.store.GetRoadmap(r.Context(), sourceID, &plan); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusAccepted, map[string]any{"status": "roadmap_not_ready"})
			return
		}
		internalError(w, a.log, "starting retest", err)
		return
	}

	// A retest is a real session and consumes a daily allocation. Exempting it
	// would make the cap trivially bypassable.
	if err := a.guard.CheckDailyCap(r.Context(), user.UID); err != nil {
		writeJSON(w, http.StatusTooManyRequests, errorBody{
			Error: "daily_cap_reached", Message: err.Error(),
		})
		return
	}

	persona := store.Persona(plan.Retest.RecommendedPersona)
	if !persona.Valid() {
		persona = source.Persona
	}
	band := plan.Retest.RecommendedBand
	if band < 2 || band > 5 {
		band = source.DifficultyBand
	}

	sess, err := a.store.CreateSession(r.Context(), &store.Session{
		UID:            user.UID,
		Mode:           store.ModeInterview,
		Status:         store.StatusConfiguring,
		Persona:        persona,
		DifficultyBand: band,
		// Carry the digest and JD forward so the candidate re-enters the room
		// immediately rather than re-uploading their resume.
		Digest:       source.Digest,
		JDText:       source.JDText,
		ResumeGCSURI: source.ResumeGCSURI,
	})
	if err != nil {
		internalError(w, a.log, "creating retest session", err)
		return
	}

	a.log.Info("retest session created",
		"session_id", sess.ID, "from_session", sourceID,
		"persona", persona, "band", band, "focus", plan.Retest.FocusAreas)

	writeJSON(w, http.StatusCreated, map[string]any{
		"sessionId":  sess.ID,
		"persona":    string(persona),
		"band":       band,
		"focusAreas": plan.Retest.FocusAreas,
	})
}
