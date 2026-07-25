// Package persona holds the three interviewers.
//
// A persona is a bundle of: a system instruction, rubric weights, a question
// distribution, a distinct Live voice, and an interruption policy. The rubric
// weights matter as much as the prompt — they are what make the same answer
// score differently for a Tech Lead than for a Product Manager, which is the
// difference between three characters and three costumes.
package persona

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santh/crucible/internal/prompts"
	"github.com/santh/crucible/internal/store"
)

// Weights are the persona's rubric weighting. They sum to 1.0 so a turn score
// stays on the same 0–10 scale regardless of who is interviewing.
type Weights struct {
	TechnicalAccuracy    float64
	CommunicationClarity float64
	Depth                float64
	Structure            float64
}

// Sum returns the total weight, used to assert the invariant in tests.
func (w Weights) Sum() float64 {
	return w.TechnicalAccuracy + w.CommunicationClarity + w.Depth + w.Structure
}

// Score applies the weighting to a set of dimension scores, producing the
// persona-weighted turn score on a 0–10 scale.
func (w Weights) Score(s store.Scores) float64 {
	return w.TechnicalAccuracy*float64(s.TechnicalAccuracy) +
		w.CommunicationClarity*float64(s.CommunicationClarity) +
		w.Depth*float64(s.Depth) +
		w.Structure*float64(s.Structure)
}

// Config is one interviewer.
type Config struct {
	ID   store.Persona
	Name string
	// Tagline is the one line shown on the persona card.
	Tagline string
	// Punishes is shown on the card too, and is the reason a user picks the
	// one that scares them rather than the one that sounds pleasant.
	Punishes string

	// Voice is a Gemini Live prebuilt voice name. Three interviewers who sound
	// identical undercut the entire multi-agent premise, so these must be
	// chosen by ear — see cmd/voicesample.
	Voice string

	// Temperature shapes register: lower is clipped and precise, higher is
	// conversational.
	Temperature float32

	Weights Weights
	Prompt  prompts.Name
}

// All personas, keyed by ID.
var all = map[store.Persona]Config{
	store.PersonaTechLead: {
		ID:       store.PersonaTechLead,
		Name:     "The Tech Lead",
		Tagline:  "Wants the mechanism, not the vocabulary.",
		Punishes: "Hand-waving, buzzwords without mechanism, \"it just works\".",
		// Charon is the lowest-pitched of the set: brisk, minimal warmth.
		Voice:       "Charon",
		Temperature: 0.6,
		Weights: Weights{
			TechnicalAccuracy:    0.50,
			Depth:                0.25,
			Structure:            0.15,
			CommunicationClarity: 0.10,
		},
		Prompt: prompts.PersonaTechLead,
	},
	store.PersonaArchitect: {
		ID:       store.PersonaArchitect,
		Name:     "The System Architect",
		Tagline:  "Will let you build the wrong design, then probe the crack.",
		Punishes: "Premature detail, unexamined defaults, no mention of tradeoffs.",
		// Orus: measured, even pacing.
		Voice:       "Orus",
		Temperature: 0.7,
		Weights: Weights{
			Structure:            0.35,
			TechnicalAccuracy:    0.30,
			Depth:                0.20,
			CommunicationClarity: 0.15,
		},
		Prompt: prompts.PersonaArchitect,
	},
	store.PersonaPM: {
		ID:       store.PersonaPM,
		Name:     "The Product Manager",
		Tagline:  "Warm, curious, and will ask who the user was.",
		Punishes: "Jargon without translation, no user in the story, blaming teammates.",
		// Aoede: warmer, slightly faster, conversational.
		Voice:       "Aoede",
		Temperature: 0.85,
		Weights: Weights{
			CommunicationClarity: 0.45,
			Structure:            0.25,
			TechnicalAccuracy:    0.20,
			Depth:                0.10,
		},
		Prompt: prompts.PersonaPM,
	},
}

// Get returns a persona configuration.
func Get(id store.Persona) (Config, error) {
	c, ok := all[id]
	if !ok {
		return Config{}, fmt.Errorf("persona: unknown persona %q", id)
	}
	return c, nil
}

