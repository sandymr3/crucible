package grading

import (
	"context"

	"github.com/santh/crucible/internal/evaluator"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/study"
	"github.com/santh/crucible/internal/vertexai"
)

// Decompose builds a syllabus for a topic.
func (s *Service) Decompose(ctx context.Context, topic string, depth study.Depth, syllabusText string) (*study.Syllabus, error) {
	return s.study.Decompose(ctx, topic, depth, syllabusText)
}

// NextQuestion generates the next drill question.
func (s *Service) NextQuestion(ctx context.Context, syl *study.Syllabus, st *study.Subtopic) (string, error) {
	return s.study.NextQuestion(ctx, syl, st)
}

// GradeStudyAnswer grades a drill answer with the interview evaluator.
//
// This is AD-6 paying off: Study Mode gets span-level heatmaps, the four rubric
// dimensions, and the concept lists that feed the roadmap, without a second
// grading implementation to keep consistent with the first.
//
// Synchronous rather than queued, unlike an interview turn. The learner is
// sitting on a form waiting for the result, and there is no conversation to
// keep moving in the meantime.
func (s *Service) GradeStudyAnswer(ctx context.Context, sessionID string, syl *study.Syllabus,
	st *study.Subtopic, question, answer string) (*store.Evaluation, error) {

	ctx = vertexai.WithSession(ctx, sessionID)

	// Map the archetype onto a difficulty band so the evaluator pitches its
	// expectations correctly: reciting a definition should not be graded
	// against the standard applied to a teach-back.
	band := 2
	switch st.Archetype {
	case study.ArchetypeApplication:
		band = 3
	case study.ArchetypeEdgeCase:
		band = 4
	case study.ArchetypeTeachBack:
		band = 4
	}

	return s.eval.Evaluate(ctx, evaluator.Input{
		TurnID:     st.ID,
		Question:   question,
		Transcript: answer,
		RoleTitle:  syl.Topic,
		Seniority:  "learner",
		Band:       band,
		// The Tech Lead's weighting is the right default for drilling: it
		// favours technical accuracy and depth over presentation.
		//
		// One exception. A teach-back is BY DEFINITION a communication test —
		// the whole point is whether they can make someone else understand —
		// so grading it on accuracy alone would miss what is being examined.
		Persona:     personaForArchetype(st.Archetype),
		DomainVocab: []string{syl.Topic, st.Name},
	})
}

func personaForArchetype(a study.Archetype) store.Persona {
	if a == study.ArchetypeTeachBack {
		return store.PersonaPM // communication-weighted
	}
	return store.PersonaTechLead
}
