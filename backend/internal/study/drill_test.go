package study

import (
	"testing"

	"github.com/santh/crucible/internal/store"
)

func evalScore(s float64) *store.Evaluation {
	return &store.Evaluation{TurnScore: s}
}

// chain builds a -> b -> c, each depending on the previous.
func chain() *Syllabus {
	return &Syllabus{
		Topic: "transformer attention",
		Subtopics: []Subtopic{
			{ID: "s1", Name: "Dot-product similarity", Depth: 1, Mastery: MasteryUnseen, Archetype: ArchetypeRecall},
			{ID: "s2", Name: "Scaled dot-product attention", Prereqs: []string{"s1"}, Depth: 2, Mastery: MasteryUnseen, Archetype: ArchetypeRecall},
			{ID: "s3", Name: "KV caching at inference", Prereqs: []string{"s2"}, Depth: 3, Mastery: MasteryUnseen, Archetype: ArchetypeRecall},
		},
	}
}

func find(t *testing.T, syl *Syllabus, id string) *Subtopic {
	t.Helper()
	i := indexOf(syl, id)
	if i < 0 {
		t.Fatalf("subtopic %s not found", id)
	}
	return &syl.Subtopics[i]
}

// THE rule that makes the mastery map mean something (PRD §7.3): a subtopic
// reaches solid only after a correct teach-back, never through recall alone.
func TestSolidRequiresTeachBack(t *testing.T) {
	syl := chain()

	// Ace recall, application, and edge case in turn.
	for _, want := range []Archetype{ArchetypeApplication, ArchetypeEdgeCase, ArchetypeTeachBack} {
		ApplyAnswer(syl, "s1", evalScore(10.0))
		st := find(t, syl, "s1")
		if st.Archetype != want {
			t.Fatalf("archetype = %s, want %s", st.Archetype, want)
		}
		if st.Mastery == MasterySolid {
			t.Fatalf("reached SOLID at archetype %s without a teach-back", st.Archetype)
		}
	}

	// Only the teach-back unlocks it.
	ApplyAnswer(syl, "s1", evalScore(9.0))
	if got := find(t, syl, "s1").Mastery; got != MasterySolid {
		t.Errorf("mastery = %s after a strong teach-back, want solid", got)
	}
}

// A teach-back that is merely adequate is not understanding.
func TestWeakTeachBackDoesNotReachSolid(t *testing.T) {
	syl := chain()
	find(t, syl, "s1").Archetype = ArchetypeTeachBack

	// Above PassThreshold but below SolidThreshold.
	ApplyAnswer(syl, "s1", evalScore(7.0))

	if got := find(t, syl, "s1").Mastery; got == MasterySolid {
		t.Errorf("mastery = solid on a 7.0 teach-back; the bar for solid must be higher")
	}
}

// A bad answer on a later archetype means the earlier ground was weaker than it
// looked, so step back rather than pressing on.
func TestPoorAnswerStepsBackToRecall(t *testing.T) {
	syl := chain()
	st := find(t, syl, "s1")
	st.Archetype = ArchetypeEdgeCase
	st.Mastery = MasteryShaky

	ApplyAnswer(syl, "s1", evalScore(2.0))

	st = find(t, syl, "s1")
	if st.Archetype != ArchetypeRecall {
		t.Errorf("archetype = %s after a poor answer, want a step back to recall", st.Archetype)
	}
	if st.Mastery != MasteryAttempted {
		t.Errorf("mastery = %s, want attempted", st.Mastery)
	}
}

func TestPartialAnswerHoldsTheSameArchetype(t *testing.T) {
	syl := chain()

	ApplyAnswer(syl, "s1", evalScore(5.0))

	st := find(t, syl, "s1")
	if st.Archetype != ArchetypeRecall {
		t.Errorf("archetype advanced to %s on a partial answer", st.Archetype)
	}
	if st.Mastery != MasteryAttempted {
		t.Errorf("mastery = %s, want attempted", st.Mastery)
	}
}

// Dependency ordering: there is no point asking about KV caching before
// attention mechanics.
func TestSubtopicsAreGatedByPrerequisites(t *testing.T) {
	syl := chain()

	if Available(syl, find(t, syl, "s2")) {
		t.Error("s2 is available while its prerequisite s1 is unseen")
	}
	if got := NextSubtopic(syl); got == nil || got.ID != "s1" {
		t.Fatalf("NextSubtopic = %v, want the foundational s1", got)
	}

	// Bring s1 to shaky, which is enough to build on.
	ApplyAnswer(syl, "s1", evalScore(8.0))

	if !Available(syl, find(t, syl, "s2")) {
		t.Error("s2 still blocked after its prerequisite reached shaky")
	}
	if Available(syl, find(t, syl, "s3")) {
		t.Error("s3 became available while s2 is still unseen")
	}
}