// MustGet returns a persona, falling back to the Tech Lead.
//
// A missing persona must never abort an interview mid-session — the fallback is
// the demo-path default, which is the least surprising thing to land on.
func MustGet(id store.Persona) Config {
	if c, err := Get(id); err == nil {
		return c
	}
	return all[store.PersonaTechLead]
}

// List returns every persona, for the selection screen.
func List() []Config {
	return []Config{
		all[store.PersonaTechLead],
		all[store.PersonaArchitect],
		all[store.PersonaPM],
	}
}

// bandDescriptions describe the five difficulty bands (PRD §10.1). Injected
// into the system instruction so the model knows what register to pitch at.
var bandDescriptions = map[int]string{
	1: "Orientation — definitional questions. Accept textbook answers.",
	2: "Application — \"given this situation, what would you do\". Want a reason attached.",
	3: "Mechanism — \"how does that work under the hood\". Want correct internals.",
	4: "Tradeoff — make them argue against their own choice. Want named alternatives and costs.",
	5: "Adversarial — deliberately underspecified. Want them to surface assumptions and push back.",
}

// BandDescription returns the register for a difficulty band.
func BandDescription(band int) string {
	if d, ok := bandDescriptions[band]; ok {
		return d
	}
	return bandDescriptions[3]
}

// InstructionInput is everything needed to assemble a system instruction.
type InstructionInput struct {
	RoleTitle       string
	Seniority       string
	Digest          map[string]any
	Band            int
	ConceptsProven  []string
	ConceptsShaky   []string
	OpeningQuestion string
}

// BuildInstruction assembles the persona's system instruction (PRD §8.4).
//
// Structure: identity → what you probe → what you never do → the candidate's
// digest → the plan → current state. Only the last block changes mid-session,
// and it changes by injection rather than by reconnecting.
func (c Config) BuildInstruction(in InstructionInput) (text, promptVersion string, err error) {
	p, err := prompts.Get(c.Prompt)
	if err != nil {
		return "", "", err
	}

	roleTitle := in.RoleTitle
	if roleTitle == "" {
		roleTitle = "technical"
	}
	seniority := in.Seniority
	if seniority == "" {
		seniority = "mid"
	}

	opening := in.OpeningQuestion
	if opening == "" {
		// With no digest we cannot ground the question in a real project, so
		// ask the candidate to supply the material instead of inventing one.
		opening = "Ask the candidate to walk you through the project they are proudest of, and why they made its central technical decision."
	}

	return p.Render(map[string]string{
		"ROLE_TITLE":           roleTitle,
		"SENIORITY":            seniority,
		"CANDIDATE_BACKGROUND": renderCandidate(in.Digest),
		"PROBE_ANGLES":         renderProbeAngles(in.Digest),
		"INTERVIEW_PLAN":       renderPlan(in.Digest),
		"BAND":                 fmt.Sprint(in.Band),
		"BAND_DESCRIPTION":     BandDescription(in.Band),
		"CONCEPTS_PROVEN":      orNone(in.ConceptsProven),
		"CONCEPTS_SHAKY":       orNone(in.ConceptsShaky),
		"OPENING_QUESTION":     opening,
	}), p.Version, nil
}

// --- Digest rendering -----------------------------------------------------
//
// The digest arrives as map[string]any because it is stored untyped, so these
// helpers navigate defensively. A malformed digest must degrade into a thinner
// interview, never into a failed one.

