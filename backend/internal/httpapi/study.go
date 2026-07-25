package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/santh/crucible/internal/authn"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/study"
)

// StudyEngine is what the study routes need.
//
// An interface so httpapi does not import the grading service, which imports
// the relay, which would close an import cycle.
type StudyEngine interface {
	Decompose(ctx context.Context, topic string, depth study.Depth, syllabusText string) (*study.Syllabus, error)
	NextQuestion(ctx context.Context, syl *study.Syllabus, st *study.Subtopic) (string, error)
	GradeStudyAnswer(ctx context.Context, sessionID string, syl *study.Syllabus, st *study.Subtopic, question, answer string) (*store.Evaluation, error)
}

// registerStudy mounts Mode B.
//
// Text-first and REST-only: drilling is faster and cheaper typed than spoken,
// and there is no conversational state to hold open. Voice is a per-question
// toggle the frontend can add over the existing live relay.
func (a *API) registerStudy(mux *http.ServeMux, authed func(http.Handler) http.Handler) {
	h := func(fn http.HandlerFunc) http.Handler { return authed(fn) }
	mux.Handle("POST /v1/sessions/{id}/syllabus", h(a.buildSyllabus))
	mux.Handle("GET /v1/sessions/{id}/study/next", h(a.nextDrill))
	mux.Handle("POST /v1/sessions/{id}/study/answer", h(a.submitDrill))
	mux.Handle("GET /v1/sessions/{id}/mastery", h(a.masteryMap))
}

type syllabusRequest struct {
	Depth        study.Depth `json:"depth,omitempty"`
	SyllabusText string      `json:"syllabusText,omitempty"`
}

func (a *API) buildSyllabus(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	var req syllabusRequest
	if r.ContentLength > 0 {
		if err := decode(r, &req); err != nil {
			badRequest(w, "invalid_body", err.Error())
			return
		}
	}

	sess, err := a.store.GetSession(r.Context(), sessionID, uid)
	if err != nil {
		a.renderStoreError(w, err, "building syllabus")
		return
	}
	if sess.Mode != store.ModeStudy {
		badRequest(w, "not_study_mode", "this session is not a study session")
		return
	}
	if strings.TrimSpace(sess.Topic) == "" {
		badRequest(w, "missing_topic", "this session has no topic")
		return
	}

	syl, err := a.study.Decompose(r.Context(), sess.Topic, req.Depth, req.SyllabusText)
	if err != nil {
		internalError(w, a.log, "decomposing syllabus", err)
		return
	}

	// Stored on the session's digest field so Study Mode reuses the existing
	// document shape rather than adding a parallel one.
	if err := a.store.UpdateSession(r.Context(), sessionID, uid, map[string]any{
		"digest": map[string]any{"syllabus": syl},
	}); err != nil {
		a.renderStoreError(w, err, "saving syllabus")
		return
	}

	a.log.Info("syllabus built",
		"session_id", sessionID, "topic", sess.Topic, "subtopics", len(syl.Subtopics))
	writeJSON(w, http.StatusOK, map[string]any{
		"syllabus": syl,
		"mastery":  study.Summarise(syl),
	})
}

// nextDrill returns the next question, or reports that the topic is complete.
func (a *API) nextDrill(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	sess, syl, ok := a.loadSyllabus(w, r, sessionID, uid)
	if !ok {
		return
	}

	next := study.NextSubtopic(syl)
	if next == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"complete": true,
			"mastery":  study.Summarise(syl),
			"message":  "Every subtopic is solid. Nothing left to drill.",
		})
		return
	}

	question, err := a.study.NextQuestion(r.Context(), syl, next)
	if err != nil {
		internalError(w, a.log, "generating drill question", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"complete":       false,
		"subtopicId":     next.ID,
		"subtopic":       next.Name,
		"archetype":      string(next.Archetype),
		"archetypeLabel": next.Archetype.Label(),
		"question":       question,
		"mastery":        study.Summarise(syl),
		"band":           sess.DifficultyBand,
	})
}

