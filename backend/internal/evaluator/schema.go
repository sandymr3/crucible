package evaluator

import "google.golang.org/genai"

// evaluationSchema is the controlled-generation schema for a turn evaluation
// (PRD §11.2), plus the per-span confidence that drives AD-4's server-side
// downgrade rule.
func evaluationSchema() *genai.Schema {
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
	score := func(desc string) *genai.Schema {
		return &genai.Schema{Type: genai.TypeInteger, Description: desc + " Integer 1-10."}
	}

	span := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"excerpt": str("Copied CHARACTER-FOR-CHARACTER from the transcript. Not corrected, tidied, or paraphrased. Spans that do not appear verbatim are discarded."),
			"verdict": {
				Type: genai.TypeString,
				Enum: []string{"validated", "incomplete", "unsupported", "incorrect"},
				Description: "Use 'incorrect' only when confident the statement is false; " +
					"use 'unsupported' when uncertain whether it is true.",
			},
			"concept":     str("The concept this span is about, in two to four words."),
			"explanation": str("Why this verdict. One or two sentences, addressed to the candidate."),
			"correction":  str("For 'incorrect' and 'incomplete' only: what the right answer looks like."),
			"confidence": {
				Type:        genai.TypeNumber,
				Description: "Genuine certainty in this verdict, 0.0 to 1.0. Low confidence on an 'incorrect' verdict is downgraded to 'unsupported' downstream.",
			},
		},
		Required: []string{"excerpt", "verdict", "concept", "explanation", "confidence"},
	}

	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"question_intent": str("What this question was actually testing."),
			"scores": {
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"technical_accuracy":    score("Is what they said correct?"),
					"communication_clarity": score("Could a competent listener follow it?"),
					"depth":                 score("Did they go past the surface?"),
					"structure":             score("Was the answer organised?"),
				},
				Required: []string{"technical_accuracy", "communication_clarity", "depth", "structure"},
			},
			"verdict_summary":       str("Two or three sentences to the candidate, naming what they got right before what they missed."),
			"spans":                 {Type: genai.TypeArray, Items: span},
			"concepts_demonstrated": strArray("Concepts they showed real command of."),
			"concepts_missing":      strArray("Specific concepts a strong answer would have covered. Drives the study plan, so be precise."),
			"ideal_answer_outline":  strArray("Three to five bullets describing a 10/10 answer."),
			"followup_probe":        str("The single sharpest next question, aimed where this answer thinned out. Asked verbatim, under 30 words, must not reveal the answer."),
			"difficulty_recommendation": {
				Type: genai.TypeString,
				Enum: []string{"raise", "hold", "lower"},
			},
		},
		Required: []string{
			"question_intent", "scores", "verdict_summary", "spans",
			"concepts_demonstrated", "concepts_missing", "ideal_answer_outline",
			"followup_probe", "difficulty_recommendation",
		},
	}
}
