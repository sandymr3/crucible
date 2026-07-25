package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

// baseEnv is a minimally valid environment. Tests mutate one key at a time so
// each case names exactly the thing it is checking.
func baseEnv(t *testing.T, overrides map[string]string) {
	t.Helper()
	env := map[string]string{
		"GOOGLE_CLOUD_PROJECT":      "crucible-test",
		"GCS_BUCKET":                "crucible-test-media",
		"VERTEX_LIVE_LOCATION":      "us-central1",
		"VERTEX_REASONING_LOCATION": "global",
		"LIVE_ACTIVITY_MODE":        "manual",
		"EVAL_RED_CONFIDENCE_MIN":   "0.75",
		"SESSION_MAX_DURATION_SEC":  "720",
		"SESSION_IDLE_TIMEOUT_SEC":  "90",
	}
	for k, v := range overrides {
		env[k] = v
	}
	// Clear everything Load reads so a stray value in the developer's shell
	// cannot make a test pass that should fail.
	for _, k := range []string{
		"GOOGLE_CLOUD_PROJECT", "GCS_BUCKET", "VERTEX_LIVE_LOCATION",
		"VERTEX_REASONING_LOCATION", "PORT", "APP_ENV", "MODEL_LIVE",
		"MODEL_REASONING", "MODEL_CHEAP", "SESSION_MAX_DURATION_SEC",
		"SESSION_IDLE_TIMEOUT_SEC", "SESSION_WARN_REMAIN_SEC",
		"MAX_HINTS_PER_TURN", "MAX_CONCURRENT_SESSIONS",
		"DAILY_SESSION_CAP_PER_USER", "LIVE_ACTIVITY_MODE",
		"INJECTION_DEADLINE_MS", "EVAL_RED_CONFIDENCE_MIN", "EVAL_MIN_WORDS",
	} {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	baseEnv(t, nil)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// These three defaults were each verified against live Vertex during
	// Phase 0. A silent change to any of them breaks the demo, so pin them.
	if cfg.ModelLive != "gemini-live-2.5-flash-native-audio" {
		t.Errorf("ModelLive = %q, want the GA native-audio model", cfg.ModelLive)
	}
	if cfg.ModelReasoning != "gemini-3.6-flash" {
		t.Errorf("ModelReasoning = %q; 3.5-flash was rejected on latency variance", cfg.ModelReasoning)
	}
	if cfg.ActivityMode != ActivityManual {
		t.Errorf("ActivityMode = %q, want manual (deterministic turn boundaries)", cfg.ActivityMode)
	}
	if cfg.InjectionDeadline != 9000*time.Millisecond {
		t.Errorf("InjectionDeadline = %v, want 9s", cfg.InjectionDeadline)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		wantErr   string
	}{
		{
			// The Live model is not served from "global": the WebSocket closes
			// with a policy violation. Catching it at startup beats catching it
			// when the interviewer fails to speak.
			name:      "live location global",
			overrides: map[string]string{"VERTEX_LIVE_LOCATION": "global"},
			wantErr:   "VERTEX_LIVE_LOCATION",
		},
		{
			name:      "missing project",
			overrides: map[string]string{"GOOGLE_CLOUD_PROJECT": ""},
			wantErr:   "GOOGLE_CLOUD_PROJECT is required",
		},
		{
			name:      "missing bucket",
			overrides: map[string]string{"GCS_BUCKET": ""},
			wantErr:   "GCS_BUCKET is required",
		},
		{
			name:      "unknown activity mode",
			overrides: map[string]string{"LIVE_ACTIVITY_MODE": "vad"},
			wantErr:   "LIVE_ACTIVITY_MODE",
		},
		{
			name:      "confidence threshold out of range",
			overrides: map[string]string{"EVAL_RED_CONFIDENCE_MIN": "1.5"},
			wantErr:   "EVAL_RED_CONFIDENCE_MIN",
		},
		{
			// An idle timeout at or above the hard cap can never fire, which
			// silently disables the most important credit guardrail we have.
			name: "idle timeout not below hard cap",
			overrides: map[string]string{
				"SESSION_IDLE_TIMEOUT_SEC": "720",
				"SESSION_MAX_DURATION_SEC": "720",
			},
			wantErr: "SESSION_IDLE_TIMEOUT_SEC",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseEnv(t, tc.overrides)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() succeeded, want error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Load() error = %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadDotEnvDoesNotOverrideRealEnv(t *testing.T) {
	// A stale .env left in the working directory must never win over an
	// explicitly exported value or a Cloud Run service variable.
	t.Setenv("MODEL_REASONING", "explicitly-set")

	dir := t.TempDir()
	path := dir + "/.env"
	if err := writeFile(path, "MODEL_REASONING=from-file\nMODEL_CHEAP=from-file\n"); err != nil {
		t.Fatalf("writing temp .env: %v", err)
	}

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv() returned error: %v", err)
	}

	if got := getenv("MODEL_REASONING"); got != "explicitly-set" {
		t.Errorf("MODEL_REASONING = %q, want the pre-existing value to win", got)
	}
	if got := getenv("MODEL_CHEAP"); got != "from-file" {
		t.Errorf("MODEL_CHEAP = %q, want the file value for an unset key", got)
	}
}

func TestLoadDotEnvMissingFileIsNotAnError(t *testing.T) {
	// On Cloud Run there is no .env, and that is the expected case.
	if err := LoadDotEnv(t.TempDir() + "/does-not-exist"); err != nil {
		t.Errorf("LoadDotEnv() on a missing file returned %v, want nil", err)
	}
}

// Small helpers so the test file does not import os directly in every case.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

func getenv(k string) string { return os.Getenv(k) }
