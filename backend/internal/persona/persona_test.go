package persona

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/santh/crucible/internal/store"
)

// Rubric weights must sum to 1.0 or the same answer scores on a different
// scale depending on who is interviewing, which silently breaks the difficulty
// ladder's promote/demote thresholds.
func TestWeightsSumToOne(t *testing.T) {
	for _, p := range List() {
		if diff := math.Abs(p.Weights.Sum() - 1.0); diff > 1e-9 {
			t.Errorf("%s weights sum to %v, want 1.0", p.ID, p.Weights.Sum())
		}
	}
}

// Three interviewers who sound identical undercut the entire premise, so a
// duplicated voice is a product bug, not a cosmetic one.
func TestPersonasHaveDistinctVoices(t *testing.T) {
	seen := map[string]store.Persona{}
	for _, p := range List() {
		if p.Voice == "" {
			t.Errorf("%s has no voice configured", p.ID)
			continue
		}
		if prev, dup := seen[p.Voice]; dup {
			t.Errorf("%s and %s share the voice %q", prev, p.ID, p.Voice)
		}
		seen[p.Voice] = p.ID
	}
}

// The weightings must actually differentiate. If every persona scored an answer
// identically they would be three costumes on one interviewer.
func TestPersonasScoreTheSameAnswerDifferently(t *testing.T) {
	// Strong communication, weak technical accuracy: a PM should rate this
	// well above a Tech Lead.
	answer := store.Scores{
		TechnicalAccuracy:    3,
		CommunicationClarity: 9,
		Depth:                3,
		Structure:            8,
	}

	techLead := MustGet(store.PersonaTechLead).Weights.Score(answer)
	pm := MustGet(store.PersonaPM).Weights.Score(answer)

	if pm <= techLead {
		t.Errorf("PM scored %.2f, Tech Lead %.2f — a well-communicated but shallow answer must favour the PM", pm, techLead)
	}
}

func TestScoreStaysOnTheTenPointScale(t *testing.T) {
	perfect := store.Scores{TechnicalAccuracy: 10, CommunicationClarity: 10, Depth: 10, Structure: 10}
	worst := store.Scores{TechnicalAccuracy: 1, CommunicationClarity: 1, Depth: 1, Structure: 1}

	for _, p := range List() {
		if got := p.Weights.Score(perfect); math.Abs(got-10) > 1e-9 {
			t.Errorf("%s: perfect answer scored %v, want 10", p.ID, got)
		}
		if got := p.Weights.Score(worst); math.Abs(got-1) > 1e-9 {
			t.Errorf("%s: worst answer scored %v, want 1", p.ID, got)
		}
	}
}

// placeholderRe catches any {{TOKEN}} left unsubstituted. A missed
// substitution would send the literal text "{{BAND}}" to the interviewer,
// which it would then try to make sense of on a live call.
var placeholderRe = regexp.MustCompile(`\{\{[A-Z_]+\}\}`)

func sampleDigest(t *testing.T) map[string]any {
	t.Helper()
	raw := `{
      "candidate": {
        "seniority_estimate": "mid",
        "primary_stack": ["Python", "Kafka"],
        "gaps_vs_jd": ["no distributed training experience"],
        "claims": [{
          "id": "c1",
          "text": "Built an async proxy handling 2000 req/s",
          "artifact": "DataMesh",
          "verifiable_depth": "high",
          "probe_angles": ["How was backpressure handled?", "How was 2000 req/s measured?"]
        }]
      },
      "role": {"title": "ML Engineer", "implied_seniority": "mid", "domain_areas": ["serving"]},
      "interview_plan": [
        {"area": "Feature pipelines", "why": "JD demands it", "opening_question_seed": "Walk me through DataMesh.", "target_band": 3},
        {"area": "Consensus", "why": "resume claims Raft", "opening_question_seed": "Explain your Raft log compaction.", "target_band": 4}
      ]
    }`
	var d map[string]any
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("bad sample digest: %v", err)
	}
	return d
}

