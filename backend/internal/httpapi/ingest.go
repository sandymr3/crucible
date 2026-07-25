package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/santh/crucible/internal/authn"
	"github.com/santh/crucible/internal/blob"
	"github.com/santh/crucible/internal/ingest"
	"github.com/santh/crucible/internal/persona"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/vertexai"
)

// registerIngest mounts the configuration-screen routes.
func (a *API) registerIngest(mux *http.ServeMux, authed func(http.Handler) http.Handler) {
	h := func(fn http.HandlerFunc) http.Handler { return authed(fn) }

	mux.Handle("POST /v1/sessions/{id}/resume", h(a.uploadResume))
	mux.Handle("POST /v1/sessions/{id}/digest", h(a.buildDigest))
	mux.Handle("PATCH /v1/sessions/{id}/plan", h(a.editPlan))

	// Persona cards are static config, but they still require auth: there is no
	// reason to expose product surface to unauthenticated callers.
	mux.Handle("GET /v1/personas", h(a.listPersonas))
}

// uploadResume accepts a PDF and stores it in Cloud Storage.
func (a *API) uploadResume(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	if _, err := a.store.GetSession(r.Context(), sessionID, uid); err != nil {
		a.renderStoreError(w, err, "uploading resume")
		return
	}

	// Bound the request before parsing so a huge upload cannot exhaust memory
	// on the way in. The stream is bounded again inside blob.Upload, because a
	// Content-Length header is a claim, not a fact.
	r.Body = http.MaxBytesReader(w, r.Body, blob.MaxResumeBytes+1024)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		badRequest(w, "invalid_upload", "could not parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		badRequest(w, "missing_file", "expected a form field named 'file'")
		return
	}
	defer file.Close()

	if !strings.EqualFold(strings.TrimSpace(header.Header.Get("Content-Type")), "application/pdf") &&
		!strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		badRequest(w, "not_pdf", "the resume must be a PDF")
		return
	}

	path := blob.ResumePath(uid, sessionID)
	uri, err := a.blob.Upload(r.Context(), path, "application/pdf", file, blob.MaxResumeBytes)
	if err != nil {
		if errors.Is(err, blob.ErrTooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorBody{
				Error: "file_too_large", Message: "the resume must be under 10 MB",
			})
			return
		}
		internalError(w, a.log, "uploading resume", err)
		return
	}

	if err := a.store.UpdateSession(r.Context(), sessionID, uid, map[string]any{
		"resumeGcsUri": uri,
	}); err != nil {
		a.renderStoreError(w, err, "recording resume uri")
		return
	}

	a.log.Info("resume uploaded", "session_id", sessionID, "filename", header.Filename)
	writeJSON(w, http.StatusOK, map[string]any{"gcsUri": uri})
}

// buildDigest runs ingestion synchronously.
//
// Synchronous on purpose: it takes 4–8 s behind a "Reading your resume…"
// screen, and a job plus a poll loop would add machinery for no user-visible
// gain at this duration.
func (a *API) buildDigest(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	sess, err := a.store.GetSession(r.Context(), sessionID, uid)
	if err != nil {
		a.renderStoreError(w, err, "building digest")
		return
	}
	if sess.ResumeGCSURI == "" {
		badRequest(w, "no_resume", "upload a resume before requesting a digest")
		return
	}

	// Attribute the call's tokens to this session so per-session unit
	// economics stay accurate.
	ctx := vertexai.WithSession(r.Context(), sessionID)

	result, err := a.ingest.Build(ctx, sess.ResumeGCSURI, sess.JDText)
	if err != nil {
		var empty *ingest.ErrEmptyDigest
		if errors.As(err, &empty) {
			// A recoverable, user-actionable failure — almost always a scanned
			// resume. 422 rather than 500: nothing is broken on our side.
			writeJSON(w, http.StatusUnprocessableEntity, errorBody{
				Error:   "empty_digest",
				Message: "We couldn't read anything usable from that resume. If it's a scan or an image, try a text-based PDF.",
			})
			return
		}
		internalError(w, a.log, "building digest", err)
		return
	}

	if err := a.store.UpdateSession(r.Context(), sessionID, uid, map[string]any{
		"digest": result.Digest,
	}); err != nil {
		a.renderStoreError(w, err, "saving digest")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"digest": result.Digest,
		"meta": map[string]any{
			"model":         result.Model,
			"promptVersion": result.PromptVersion,
			"durationMs":    result.DurationMs,
			"claims":        result.ClaimCount,
			"planAreas":     result.PlanCount,
		},
	})
}

type editPlanRequest struct {
	// DroppedAreas names interview_plan areas the user unchecked.
	DroppedAreas []string `json:"droppedAreas"`
}

// editPlan lets the user drop question areas before entering the room.
//
// A small feature with an outsized effect: it converts the tool from something
// happening *to* the user into something they configured.
func (a *API) editPlan(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	var req editPlanRequest
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid_body", err.Error())
		return
	}

	sess, err := a.store.GetSession(r.Context(), sessionID, uid)
	if err != nil {
		a.renderStoreError(w, err, "editing plan")
		return
	}
	plan, ok := sess.Digest["interview_plan"].([]any)
	if !ok {
		badRequest(w, "no_plan", "this session has no interview plan yet")
		return
	}

	dropped := make(map[string]bool, len(req.DroppedAreas))
	for _, name := range req.DroppedAreas {
		dropped[strings.ToLower(strings.TrimSpace(name))] = true
	}

	// Areas are marked rather than removed, so the user's choice stays
	// auditable and is reversible by unchecking the box again.
	remaining := 0
	for _, raw := range plan {
		area, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := area["area"].(string)
		isDropped := dropped[strings.ToLower(strings.TrimSpace(name))]
		area["dropped"] = isDropped
		if !isDropped {
			remaining++
		}
	}

	if remaining == 0 {
		badRequest(w, "empty_plan", "keep at least one question area")
		return
	}

	if err := a.store.UpdateSession(r.Context(), sessionID, uid, map[string]any{
		"digest.interview_plan": plan,
	}); err != nil {
		a.renderStoreError(w, err, "saving plan")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remainingAreas": remaining})
}

// listPersonas returns the three interviewer cards.
func (a *API) listPersonas(w http.ResponseWriter, _ *http.Request) {
	cards := make([]map[string]any, 0, 3)
	for _, p := range persona.List() {
		cards = append(cards, map[string]any{
			"id":      string(p.ID),
			"name":    p.Name,
			"tagline": p.Tagline,
			// Shown on the card so a user can pick the one that scares them.
			"punishes": p.Punishes,
			"weights": map[string]float64{
				"technicalAccuracy":    p.Weights.TechnicalAccuracy,
				"communicationClarity": p.Weights.CommunicationClarity,
				"depth":                p.Weights.Depth,
				"structure":            p.Weights.Structure,
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"personas": cards})
}

// ensure store.Persona stays referenced for the compiler when this file is
// edited independently of api.go.
var _ = store.PersonaTechLead
