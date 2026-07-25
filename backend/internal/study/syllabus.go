// Package study is Mode B: topic-driven drilling.
//
// It exists because the problem statement says "a topic *or* a job role", and
// shipping only the interview half leaves a scoring criterion open.
//
// Almost none of it is new machinery. Evaluation, the difficulty ladder, and
// the roadmap are reused wholesale (AD-6) — only ingestion and question
// generation differ. What Study Mode adds is a real dependency graph over
// subtopics, and a mastery model where "solid" has to be earned by teaching the
// idea back rather than by reciting it.
package study

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/prompts"
	"github.com/santh/crucible/internal/vertexai"
)

// Depth is how far the drilling goes.
type Depth string

const (
	DepthSurvey         Depth = "survey"
	DepthExamReady      Depth = "exam_ready"
	DepthInterviewReady Depth = "interview_ready"
)

// Valid reports whether d is a known depth.
func (d Depth) Valid() bool {
	switch d {
	case DepthSurvey, DepthExamReady, DepthInterviewReady:
		return true
	}
	return false
}

// Mastery is per-subtopic progress (PRD §7.3).
type Mastery string

const (
	MasteryUnseen    Mastery = "unseen"
	MasteryAttempted Mastery = "attempted"
	MasteryShaky     Mastery = "shaky"
	// MasterySolid is reachable ONLY through a correct teach-back. Reciting a
	// definition is not the same as understanding it, and a mastery map that
	// cannot tell those apart is decoration.
	MasterySolid Mastery = "solid"
)

// Subtopic is one node of the syllabus graph.
type Subtopic struct {
	ID   string `firestore:"id" json:"id"`
	Name string `firestore:"name" json:"name"`
	// Prereqs are IDs that should be understood first. These are REAL edges
	// from the decomposition, not a heuristic — which makes this graph better
	// prerequisite information than anything the roadmap can infer.
	Prereqs []string `firestore:"prereqs" json:"prereqs"`
	// Depth is the node's level in the dependency graph, 1 upward.
	Depth int    `firestore:"depth" json:"depth"`
	Why   string `firestore:"why" json:"why"`

	Mastery Mastery `firestore:"mastery" json:"mastery"`
	// Archetype is the next question type to ask for this subtopic.
	Archetype Archetype `firestore:"archetype" json:"archetype"`
	Attempts  int       `firestore:"attempts" json:"attempts"`
	// TeachBackPassed records the one event that unlocks solid.
	TeachBackPassed bool `firestore:"teachBackPassed" json:"teachBackPassed"`
}

// Syllabus is the decomposed topic.
type Syllabus struct {
	Topic     string     `firestore:"topic" json:"topic"`
	Depth     Depth      `firestore:"depth" json:"depth"`
	Subtopics []Subtopic `firestore:"subtopics" json:"subtopics"`
	CreatedAt time.Time  `firestore:"createdAt" json:"createdAt"`
}

// Decomposer turns a topic into a syllabus.
type Decomposer struct {
	cfg *config.Config
	log *slog.Logger
	vx  *vertexai.Client
}

// NewDecomposer builds the decomposer.
func NewDecomposer(cfg *config.Config, log *slog.Logger, vx *vertexai.Client) *Decomposer {
	return &Decomposer{cfg: cfg, log: log, vx: vx}
}

const decomposeTimeout = 90 * time.Second