func TestBuildInstructionSubstitutesEveryPlaceholder(t *testing.T) {
	digest := sampleDigest(t)

	for _, p := range List() {
		text, version, err := p.BuildInstruction(InstructionInput{
			RoleTitle:       "ML Engineer",
			Seniority:       "mid",
			Digest:          digest,
			Band:            3,
			ConceptsProven:  []string{"queueing"},
			ConceptsShaky:   []string{"backpressure"},
			OpeningQuestion: "Walk me through DataMesh.",
		})
		if err != nil {
			t.Fatalf("%s: BuildInstruction failed: %v", p.ID, err)
		}
		if version == "" {
			t.Errorf("%s: no prompt version returned", p.ID)
		}
		if leftover := placeholderRe.FindString(text); leftover != "" {
			t.Errorf("%s: unsubstituted placeholder %s", p.ID, leftover)
		}

		// The digest must actually reach the instruction, or the interview is
		// generic no matter how good the digest was.
		for _, want := range []string{"DataMesh", "backpressure", "queueing", "ML Engineer"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: instruction is missing %q", p.ID, want)
			}
		}
	}
}

// A session with no resume must still produce a usable interviewer rather than
// an instruction full of empty sections.
func TestBuildInstructionSurvivesEmptyDigest(t *testing.T) {
	p := MustGet(store.PersonaTechLead)

	text, _, err := p.BuildInstruction(InstructionInput{Band: 3})
	if err != nil {
		t.Fatalf("BuildInstruction with no digest failed: %v", err)
	}
	if leftover := placeholderRe.FindString(text); leftover != "" {
		t.Errorf("unsubstituted placeholder %s", leftover)
	}
	if !strings.Contains(text, "No resume was provided") {
		t.Error("expected an explicit no-resume fallback in the instruction")
	}
}

// Dropped areas are the user's explicit choice on the review screen. Leaking
// one into the interviewer's plan would have it ask about something the user
// deliberately removed.
func TestDroppedPlanAreasAreExcluded(t *testing.T) {
	digest := sampleDigest(t)
	plan := digest["interview_plan"].([]any)
	plan[0].(map[string]any)["dropped"] = true

	rendered := renderPlan(digest)
	if strings.Contains(rendered, "Feature pipelines") {
		t.Error("dropped area leaked into the rendered plan")
	}
	if !strings.Contains(rendered, "Consensus") {
		t.Error("kept area missing from the rendered plan")
	}

	// The opening question must also skip the dropped area.
	if got := OpeningQuestion(digest); got != "Explain your Raft log compaction." {
		t.Errorf("OpeningQuestion = %q, want the first undropped area's seed", got)
	}
}

func TestUnknownPersonaFallsBackRatherThanFailing(t *testing.T) {
	// A bad persona value must never abort an interview mid-session.
	if got := MustGet(store.Persona("cto")); got.ID != store.PersonaTechLead {
		t.Errorf("MustGet(unknown) = %s, want a tech_lead fallback", got.ID)
	}
	if _, err := Get(store.Persona("cto")); err == nil {
		t.Error("Get(unknown) returned no error")
	}
}

func TestBandDescriptionsCoverEveryBand(t *testing.T) {
	for band := 1; band <= 5; band++ {
		if BandDescription(band) == "" {
			t.Errorf("band %d has no description", band)
		}
	}
	// Out-of-range must degrade to the mid band rather than emit an empty
	// string into the system instruction.
	if BandDescription(99) != BandDescription(3) {
		t.Error("out-of-range band should fall back to band 3")
	}
}

func TestRoleFromHandlesMissingRole(t *testing.T) {
	title, seniority := RoleFrom(map[string]any{})
	if title != "" || seniority != "" {
		t.Errorf("RoleFrom(empty) = (%q, %q), want empty strings", title, seniority)
	}
}
