// Package vertexai wraps the Google Gen AI SDK with the things every call in
// this project needs: correct Vertex backend selection, credential resolution
// that works identically on a laptop and on Cloud Run, bounded retries, and a
// hook for the token-usage ledger.
//
// Nothing else in the codebase constructs a genai.Client directly.
package vertexai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/auth/credentials"
	"google.golang.org/genai"

	"github.com/santh/crucible/internal/config"
)

// scopeCloudPlatform is the only OAuth scope Vertex needs.
const scopeCloudPlatform = "https://www.googleapis.com/auth/cloud-platform"

// Client is the shared Vertex handle.
//
// It holds two underlying SDK clients because Vertex does not serve both model
// families from one location: the Live native-audio model is regional
// (us-central1) and the Gemini 3.x reasoning models are global-only. A single
// client pinned to either location cannot reach the other family.
type Client struct {
	live *genai.Client // regional — Live bidi only
	text *genai.Client // global — reasoning and cheap models

	cfg   *config.Config
	log   *slog.Logger
	usage UsageRecorder

	// credentialSource records how we authenticated, purely so startup logs can
	// answer "is this billing to Vertex or to a stray API key?" without a trip
	// to the console.
	credentialSource string
}

// New constructs the Vertex client.
//
// Credential resolution is deliberately a single path for both environments:
// DetectDefault falls back to Application Default Credentials when
// GOOGLE_APPLICATION_CREDENTIALS is unset, which is exactly the Cloud Run case
// (attached service account, no key file on disk). Locally the same call picks
// up the gitignored key.json. Same binary, both environments, no branching.
func New(ctx context.Context, cfg *config.Config, log *slog.Logger, usage UsageRecorder) (*Client, error) {
	if usage == nil {
		usage = nopRecorder{}
	}

	keyFile := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	source := "application-default-credentials"
	if keyFile != "" {
		source = "service-account-key-file"
		if _, err := os.Stat(keyFile); err != nil {
			return nil, fmt.Errorf("vertexai: GOOGLE_APPLICATION_CREDENTIALS points at %q which cannot be read: %w", keyFile, err)
		}
	}

	creds, err := credentials.DetectDefault(&credentials.DetectOptions{
		CredentialsFile: keyFile, // "" => ADC
		Scopes:          []string{scopeCloudPlatform},
	})
	if err != nil {
		return nil, fmt.Errorf("vertexai: resolving credentials (%s): %w", source, err)
	}

	// Backend is set explicitly rather than inferred. GOOGLE_GENAI_USE_VERTEXAI
	// can otherwise silently route us at the Gemini API, which would bill
	// against an API key instead of the Vertex project and quietly invalidate
	// the whole "every token is billed to Vertex" design principle.
	newFor := func(location string) (*genai.Client, error) {
		return genai.NewClient(ctx, &genai.ClientConfig{
			Backend:     genai.BackendVertexAI,
			Project:     cfg.ProjectID,
			Location:    location,
			Credentials: creds,
		})
	}

	liveClient, err := newFor(cfg.LiveLocation)
	if err != nil {
		return nil, fmt.Errorf("vertexai: creating live client (%s): %w", cfg.LiveLocation, err)
	}
	textClient, err := newFor(cfg.ReasoningLocation)
	if err != nil {
		return nil, fmt.Errorf("vertexai: creating text client (%s): %w", cfg.ReasoningLocation, err)
	}

	c := &Client{
		live:             liveClient,
		text:             textClient,
		cfg:              cfg,
		log:              log,
		usage:            usage,
		credentialSource: source,
	}

	log.Info("vertex client ready",
		"backend", "vertex-ai",
		"project", cfg.ProjectID,
		"live_location", cfg.LiveLocation,
		"reasoning_location", cfg.ReasoningLocation,
		"credential_source", source,
	)
	return c, nil
}

// RawLive exposes the region-pinned SDK client. The live relay needs it for
// Live.Connect. Do not use it for text generation: the reasoning models are
// not served from this location.
func (c *Client) RawLive() *genai.Client { return c.live }

// RawText exposes the global SDK client used for every non-conversational call.
func (c *Client) RawText() *genai.Client { return c.text }

// RecordUsage books token usage from a GenerateContent response.
//
// Exposed for packages that build their own request (a PDF part, a response
// schema) and therefore call the SDK directly rather than through
// GenerateText. Without it those calls would be invisible to the ledger.
func (c *Client) RecordUsage(ctx context.Context, model string, m *genai.GenerateContentResponseUsageMetadata) {
	if u := UsageFromGenerate(model, m); u != nil {
		c.usage.Record(ctx, u)
	}
}

// RecordLiveUsage books token usage reported by a Live session.
//
// The Live path cannot go through the wrapper methods — it drives the SDK
// session object directly — so without this call the single largest cost in the
// system would never reach the ledger. Live audio dominates spend; everything
// else is rounding error. Missing it would make the usage numbers not merely
// incomplete but actively misleading.
func (c *Client) RecordLiveUsage(ctx context.Context, m *genai.UsageMetadata) {
	if u := UsageFromLive(c.cfg.ModelLive, m); u != nil {
		c.usage.Record(ctx, u)
	}
}

// Config returns the configuration this client was built with.
func (c *Client) Config() *config.Config { return c.cfg }

// CredentialSource reports how authentication resolved, for /readyz output.
func (c *Client) CredentialSource() string { return c.credentialSource }

