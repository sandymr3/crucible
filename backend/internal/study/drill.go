package study

import (
	"strings"

	"github.com/santh/crucible/internal/store"
)

// Archetype is the kind of question being asked (PRD §7.2).
//
// Study Mode cycles these deliberately rather than asking at random. The cycle
// is the pedagogy: knowing a definition, applying it, knowing where it breaks,
// and being able to teach it are four different things, and only the last one
// demonstrates understanding.
type Archetype string

const (
	// ArchetypeRecall — "state the formula and name each term".
	ArchetypeRecall Archetype = "recall"
	// ArchetypeApplication — "given this situation, what happens".
	ArchetypeApplication Archetype = "application"
	// ArchetypeEdgeCase — "what breaks if you remove this".
	ArchetypeEdgeCase Archetype = "edge_case"
	// ArchetypeTeachBack — "explain this to someone who knows linear algebra
	// but not ML".
	//
	// The highest-signal question type in the entire product. You cannot
	// teach what you have only memorised.
	ArchetypeTeachBack Archetype = "teach_back"
)

// cycle is the fixed progression through the archetypes.
var cycle = []Archetype{
	ArchetypeRecall, ArchetypeApplication, ArchetypeEdgeCase, ArchetypeTeachBack,
}

// Next returns the archetype following a. Teach-back is terminal: a subtopic
// that reached it stays there until it is passed.
func (a Archetype) Next() Archetype {
	for i, c := range cycle {
		if c == a && i+1 < len(cycle) {
			return cycle[i+1]
		}
	}
	return ArchetypeTeachBack
}

// Label renders an archetype for the UI.
func (a Archetype) Label() string {
	switch a {
	case ArchetypeRecall:
		return "Recall"
	case ArchetypeApplication:
		return "Application"
	case ArchetypeEdgeCase:
		return "Edge case"
	case ArchetypeTeachBack:
		return "Teach it back"
	}
	return string(a)
}

// Score thresholds for mastery transitions.
const (
	// PassThreshold is the score an answer must reach to advance the archetype.
	PassThreshold = 6.5
	// SolidThreshold is what a teach-back must reach to count as understanding.
	// Deliberately higher: this is the one transition that means something.
	SolidThreshold = 7.5
)

// Progress is the outcome of grading one drill answer.
type Progress struct {
	SubtopicID    string
	From          Mastery
	To            Mastery
	Passed        bool
	NextArchetype Archetype
	// Unlocked lists subtopics that became available because of this answer.
	Unlocked []string
}

// ApplyAnswer folds a graded answer into the syllabus.
//
// Pure: no I/O, no clock. The caller persists the result.
func ApplyAnswer(syl *Syllabus, subtopicID string, eval *store.Evaluation) Progress {
	idx := indexOf(syl, subtopicID)
	if idx < 0 {
		return Progress{SubtopicID: subtopicID}
	}
	st := &syl.Subtopics[idx]

	p := Progress{SubtopicID: subtopicID, From: st.Mastery, To: st.Mastery, NextArchetype: st.Archetype}
	if eval == nil {
		// An ungraded answer is not evidence in either direction.
		return p
	}

	st.Attempts++
	score := eval.TurnScore
	asked := st.Archetype

	switch {
	case asked == ArchetypeTeachBack && score >= SolidThreshold:
		// The only path to solid.
		st.TeachBackPassed = true
		st.Mastery = MasterySolid
		p.Passed = true

	case score >= PassThreshold:
		// Progress through the cycle, but a subtopic cannot become solid
		// without the teach-back, however well it recites.
		st.Archetype = asked.Next()
		if st.Mastery != MasterySolid {
			st.Mastery = MasteryShaky
		}
		p.Passed = true

	case score >= 4.0:
		// Partial. Stay on the same archetype and try it from another angle.
		if st.Mastery == MasteryUnseen || st.Mastery == MasteryAttempted {
			st.Mastery = MasteryAttempted
		}

	default:
		// A weak answer on a later archetype means the earlier ground was not
		// as solid as it looked. Step back rather than pressing on.
		if asked != ArchetypeRecall {
			st.Archetype = ArchetypeRecall
		}
		st.Mastery = MasteryAttempted
	}

	p.To = st.Mastery
	p.NextArchetype = st.Archetype
	p.Unlocked = newlyAvailable(syl, subtopicID)
	return p
}

