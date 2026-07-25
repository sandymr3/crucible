package study

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/santh/crucible/internal/prompts"
)

// archetypeBrief tells the generator what shape of question to write.
var archetypeBrief = map[Archetype]string{
	ArchetypeRecall: "Ask them to state it precisely. Definition, formula, or " +
		"mechanism, with the terms named.",
	ArchetypeApplication: "Give a concrete situation with real numbers or a named " +
		"scenario, and ask what happens or what they would do. They must apply the " +
		"idea, not restate it.",
	ArchetypeEdgeCase: "Ask what breaks. Remove a constraint, push a parameter to an " +
		"extreme, or violate an assumption, and ask what fails and why.",
	ArchetypeTeachBack: "Ask them to explain it to a specific audience with specific " +
		"prior knowledge. Name what that audience already knows and what they do not. " +
		"Forbid one term they would lean on — that constraint is what separates " +
		"understanding from recitation.",
}

const questionTimeout = 45 * time.Second

// NextQuestion generates a drill question for a subtopic.
//
// Uses the cheap model: this is a small, well-specified generation task and the
// learner is waiting on it between answers.
func (d *Decomposer) NextQuestion(ctx context.Context, syl *Syllabus, st *Subtopic) (string, error) {
	p, err := prompts.Get(prompts.StudyQuestion)
	if err != nil {
		return "", err
	}

	why := ""
	if st.Why != "" {
		why = "This subtopic matters because: " + st.Why
	}

	instruction := p.Render(map[string]string{
		"TOPIC":           syl.Topic,
		"SUBTOPIC":        st.Name,
		"SUBTOPIC_WHY":    why,
		"ARCHETYPE":       string(st.Archetype),
		"ARCHETYPE_BRIEF": archetypeBrief[st.Archetype],
		"PROVEN":          orNone(ProvenConcepts(syl)),
	})

	ctx, cancel := context.WithTimeout(ctx, questionTimeout)
	defer cancel()

	// Warmer than grading: a drill question should not read like a form field.
	temp := float32(0.7)
	maxTokens := int32(160)

	text, err := d.vx.GenerateText(ctx, d.cfg.ModelCheap, instruction,
		&genai.GenerateContentConfig{Temperature: &temp, MaxOutputTokens: maxTokens})
	if err != nil {
		return "", fmt.Errorf("study: question generation failed: %w", err)
	}

	q := strings.TrimSpace(strings.Trim(strings.TrimSpace(text), `"`))
	if q == "" {
		return "", fmt.Errorf("study: model returned an empty question")
	}
	return q, nil
}

func orNone(items []string) string {
	if len(items) == 0 {
		return "nothing yet"
	}
	return strings.Join(items, ", ")
}
