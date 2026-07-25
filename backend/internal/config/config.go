// Package config is the single source of every tunable in Crucible.
//
// Two rules hold this project together and this file enforces the first one:
// no model ID, timeout, or cap appears anywhere outside this package. You will
// want to swap a model or loosen a cap at 2 a.m. without recompiling your
// understanding of the codebase.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ActivityMode selects how a user's speaking turn is delimited.
//
// See AD-2 in the plan. Manual is the demo default: the client sends explicit
// activity_start / activity_end signals, which makes turn boundaries
// deterministic, structurally prevents the model from hearing its own voice as
// user speech, and lets us skip sending audio frames during silence (live audio
// is the single largest cost in this system).
type ActivityMode string

const (
	// ActivityManual disables server-side VAD. The client owns the boundary.
	ActivityManual ActivityMode = "manual"
	// ActivityAuto uses the Live API's own voice activity detection, which
	// enables natural barge-in at the cost of boundary determinism.
	ActivityAuto ActivityMode = "auto"
)

// Config is the fully resolved runtime configuration. Build it once at startup
// with Load and pass it down; nothing should read the environment again.
type Config struct {
	// --- Platform ---
	ProjectID string
	GCSBucket string
	Port      string
	Env       string // "local" | "cloudrun"

	// Vertex serves these two model families from different locations, and
	// they cannot be collapsed into one. Verified against this project on
	// 25 Jul 2026 with cmd/regionprobe:
	//
	//   Live native-audio  → us-central1 / us-east4 / europe-west4.
	//                        "global" rejects it outright (close 1008).
	//   Gemini 3.x text    → "global" only. us-central1 serves nothing
	//                        newer than the 2.5 family.
	//
	// The PRD advises pinning one region for both to avoid latency asymmetry.
	// That is no longer possible. The asymmetry is real but lands only on
	// post-turn calls (evaluation, delivery, roadmap), never on the live
	// conversation, so it costs us nothing that a user can perceive.
	LiveLocation      string
	ReasoningLocation string

	// --- Models. Verified available on Vertex 25 Jul 2026. ---
	ModelLive      string // native-audio speech-to-speech, the conversation itself
	ModelReasoning string // digest, evaluation, delivery, roadmap, syllabus
	ModelCheap     string // hints, labels, cheap fan-out

	// --- Session guardrails (PRD §21.2). All server-enforced. ---
	SessionMaxDuration  time.Duration
	SessionIdleTimeout  time.Duration
	MaxHintsPerTurn     int
	MaxConcurrent       int
	DailySessionCap     int
	SessionWarnAtRemain time.Duration

	// --- Live behaviour ---
	ActivityMode ActivityMode

	// InjectionDeadline bounds how long we wait for a grade before steering the
	// conversation with deterministic data instead (AD-3).
	//
	// This is NOT a silence budget. The interviewer is already acknowledging
	// the answer during this window; the deadline governs how quickly we can
	// steer its NEXT question. So it must exceed the real evaluation latency,
	// or the fallback wins every race and the grader's followup_probe — the
	// sharpest question available, aimed exactly where the answer thinned out —
	// never reaches the conversation at all.
	//
	// Measured evaluation latency on this project is 6-7s, so 3.5s (the PRD's
	// figure, sized for a ~3s grader) made the fallback unconditional. 9s lets
	// the grader win normally while still bounding a hung call.
	InjectionDeadline time.Duration

	// --- Evaluation ---
	// EvalRedConfidenceMin is the server-side gate on red spans (AD-4). An
	// "incorrect" verdict below this confidence is rewritten to "unsupported".
	// PRD §5.1 calls a false red the most important failure to avoid, and a
	// prompt instruction alone is not a reliable calibration mechanism.
	EvalRedConfidenceMin float64
	// EvalMinWords skips grading turns too short to carry signal, which saves
	// a model call on every "yes" and "can you repeat that".
	EvalMinWords int

	// EvalThinkingBudget caps the reasoning tokens the grader may spend.
	//
	// Measured against this project: unbounded thinking costs roughly 3s of
	// extra latency per evaluation. Span-level judgement genuinely benefits
	// from reasoning, so this is a dial rather than a switch — negative means
	// "model default", 0 disables thinking entirely.
	EvalThinkingBudget int

	// RoadmapHorizonDays is how many days of study the plan covers.
	//
	// Ideally the user supplies this ("my interview is in 11 days"). Until the
	// UI asks, a week is the default: long enough to be a real plan, short
	// enough that every day still earns its place.
	RoadmapHorizonDays int
}