func renderCandidate(digest map[string]any) string {
	cand, ok := digest["candidate"].(map[string]any)
	if !ok {
		return "No resume was provided. Ask the candidate to describe their background themselves."
	}

	var b strings.Builder
	if v, ok := cand["seniority_estimate"].(string); ok && v != "" {
		fmt.Fprintf(&b, "Estimated seniority: %s\n", v)
	}
	if stack := stringSlice(cand["primary_stack"]); len(stack) > 0 {
		fmt.Fprintf(&b, "Primary stack: %s\n", strings.Join(stack, ", "))
	}
	if gaps := stringSlice(cand["gaps_vs_jd"]); len(gaps) > 0 {
		fmt.Fprintf(&b, "Gaps versus the job description: %s\n", strings.Join(gaps, "; "))
	}

	if claims, ok := cand["claims"].([]any); ok && len(claims) > 0 {
		b.WriteString("\nClaims made on the resume:\n")
		for _, raw := range claims {
			cl, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			text, _ := cl["text"].(string)
			artifact, _ := cl["artifact"].(string)
			depth, _ := cl["verifiable_depth"].(string)
			if text == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s", text)
			if artifact != "" {
				fmt.Fprintf(&b, " (%s)", artifact)
			}
			if depth != "" {
				fmt.Fprintf(&b, " [depth: %s]", depth)
			}
			b.WriteString("\n")
		}
	}

	if b.Len() == 0 {
		return "No resume was provided. Ask the candidate to describe their background themselves."
	}
	return b.String()
}

func renderProbeAngles(digest map[string]any) string {
	cand, ok := digest["candidate"].(map[string]any)
	if !ok {
		return "None precomputed. Probe whatever the candidate raises."
	}
	claims, ok := cand["claims"].([]any)
	if !ok || len(claims) == 0 {
		return "None precomputed. Probe whatever the candidate raises."
	}

	var b strings.Builder
	for _, raw := range claims {
		cl, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		angles := stringSlice(cl["probe_angles"])
		if len(angles) == 0 {
			continue
		}
		artifact, _ := cl["artifact"].(string)
		if artifact == "" {
			artifact = "claim"
		}
		fmt.Fprintf(&b, "On %s:\n", artifact)
		for _, a := range angles {
			fmt.Fprintf(&b, "  - %s\n", a)
		}
	}
	if b.Len() == 0 {
		return "None precomputed. Probe whatever the candidate raises."
	}
	return b.String()
}

func renderPlan(digest map[string]any) string {
	plan, ok := digest["interview_plan"].([]any)
	if !ok || len(plan) == 0 {
		return "No fixed plan. Follow the candidate's strongest claims."
	}

	var b strings.Builder
	n := 0
	for _, raw := range plan {
		area, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		// A dropped area is one the user unchecked on the review screen. It
		// stays in the document so the choice is auditable, but it must not
		// reach the interviewer.
		if dropped, _ := area["dropped"].(bool); dropped {
			continue
		}
		name, _ := area["area"].(string)
		if name == "" {
			continue
		}
		n++
		fmt.Fprintf(&b, "%d. %s", n, name)
		if why, _ := area["why"].(string); why != "" {
			fmt.Fprintf(&b, " — %s", why)
		}
		if band, ok := numeric(area["target_band"]); ok {
			fmt.Fprintf(&b, " (target band %d)", int(band))
		}
		b.WriteString("\n")
	}
	if n == 0 {
		return "No fixed plan. Follow the candidate's strongest claims."
	}
	return b.String()
}

// OpeningQuestion returns the seed question for the first undropped plan area.
func OpeningQuestion(digest map[string]any) string {
	plan, ok := digest["interview_plan"].([]any)
	if !ok {
		return ""
	}
	for _, raw := range plan {
		area, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if dropped, _ := area["dropped"].(bool); dropped {
			continue
		}
		if seed, _ := area["opening_question_seed"].(string); seed != "" {
			return seed
		}
	}
	return ""
}

// RoleFrom extracts the role title and implied seniority from a digest.
func RoleFrom(digest map[string]any) (title, seniority string) {
	role, ok := digest["role"].(map[string]any)
	if !ok {
		return "", ""
	}
	title, _ = role["title"].(string)
	seniority, _ = role["implied_seniority"].(string)
	return title, seniority
}

// --- Small helpers --------------------------------------------------------

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// numeric normalises the several shapes a JSON number can take after passing
// through encoding/json and Firestore.
func numeric(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func orNone(items []string) string {
	if len(items) == 0 {
		return "none yet"
	}
	return strings.Join(items, ", ")
}