// Decompose turns a topic string into a dependency-ordered syllabus.
func (d *Decomposer) Decompose(ctx context.Context, topic string, depth Depth, syllabusText string) (*Syllabus, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, fmt.Errorf("study: a topic is required")
	}
	if !depth.Valid() {
		depth = DepthExamReady
	}
	if strings.TrimSpace(syllabusText) == "" {
		syllabusText = "(none supplied — infer the standard scope of this topic)"
	}

	p, err := prompts.Get(prompts.SyllabusDecompose)
	if err != nil {
		return nil, err
	}
	instruction := p.Render(map[string]string{
		"TOPIC":         topic,
		"DEPTH":         string(depth),
		"DEPTH_NOTE":    depthNote(depth),
		"SYLLABUS_TEXT": syllabusText,
	})

	ctx, cancel := context.WithTimeout(ctx, decomposeTimeout)
	defer cancel()

	temp := float32(0.3)
	raw, err := d.vx.GenerateStructured(ctx, d.cfg.ModelReasoning, genai.Text(instruction),
		&genai.GenerateContentConfig{
			Temperature:      &temp,
			ResponseMIMEType: "application/json",
			ResponseSchema:   syllabusSchema(),
		})
	if err != nil {
		return nil, fmt.Errorf("study: decomposition failed: %w", err)
	}

	var parsed struct {
		Subtopics []struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			Prereqs []string `json:"prereqs"`
			Depth   int      `json:"depth"`
			Why     string   `json:"why"`
		} `json:"subtopics"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("study: decomposition was not valid JSON: %w", err)
	}
	if len(parsed.Subtopics) == 0 {
		return nil, fmt.Errorf("study: no subtopics produced for %q", topic)
	}

	s := &Syllabus{Topic: topic, Depth: depth, CreatedAt: time.Now()}
	valid := map[string]bool{}
	for _, st := range parsed.Subtopics {
		if strings.TrimSpace(st.ID) != "" {
			valid[st.ID] = true
		}
	}

	for _, st := range parsed.Subtopics {
		if strings.TrimSpace(st.Name) == "" || strings.TrimSpace(st.ID) == "" {
			continue
		}
		// Drop dangling and self-referential edges. A prerequisite pointing at
		// a node that does not exist would make that subtopic permanently
		// unreachable, and the drill loop would run out of questions.
		prereqs := make([]string, 0, len(st.Prereqs))
		for _, p := range st.Prereqs {
			if p != st.ID && valid[p] {
				prereqs = append(prereqs, p)
			}
		}

		s.Subtopics = append(s.Subtopics, Subtopic{
			ID: st.ID, Name: st.Name, Prereqs: prereqs,
			Depth:     clampInt(st.Depth, 1, 6),
			Why:       strings.TrimSpace(st.Why),
			Mastery:   MasteryUnseen,
			Archetype: ArchetypeRecall,
		})
		if len(s.Subtopics) >= maxSubtopics {
			break
		}
	}

	if err := breakCycles(s.Subtopics); err != nil {
		d.log.Warn("syllabus contained a dependency cycle; edges were dropped",
			"topic", topic, "detail", err.Error())
	}

	d.log.Info("syllabus decomposed",
		"topic", topic, "depth", depth,
		"subtopics", len(s.Subtopics),
		"prompt_version", p.Version)

	return s, nil
}

// maxSubtopics bounds the graph. Past this a "study plan" is a reading list
// nobody finishes.
const maxSubtopics = 12

// breakCycles removes edges that would make a subtopic unreachable.
//
// A cycle means no node in it ever becomes available, and the drill loop stalls
// with questions remaining. Detecting it here costs nothing; discovering it as
// "the app stopped asking questions" costs a demo.
func breakCycles(subtopics []Subtopic) error {
	index := map[string]int{}
	for i, s := range subtopics {
		index[s.ID] = i
	}

	const (
		white = 0 // unvisited
		grey  = 1 // on the current path
		black = 2 // fully explored
	)
	colour := make([]int, len(subtopics))
	var removed []string

	var visit func(i int)
	visit = func(i int) {
		colour[i] = grey
		kept := subtopics[i].Prereqs[:0]
		for _, p := range subtopics[i].Prereqs {
			j, ok := index[p]
			if !ok {
				continue
			}
			switch colour[j] {
			case grey:
				// Back edge: this prerequisite depends on us.
				removed = append(removed, subtopics[i].ID+"->"+p)
				continue
			case white:
				visit(j)
			}
			kept = append(kept, p)
		}
		subtopics[i].Prereqs = kept
		colour[i] = black
	}

	for i := range subtopics {
		if colour[i] == white {
			visit(i)
		}
	}

	if len(removed) > 0 {
		return fmt.Errorf("dropped cyclic edges: %s", strings.Join(removed, ", "))
	}
	return nil
}

func depthNote(d Depth) string {
	switch d {
	case DepthSurvey:
		return "Survey: the shape of the topic and the vocabulary. 4 to 6 subtopics, mostly breadth."
	case DepthInterviewReady:
		return "Interview-ready: mechanism, tradeoffs, and failure modes. 8 to 12 subtopics, deep."
	default:
		return "Exam-ready: definitions, mechanisms, and worked application. 6 to 9 subtopics."
	}
}

func syllabusSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"subtopics": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"id":   {Type: genai.TypeString, Description: "Short stable id: s1, s2, s3."},
						"name": {Type: genai.TypeString, Description: "The subtopic, as a learner would name it."},
						"prereqs": {
							Type:        genai.TypeArray,
							Description: "IDs that must be understood BEFORE this one. Must reference ids in this same list. No cycles.",
							Items:       &genai.Schema{Type: genai.TypeString},
						},
						"depth": {Type: genai.TypeInteger, Description: "1 for foundational, rising with dependency depth."},
						"why":   {Type: genai.TypeString, Description: "One short sentence on why this must be understood before what follows."},
					},
					Required: []string{"id", "name", "prereqs", "depth", "why"},
				},
			},
		},
		Required: []string{"subtopics"},
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
