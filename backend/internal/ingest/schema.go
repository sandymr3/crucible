package ingest

import "google.golang.org/genai"

// digestSchema is the controlled-generation schema for the Session Digest
// (PRD §6.1).
//
// Every non-conversational call in this project uses responseMimeType
// application/json with an explicit schema. Guaranteed parse, no markdown-fence
// stripping, and Go unmarshals straight into a typed struct.
//
// The schema is deliberately shallow. Deeply nested schemas measurably increase
// malformed-output rates and are far harder to debug at 3 a.m.
func digestSchema() *genai.Schema {
	str := func(desc string) *genai.Schema {
		return &genai.Schema{Type: genai.TypeString, Description: desc}
	}
	strArray := func(desc string) *genai.Schema {
		return &genai.Schema{
			Type:        genai.TypeArray,
			Description: desc,
			Items:       &genai.Schema{Type: genai.TypeString},
		}
	}

	claim := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"id":       str("Stable identifier, e.g. c1."),
			"text":     str("The assertion in one sentence."),
			"artifact": str("Which project, role, or resume line it came from."),
			"verifiable_depth": {
				Type:        genai.TypeString,
				Description: "How deeply this can be probed before exceeding what the resume supports.",
				Enum:        []string{"high", "medium", "low"},
			},
			"probe_angles": strArray("Two to four specific questions that test whether the candidate actually did this. Attack mechanism, measurement, or tradeoff."),
		},
		Required: []string{"id", "text", "artifact", "verifiable_depth", "probe_angles"},
	}

	candidate := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"seniority_estimate": {
				Type: genai.TypeString,
				Enum: []string{"entry", "junior", "mid", "senior"},
			},
			"primary_stack": strArray("Technologies the candidate demonstrably works in, using their own capitalisation."),
			"claims": {
				Type:        genai.TypeArray,
				Description: "Between 3 and 8 checkable assertions.",
				Items:       claim,
			},
			"gaps_vs_jd": strArray("Specific, factual requirements the JD asks for that the resume does not evidence."),
		},
		Required: []string{"seniority_estimate", "primary_stack", "claims", "gaps_vs_jd"},
	}

	role := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"title":         str("Role title from the job description."),
			"must_haves":    strArray("Hard requirements."),
			"nice_to_haves": strArray("Preferred but not required."),
			"implied_seniority": {
				Type: genai.TypeString,
				Enum: []string{"entry", "junior", "mid", "senior"},
			},
			// These become the axes of the report's radar chart, so they must
			// be distinguishable by an interview answer.
			"domain_areas": strArray("Three to six technical areas this role is actually assessed on."),
		},
		Required: []string{"title", "must_haves", "nice_to_haves", "implied_seniority", "domain_areas"},
	}

	planItem := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"area":                  str("The question area."),
			"why":                   str("One sentence tying it to both the resume and the JD. Shown to the candidate, so it must read as reasoning rather than accusation."),
			"opening_question_seed": str("A concrete first question referencing something specific from the resume by name."),
			"target_band": {
				Type:        genai.TypeInteger,
				Description: "Difficulty 1-5. Do not set every area to the same band.",
			},
		},
		Required: []string{"area", "why", "opening_question_seed", "target_band"},
	}

	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"candidate": candidate,
			"role":      role,
			"interview_plan": {
				Type:        genai.TypeArray,
				Description: "Five areas ordered as an interview flows: warm ground first, hardest last.",
				Items:       planItem,
			},
		},
		Required: []string{"candidate", "role", "interview_plan"},
	}
}