// NextSubtopic picks what to drill next.
//
// Availability is the dependency rule: a subtopic is only offered once every
// prerequisite has been at least attempted successfully. There is no point
// asking about KV caching on turn one if attention mechanics are still unseen.
func NextSubtopic(syl *Syllabus) *Subtopic {
	var best *Subtopic

	for i := range syl.Subtopics {
		st := &syl.Subtopics[i]
		if st.Mastery == MasterySolid {
			continue // never re-drill something proven
		}
		if !Available(syl, st) {
			continue
		}
		// Prefer the shallowest available node, then the least attempted.
		// Shallow-first keeps the session moving forward through the graph
		// rather than grinding on one hard leaf.
		if best == nil ||
			st.Depth < best.Depth ||
			(st.Depth == best.Depth && st.Attempts < best.Attempts) {
			best = st
		}
	}
	return best
}

// Available reports whether a subtopic's prerequisites are met.
func Available(syl *Syllabus, st *Subtopic) bool {
	for _, id := range st.Prereqs {
		i := indexOf(syl, id)
		if i < 0 {
			continue // dangling edges were dropped at decomposition
		}
		switch syl.Subtopics[i].Mastery {
		case MasteryShaky, MasterySolid:
			// Good enough to build on. Requiring solid everywhere would make
			// the session unfinishable, since solid needs a teach-back each.
		default:
			return false
		}
	}
	return true
}

// newlyAvailable lists subtopics whose only blocker was the one just answered.
func newlyAvailable(syl *Syllabus, justAnswered string) []string {
	var out []string
	for i := range syl.Subtopics {
		st := &syl.Subtopics[i]
		if st.Mastery != MasteryUnseen || !containsString(st.Prereqs, justAnswered) {
			continue
		}
		if Available(syl, st) {
			out = append(out, st.ID)
		}
	}
	return out
}

// Complete reports whether every subtopic is solid.
func Complete(syl *Syllabus) bool {
	for _, st := range syl.Subtopics {
		if st.Mastery != MasterySolid {
			return false
		}
	}
	return len(syl.Subtopics) > 0
}

// Stats summarises the mastery map for the UI.
type Stats struct {
	Total     int `json:"total"`
	Unseen    int `json:"unseen"`
	Attempted int `json:"attempted"`
	Shaky     int `json:"shaky"`
	Solid     int `json:"solid"`
}

// Summarise counts subtopics by mastery.
func Summarise(syl *Syllabus) Stats {
	s := Stats{Total: len(syl.Subtopics)}
	for _, st := range syl.Subtopics {
		switch st.Mastery {
		case MasterySolid:
			s.Solid++
		case MasteryShaky:
			s.Shaky++
		case MasteryAttempted:
			s.Attempted++
		default:
			s.Unseen++
		}
	}
	return s
}

// MissingConcepts returns subtopics that are not solid, for the roadmap.
//
// This is how Study Mode reuses the roadmap machinery unchanged: it produces
// the same shape of gap list that the interview evaluator does.
func MissingConcepts(syl *Syllabus) []string {
	var out []string
	for _, st := range syl.Subtopics {
		if st.Mastery != MasterySolid {
			out = append(out, st.Name)
		}
	}
	return out
}

// ProvenConcepts returns the solid subtopics.
func ProvenConcepts(syl *Syllabus) []string {
	var out []string
	for _, st := range syl.Subtopics {
		if st.Mastery == MasterySolid {
			out = append(out, st.Name)
		}
	}
	return out
}

func indexOf(syl *Syllabus, id string) int {
	for i := range syl.Subtopics {
		if syl.Subtopics[i].ID == id {
			return i
		}
	}
	return -1
}

func containsString(items []string, s string) bool {
	for _, item := range items {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}