func TestAnsweringUnlocksDependents(t *testing.T) {
	syl := chain()

	p := ApplyAnswer(syl, "s1", evalScore(8.0))

	if len(p.Unlocked) != 1 || p.Unlocked[0] != "s2" {
		t.Errorf("Unlocked = %v, want [s2]", p.Unlocked)
	}
}

// Requiring solid prerequisites everywhere would make a session unfinishable,
// since every solid needs its own teach-back.
func TestShakyPrerequisiteIsEnoughToProceed(t *testing.T) {
	syl := chain()
	find(t, syl, "s1").Mastery = MasteryShaky

	if !Available(syl, find(t, syl, "s2")) {
		t.Error("a shaky prerequisite should be enough to proceed")
	}
}

func TestSolidSubtopicsAreNeverRedrilled(t *testing.T) {
	syl := chain()
	for i := range syl.Subtopics {
		syl.Subtopics[i].Mastery = MasterySolid
	}

	if got := NextSubtopic(syl); got != nil {
		t.Errorf("NextSubtopic = %s on a fully solid syllabus, want nil", got.ID)
	}
	if !Complete(syl) {
		t.Error("Complete() = false with every subtopic solid")
	}
}

func TestNextSubtopicPrefersShallowest(t *testing.T) {
	syl := &Syllabus{Subtopics: []Subtopic{
		{ID: "deep", Depth: 4, Mastery: MasteryUnseen, Archetype: ArchetypeRecall},
		{ID: "shallow", Depth: 1, Mastery: MasteryUnseen, Archetype: ArchetypeRecall},
	}}

	if got := NextSubtopic(syl); got == nil || got.ID != "shallow" {
		t.Errorf("NextSubtopic = %v, want the shallowest available", got)
	}
}

func TestUngradedAnswerChangesNothing(t *testing.T) {
	syl := chain()
	before := *find(t, syl, "s1")

	ApplyAnswer(syl, "s1", nil)

	after := find(t, syl, "s1")
	if after.Mastery != before.Mastery || after.Attempts != before.Attempts {
		t.Error("an ungraded answer mutated mastery or attempts")
	}
}

func TestArchetypeCycleIsFixedAndTerminatesAtTeachBack(t *testing.T) {
	got := ArchetypeRecall
	want := []Archetype{ArchetypeApplication, ArchetypeEdgeCase, ArchetypeTeachBack, ArchetypeTeachBack}
	for i, w := range want {
		got = got.Next()
		if got != w {
			t.Errorf("step %d: archetype = %s, want %s", i, got, w)
		}
	}
}

func TestSummariseAndConceptLists(t *testing.T) {
	syl := chain()
	syl.Subtopics[0].Mastery = MasterySolid
	syl.Subtopics[1].Mastery = MasteryShaky

	s := Summarise(syl)
	if s.Total != 3 || s.Solid != 1 || s.Shaky != 1 || s.Unseen != 1 {
		t.Errorf("Stats = %+v", s)
	}

	// The roadmap consumes these, so their shape must match what the interview
	// evaluator produces.
	if proven := ProvenConcepts(syl); len(proven) != 1 || proven[0] != "Dot-product similarity" {
		t.Errorf("ProvenConcepts = %v", proven)
	}
	if missing := MissingConcepts(syl); len(missing) != 2 {
		t.Errorf("MissingConcepts = %v, want the two non-solid subtopics", missing)
	}
}

// A cycle would make every node in it permanently unreachable, and the drill
// loop would stall with questions remaining.
func TestCyclesAreBrokenAtDecomposition(t *testing.T) {
	subtopics := []Subtopic{
		{ID: "a", Prereqs: []string{"c"}},
		{ID: "b", Prereqs: []string{"a"}},
		{ID: "c", Prereqs: []string{"b"}},
	}

	if err := breakCycles(subtopics); err == nil {
		t.Error("breakCycles reported no error on a genuine cycle")
	}

	syl := &Syllabus{Subtopics: subtopics}
	for i := range syl.Subtopics {
		syl.Subtopics[i].Mastery = MasteryUnseen
		syl.Subtopics[i].Archetype = ArchetypeRecall
	}
	if NextSubtopic(syl) == nil {
		t.Error("no subtopic is reachable after cycle breaking; the drill loop would stall")
	}
}

func TestSelfReferentialPrereqDoesNotDeadlock(t *testing.T) {
	subtopics := []Subtopic{{ID: "a", Prereqs: []string{"a"}}}
	_ = breakCycles(subtopics)

	syl := &Syllabus{Subtopics: []Subtopic{{ID: "a", Mastery: MasteryUnseen, Archetype: ArchetypeRecall}}}
	if NextSubtopic(syl) == nil {
		t.Error("a self-referential prerequisite made the only subtopic unreachable")
	}
}
