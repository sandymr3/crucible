package vertexai

import (
	"context"

	"google.golang.org/genai"
)

// Usage is a model-agnostic view of token consumption.
//
// The SDK exposes two unrelated usage types — GenerateContentResponseUsageMetadata
// for request/response calls and UsageMetadata for the Live session — that carry
// the same information under different field names (CandidatesTokenCount vs
// ResponseTokenCount). The ledger should not care which one it came from, so
// both normalise into this.
type Usage struct {
	Model string `json:"model"`

	PromptTokens   int64 `json:"promptTokens"`
	ResponseTokens int64 `json:"responseTokens"`
	CachedTokens   int64 `json:"cachedTokens"`
	ThoughtsTokens int64 `json:"thoughtsTokens"`
	ToolTokens     int64 `json:"toolTokens"`
	TotalTokens    int64 `json:"totalTokens"`

	// Modality breakdown. Audio tokens cost substantially more than text and a
	// live session consumes them continuously in both directions, so a total
	// token count alone tells you almost nothing about where the money went
	// (PRD §21.1). These fields are what make the per-session unit economics
	// on /sessions/{id}/usage meaningful.
	PromptAudioTokens   int64 `json:"promptAudioTokens"`
	PromptTextTokens    int64 `json:"promptTextTokens"`
	ResponseAudioTokens int64 `json:"responseAudioTokens"`
	ResponseTextTokens  int64 `json:"responseTextTokens"`
}

// UsageFromGenerate normalises usage from a GenerateContent response.
func UsageFromGenerate(model string, m *genai.GenerateContentResponseUsageMetadata) *Usage {
	if m == nil {
		return nil
	}
	u := &Usage{
		Model:          model,
		PromptTokens:   int64(m.PromptTokenCount),
		ResponseTokens: int64(m.CandidatesTokenCount),
		CachedTokens:   int64(m.CachedContentTokenCount),
		ThoughtsTokens: int64(m.ThoughtsTokenCount),
		ToolTokens:     int64(m.ToolUsePromptTokenCount),
		TotalTokens:    int64(m.TotalTokenCount),
	}
	u.PromptAudioTokens, u.PromptTextTokens = splitModality(m.PromptTokensDetails)
	u.ResponseAudioTokens, u.ResponseTextTokens = splitModality(m.CandidatesTokensDetails)
	return u
}

// UsageFromLive normalises usage from a Live session server message.
func UsageFromLive(model string, m *genai.UsageMetadata) *Usage {
	if m == nil {
		return nil
	}
	u := &Usage{
		Model:          model,
		PromptTokens:   int64(m.PromptTokenCount),
		ResponseTokens: int64(m.ResponseTokenCount),
		CachedTokens:   int64(m.CachedContentTokenCount),
		ThoughtsTokens: int64(m.ThoughtsTokenCount),
		ToolTokens:     int64(m.ToolUsePromptTokenCount),
		TotalTokens:    int64(m.TotalTokenCount),
	}
	u.PromptAudioTokens, u.PromptTextTokens = splitModality(m.PromptTokensDetails)
	u.ResponseAudioTokens, u.ResponseTextTokens = splitModality(m.ResponseTokensDetails)
	return u
}

func splitModality(details []*genai.ModalityTokenCount) (audio, text int64) {
	for _, d := range details {
		if d == nil {
			continue
		}
		// Note: token-count details use MediaModality, a different enum from
		// the Modality used for ResponseModalities. They share string values
		// but are not interchangeable types.
		switch d.Modality {
		case genai.MediaModalityAudio:
			audio += int64(d.TokenCount)
		case genai.MediaModalityText:
			text += int64(d.TokenCount)
		}
	}
	return audio, text
}

// UsageRecorder receives token accounting for every call we make. Phase 2
// implements a Firestore-backed one; until then a no-op keeps call sites honest
// so the ledger is never bolted on afterwards.
type UsageRecorder interface {
	Record(ctx context.Context, u *Usage)
}

type nopRecorder struct{}

func (nopRecorder) Record(context.Context, *Usage) {}