// GenerateText runs a plain text completion. Used by /readyz and by the Phase 0
// model A/B; production paths use structured generation instead.
func (c *Client) GenerateText(ctx context.Context, model, prompt string, cfgOverride *genai.GenerateContentConfig) (string, error) {
	var out string
	err := c.withRetry(ctx, "generate_text", model, func(ctx context.Context) error {
		resp, err := c.text.Models.GenerateContent(ctx, model, genai.Text(prompt), cfgOverride)
		if err != nil {
			return err
		}
		if resp != nil {
			c.usage.Record(ctx, UsageFromGenerate(model, resp.UsageMetadata))
		}
		out = resp.Text()
		return nil
	})
	return out, err
}

// GenerateStructured runs a controlled-generation call with retries and usage
// accounting.
//
// Every structured call goes through here rather than reaching for RawText().
// Calling the SDK directly bypasses both the backoff and the ledger — a mistake
// this codebase has now made twice, once losing all Live token accounting and
// once turning a transient 429 into a failed evaluation mid-interview.
func (c *Client) GenerateStructured(ctx context.Context, model string, contents []*genai.Content, genCfg *genai.GenerateContentConfig) (string, error) {
	var out string
	err := c.withRetry(ctx, "generate_structured", model, func(ctx context.Context) error {
		resp, err := c.text.Models.GenerateContent(ctx, model, contents, genCfg)
		if err != nil {
			return err
		}
		if resp != nil {
			c.usage.Record(ctx, UsageFromGenerate(model, resp.UsageMetadata))
		}
		out = resp.Text()
		return nil
	})
	return out, err
}

// Ping issues the cheapest possible real inference call. /readyz uses it so
// that "ready" means "Vertex actually answers", not merely "the process is up".
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := c.text.Models.GenerateContent(ctx, c.cfg.ModelCheap, genai.Text("ping"),
		&genai.GenerateContentConfig{MaxOutputTokens: int32(1)})
	if err != nil {
		return fmt.Errorf("vertexai: ping %s in %s: %w", c.cfg.ModelCheap, c.cfg.ReasoningLocation, err)
	}
	return nil
}

// --- Retry ---------------------------------------------------------------

const (
	retryAttempts = 4
	// retryBase is the backoff unit for ordinary transient errors — a dropped
	// connection, a 503, a timeout.
	retryBase = 250 * time.Millisecond

	// rateLimitBase is the unit for 429s, and it is deliberately an order of
	// magnitude larger.
	//
	// A 429 means "slow down". Retrying 130 ms later is not slowing down, and
	// with full jitter on a 250 ms base the entire retry budget was being spent
	// inside one second — every attempt landing while the limit was still in
	// force. Observed in practice: three attempts, 226 ms and 129 ms apart,
	// then permanent failure on a condition that clears in seconds.
	rateLimitBase = 2 * time.Second
)

// withRetry applies bounded, jittered exponential backoff to transient
// failures. Permanent failures return immediately: a 400 will not become a 200
// on the third try, and burning six seconds discovering that during a live
// interview is worse than failing fast and degrading.
func (c *Client) withRetry(ctx context.Context, op, model string, fn func(context.Context) error) error {
	var lastErr error
	started := time.Now()

	for attempt := 1; attempt <= retryAttempts; attempt++ {
		err := fn(ctx)
		if err == nil {
			c.log.Debug("vertex call ok",
				"op", op, "model", model, "attempt", attempt,
				"duration_ms", time.Since(started).Milliseconds())
			return nil
		}
		lastErr = err

		if !isTransient(err) {
			c.log.Error("vertex call failed permanently",
				"op", op, "model", model, "attempt", attempt, "error", err.Error())
			return err
		}
		if attempt == retryAttempts {
			break
		}

		// Exponential with DECORRELATED jitter: the delay is drawn from the
		// upper half of the window rather than from zero. Full jitter can draw
		// a near-zero delay, which against a rate limit means retrying
		// immediately and wasting an attempt. Keeping the lower bound at half
		// the window preserves the anti-lockstep property while guaranteeing
		// the wait actually grows.
		base := retryBase
		if isRateLimited(err) {
			base = rateLimitBase
		}
		window := base * time.Duration(1<<(attempt-1))
		delay := window/2 + time.Duration(rand.Int63n(int64(window/2)))

		c.log.Warn("vertex call transient failure, retrying",
			"op", op, "model", model, "attempt", attempt,
			"delay_ms", delay.Milliseconds(), "error", err.Error())

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("vertexai: %s on %s failed after %d attempts: %w", op, model, retryAttempts, lastErr)
}

// isRateLimited reports whether an error is a 429, which needs a materially
// longer backoff than other transient failures.
func isRateLimited(err error) bool {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == http.StatusTooManyRequests
	}
	return strings.Contains(strings.ToLower(err.Error()), "resource exhausted")
}

// isTransient classifies errors worth retrying: rate limits, upstream
// unavailability, and timeouts.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// context.Canceled means the caller gave up (session ended, client hung up).
	// Retrying that is pure waste.
	if errors.Is(err, context.Canceled) {
		return false
	}

	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
		return false
	}

	// Fall back to string matching for transport-level failures the SDK does
	// not wrap in an APIError.
	s := strings.ToLower(err.Error())
	for _, needle := range []string{
		"connection reset", "broken pipe", "eof",
		"timeout", "temporarily unavailable", "no such host",
	} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