// Load reads configuration from the environment, applying the defaults that
// matter and failing loudly on the handful of values that have no safe default.
func Load() (*Config, error) {
	c := &Config{
		ProjectID:         env("GOOGLE_CLOUD_PROJECT", ""),
		LiveLocation:      env("VERTEX_LIVE_LOCATION", "us-central1"),
		ReasoningLocation: env("VERTEX_REASONING_LOCATION", "global"),
		GCSBucket:         env("GCS_BUCKET", ""),
		Port:              env("PORT", "8080"),
		Env:               env("APP_ENV", "local"),

		ModelLive:      env("MODEL_LIVE", "gemini-live-2.5-flash-native-audio"),
		ModelReasoning: env("MODEL_REASONING", "gemini-3.6-flash"),
		ModelCheap:     env("MODEL_CHEAP", "gemini-3.5-flash-lite"),

		SessionMaxDuration:  envSeconds("SESSION_MAX_DURATION_SEC", 720),
		SessionIdleTimeout:  envSeconds("SESSION_IDLE_TIMEOUT_SEC", 90),
		SessionWarnAtRemain: envSeconds("SESSION_WARN_REMAIN_SEC", 120),
		MaxHintsPerTurn:     envInt("MAX_HINTS_PER_TURN", 2),
		MaxConcurrent:       envInt("MAX_CONCURRENT_SESSIONS", 10),
		DailySessionCap:     envInt("DAILY_SESSION_CAP_PER_USER", 5),

		ActivityMode:      ActivityMode(env("LIVE_ACTIVITY_MODE", string(ActivityManual))),
		InjectionDeadline: envMillis("INJECTION_DEADLINE_MS", 9000),

		EvalRedConfidenceMin: envFloat("EVAL_RED_CONFIDENCE_MIN", 0.75),
		EvalMinWords:         envInt("EVAL_MIN_WORDS", 15),
		EvalThinkingBudget:   envInt("EVAL_THINKING_BUDGET", 512),
		RoadmapHorizonDays:   envInt("ROADMAP_HORIZON_DAYS", 7),
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	var problems []string

	if c.ProjectID == "" {
		problems = append(problems, "GOOGLE_CLOUD_PROJECT is required")
	}
	if c.GCSBucket == "" {
		problems = append(problems, "GCS_BUCKET is required")
	}

	// Live models are not served from "global" — the WebSocket is closed with a
	// policy violation rather than failing gracefully, so this is worth
	// catching at startup instead of at the first interview.
	if c.LiveLocation == "" || c.LiveLocation == "global" {
		problems = append(problems,
			fmt.Sprintf("VERTEX_LIVE_LOCATION must be an explicit region (got %q); Live models are not served from \"global\"", c.LiveLocation))
	}
	if c.ReasoningLocation == "" {
		problems = append(problems, "VERTEX_REASONING_LOCATION is required")
	}

	switch c.ActivityMode {
	case ActivityManual, ActivityAuto:
	default:
		problems = append(problems,
			fmt.Sprintf("LIVE_ACTIVITY_MODE must be %q or %q (got %q)", ActivityManual, ActivityAuto, c.ActivityMode))
	}

	if c.EvalRedConfidenceMin < 0 || c.EvalRedConfidenceMin > 1 {
		problems = append(problems,
			fmt.Sprintf("EVAL_RED_CONFIDENCE_MIN must be within [0,1] (got %v)", c.EvalRedConfidenceMin))
	}

	if c.SessionIdleTimeout >= c.SessionMaxDuration {
		problems = append(problems,
			"SESSION_IDLE_TIMEOUT_SEC must be less than SESSION_MAX_DURATION_SEC, otherwise the idle guard never fires")
	}

	if len(problems) > 0 {
		return errors.New("config: " + strings.Join(problems, "; "))
	}
	return nil
}

// OnCloudRun reports whether we are running inside Cloud Run, where credentials
// resolve from the attached service account rather than a key file.
func (c *Config) OnCloudRun() bool {
	return os.Getenv("K_SERVICE") != ""
}

// Redacted returns a log-safe view of the configuration. Deliberately omits
// anything credential-shaped so it can be dumped at startup without care.
func (c *Config) Redacted() map[string]any {
	return map[string]any{
		"project":            c.ProjectID,
		"live_location":      c.LiveLocation,
		"reasoning_location": c.ReasoningLocation,
		"bucket":             c.GCSBucket,
		"env":                c.Env,
		"on_cloud_run":       c.OnCloudRun(),
		"model_live":         c.ModelLive,
		"model_reasoning":    c.ModelReasoning,
		"model_cheap":        c.ModelCheap,
		"activity_mode":      string(c.ActivityMode),
		"injection_deadline": c.InjectionDeadline.String(),
		"session_max":        c.SessionMaxDuration.String(),
		"session_idle":       c.SessionIdleTimeout.String(),
		"max_concurrent":     c.MaxConcurrent,
		"daily_cap":          c.DailySessionCap,
		"eval_red_conf_min":  c.EvalRedConfidenceMin,
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(env(key, ""), 64); err == nil {
		return v
	}
	return def
}

func envSeconds(key string, def int) time.Duration {
	return time.Duration(envInt(key, def)) * time.Second
}

func envMillis(key string, def int) time.Duration {
	return time.Duration(envInt(key, def)) * time.Millisecond
}
