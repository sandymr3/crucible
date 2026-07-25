// Package ingest turns a resume PDF and a job description into the Session
// Digest that drives the entire interview.
//
// There is deliberately no PDF text-extraction library here. Gemini reads the
// PDF natively and handles two-column layouts, tables, and design-heavy resumes
// that defeat text extractors — including scanned ones, which a text extractor
// returns as empty. Adding a parser would be slower to build and worse at the
// job.
package ingest

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

// Ingester builds session digests.
type Ingester struct {
	cfg *config.Config
	log *slog.Logger
	vx  *vertexai.Client
}

// New builds the ingester.
func New(cfg *config.Config, log *slog.Logger, vx *vertexai.Client) *Ingester {
	return &Ingester{cfg: cfg, log: log, vx: vx}
}

// Result carries the digest plus the diagnostics needed to trust it.
type Result struct {
	Digest        map[string]any
	PromptVersion string
	Model         string
	DurationMs    int64
	ClaimCount    int
	PlanCount     int
}

// ErrEmptyDigest means the model returned nothing usable.
//
// Almost always a resume that is an image scan with no recoverable text, or a
// PDF that is not a resume at all. The caller surfaces a re-upload prompt
// rather than starting an interview grounded in nothing (PRD risk R14).
type ErrEmptyDigest struct{ Reason string }

func (e *ErrEmptyDigest) Error() string {
	return "ingest: digest is empty — " + e.Reason
}

// digestTimeout bounds the call. The PRD budgets 4–8 s for this and it is
// synchronous behind a "Reading your resume…" screen, so a slow call is worth
// waiting for but a hung one is not.
const digestTimeout = 90 * time.Second

// Build runs the digest call.
//
// resumeURI is a gs:// URI. Passing the GCS URI rather than inlining base64
// keeps the request small and lets Vertex fetch the bytes itself.
func (i *Ingester) Build(ctx context.Context, resumeURI, jdText string) (*Result, error) {
	if resumeURI == "" {
		return nil, fmt.Errorf("ingest: a resume is required")
	}

	p, err := prompts.Get(prompts.Digest)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(jdText) == "" {
		jdText = "(No job description was supplied. Infer a plausible target role from the resume itself, and set role.title accordingly.)"
	}
	instruction := p.Render(map[string]string{"JD_TEXT": jdText})

	ctx, cancel := context.WithTimeout(ctx, digestTimeout)
	defer cancel()

	parts := []*genai.Part{
		{FileData: &genai.FileData{FileURI: resumeURI, MIMEType: "application/pdf"}},
		{Text: instruction},
	}

	started := time.Now()
	// Low temperature: this is extraction, not creativity. A digest that
	// changes shape between runs makes every downstream bug irreproducible.
	temp := float32(0.2)
	raw, err := i.vx.GenerateStructured(ctx, i.cfg.ModelReasoning,
		[]*genai.Content{{Role: "user", Parts: parts}},
		&genai.GenerateContentConfig{
			Temperature:      &temp,
			ResponseMIMEType: "application/json",
			ResponseSchema:   digestSchema(),
		})
	if err != nil {
		return nil, fmt.Errorf("ingest: digest call failed: %w", err)
	}
	duration := time.Since(started)

	if strings.TrimSpace(raw) == "" {
		return nil, &ErrEmptyDigest{Reason: "the model returned no content"}
	}

	var digest map[string]any
	if err := json.Unmarshal([]byte(raw), &digest); err != nil {
		return nil, fmt.Errorf("ingest: digest was not valid JSON despite the schema: %w", err)
	}

	// Schema compliance is not semantic sanity. A response can satisfy every
	// required field and still be useless, so validate what actually matters.
	claims, plan, err := validate(digest)
	if err != nil {
		return nil, err
	}

	i.log.Info("digest built",
		"model", i.cfg.ModelReasoning,
		"prompt_version", p.Version,
		"duration_ms", duration.Milliseconds(),
		"claims", claims,
		"plan_areas", plan)

	return &Result{
		Digest:        digest,
		PromptVersion: p.Version,
		Model:         i.cfg.ModelReasoning,
		DurationMs:    duration.Milliseconds(),
		ClaimCount:    claims,
		PlanCount:     plan,
	}, nil
}

// maxClaims and maxPlanAreas bound what reaches the system instruction. An
// over-eager digest would otherwise inflate every live session's prompt, and
// the Live API charges for that on every turn.
const (
	maxClaims    = 8
	maxPlanAreas = 6
)

// validate checks the digest is usable and trims it to sane bounds.
func validate(digest map[string]any) (claimCount, planCount int, err error) {
	cand, ok := digest["candidate"].(map[string]any)
	if !ok {
		return 0, 0, &ErrEmptyDigest{Reason: "no candidate section"}
	}

	claims, _ := cand["claims"].([]any)
	if len(claims) == 0 {
		// The single most likely cause is a scanned resume with no recoverable
		// text. Say so, because "try again" is not actionable advice.
		return 0, 0, &ErrEmptyDigest{
			Reason: "no claims could be extracted; the resume may be an image scan or may not be a resume",
		}
	}
	if len(claims) > maxClaims {
		claims = claims[:maxClaims]
		cand["claims"] = claims
	}

	plan, _ := digest["interview_plan"].([]any)
	if len(plan) == 0 {
		return 0, 0, &ErrEmptyDigest{Reason: "no interview plan was produced"}
	}
	if len(plan) > maxPlanAreas {
		plan = plan[:maxPlanAreas]
		digest["interview_plan"] = plan
	}

	// Clamp target bands into range. Band 1 is excluded deliberately: it is
	// insulting to a candidate with a real resume and wastes the opening of a
	// short session.
	for _, raw := range plan {
		area, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if b, ok := area["target_band"].(float64); ok {
			area["target_band"] = float64(clamp(int(b), 2, 5))
		}
	}

	return len(claims), len(plan), nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