type drillAnswer struct {
	SubtopicID string `json:"subtopicId"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

// submitDrill grades an answer and advances the mastery map.
func (a *API) submitDrill(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())
	sessionID := r.PathValue("id")

	var req drillAnswer
	if err := decode(r, &req); err != nil {
		badRequest(w, "invalid_body", err.Error())
		return
	}
	if req.SubtopicID == "" || strings.TrimSpace(req.Answer) == "" {
		badRequest(w, "missing_fields", "subtopicId and answer are required")
		return
	}

	_, syl, ok := a.loadSyllabus(w, r, sessionID, uid)
	if !ok {
		return
	}

	idx := -1
	for i := range syl.Subtopics {
		if syl.Subtopics[i].ID == req.SubtopicID {
			idx = i
			break
		}
	}
	if idx < 0 {
		badRequest(w, "unknown_subtopic", "no such subtopic in this syllabus")
		return
	}
	st := &syl.Subtopics[idx]

	// The SAME evaluator the interview uses (AD-6). Study Mode gets span-level
	// feedback and the four rubric dimensions for free.
	eval, err := a.study.GradeStudyAnswer(r.Context(), sessionID, syl, st, req.Question, req.Answer)
	if err != nil {
		internalError(w, a.log, "grading drill answer", err)
		return
	}

	progress := study.ApplyAnswer(syl, req.SubtopicID, eval)

	if err := a.store.UpdateSession(r.Context(), sessionID, uid, map[string]any{
		"digest":           map[string]any{"syllabus": syl},
		"coverage.proven":  study.ProvenConcepts(syl),
		"coverage.missing": study.MissingConcepts(syl),
	}); err != nil {
		a.renderStoreError(w, err, "saving drill progress")
		return
	}

	a.log.Info("drill answer graded",
		"session_id", sessionID, "subtopic", st.Name,
		"archetype", string(progress.NextArchetype),
		"score", eval.TurnScore,
		"mastery", string(progress.From)+"->"+string(progress.To),
		"unlocked", progress.Unlocked)

	writeJSON(w, http.StatusOK, map[string]any{
		"evaluation":    eval,
		"masteryFrom":   string(progress.From),
		"masteryTo":     string(progress.To),
		"passed":        progress.Passed,
		"nextArchetype": string(progress.NextArchetype),
		"unlocked":      progress.Unlocked,
		"mastery":       study.Summarise(syl),
		"complete":      study.Complete(syl),
	})
}

// masteryMap returns the graph plus progress, for the dependency-graph render.
func (a *API) masteryMap(w http.ResponseWriter, r *http.Request) {
	uid := authn.MustUID(r.Context())

	_, syl, ok := a.loadSyllabus(w, r, r.PathValue("id"), uid)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"topic":     syl.Topic,
		"subtopics": syl.Subtopics,
		"mastery":   study.Summarise(syl),
		"complete":  study.Complete(syl),
	})
}

// loadSyllabus reads and decodes the syllabus off the session.
func (a *API) loadSyllabus(w http.ResponseWriter, r *http.Request, sessionID, uid string) (*store.Session, *study.Syllabus, bool) {
	sess, err := a.store.GetSession(r.Context(), sessionID, uid)
	if err != nil {
		a.renderStoreError(w, err, "loading syllabus")
		return nil, nil, false
	}

	raw, ok := sess.Digest["syllabus"]
	if !ok {
		badRequest(w, "no_syllabus", "build the syllabus first")
		return nil, nil, false
	}

	// Round-trip through JSON: the digest is stored untyped, and this is the
	// least fragile way back to a struct.
	b, err := json.Marshal(raw)
	if err != nil {
		internalError(w, a.log, "encoding syllabus", err)
		return nil, nil, false
	}
	var syl study.Syllabus
	if err := json.Unmarshal(b, &syl); err != nil {
		internalError(w, a.log, "decoding syllabus", err)
		return nil, nil, false
	}
	if len(syl.Subtopics) == 0 {
		badRequest(w, "empty_syllabus", "this syllabus has no subtopics")
		return nil, nil, false
	}
	return sess, &syl, true
}

var _ = time.Now
