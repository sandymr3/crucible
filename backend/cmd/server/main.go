// Command server is the single Crucible backend binary: REST API, WebSocket
// relay to the Vertex Live API, and background evaluation workers, all in one
// process (AD-1).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/santh/crucible/internal/authn"
	"github.com/santh/crucible/internal/blob"
	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/delivery"
	"github.com/santh/crucible/internal/evaluator"
	"github.com/santh/crucible/internal/grading"
	"github.com/santh/crucible/internal/guardrails"
	"github.com/santh/crucible/internal/httpapi"
	"github.com/santh/crucible/internal/ingest"
	"github.com/santh/crucible/internal/live"
	"github.com/santh/crucible/internal/logging"
	"github.com/santh/crucible/internal/persona"
	"github.com/santh/crucible/internal/prompts"
	"github.com/santh/crucible/internal/replay"
	"github.com/santh/crucible/internal/roadmap"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/study"
	"github.com/santh/crucible/internal/vertexai"
	"github.com/santh/crucible/internal/worker"
)

// buildVersion is stamped at build time via -ldflags so a deployed revision can
// be identified without guessing which commit is live.
var buildVersion = "dev"

func main() {
	// Local convenience only; a missing .env is the normal Cloud Run case.
	_ = config.LoadDotEnv(".env")

	log := logging.New(os.Getenv("LOG_LEVEL"))

	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration invalid", "error", err.Error())
		os.Exit(1)
	}
	log.Info("configuration loaded", "config", cfg.Redacted(), "version", buildVersion)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Every dependency is constructed at startup so a misconfiguration fails
	// here rather than during an interview.
	st, err := store.New(ctx, cfg.ProjectID)
	if err != nil {
		log.Error("firestore unavailable", "error", err.Error())
		os.Exit(1)
	}
	defer st.Close()

	vx, err := vertexai.New(ctx, cfg, log, vertexai.NewFirestoreLedger(st, log))
	if err != nil {
		log.Error("vertex client unavailable", "error", err.Error())
		os.Exit(1)
	}

	devAllowAnon := os.Getenv("DEV_ALLOW_ANON") == "true"
	if devAllowAnon && cfg.OnCloudRun() {
		// Refuse rather than warn. An unauthenticated WebSocket in front of a
		// billing API is a credit leak that only has to be found once.
		log.Error("DEV_ALLOW_ANON is set on Cloud Run; refusing to start")
		os.Exit(1)
	}

	verifier, err := authn.NewVerifier(ctx, cfg.ProjectID, log, devAllowAnon)
	if err != nil {
		log.Error("auth verifier unavailable", "error", err.Error())
		os.Exit(1)
	}

	// Fail fast if a prompt asset is missing: they are compiled into the
	// binary, so this is a build defect and must not surface mid-interview.
	if err := prompts.Load(); err != nil {
		log.Error("prompt assets unavailable", "error", err.Error())
		os.Exit(1)
	}
	log.Info("prompts loaded", "versions", prompts.Versions())

	// Replay fixtures are optional; the product works without them.
	if err := replay.Load(); err != nil {
		log.Warn("replay fixtures unavailable", "error", err.Error())
	} else if n := len(replay.List()); n > 0 {
		log.Info("replay fixtures loaded", "count", n)
	}

	bl, err := blob.New(ctx, cfg.GCSBucket)
	if err != nil {
		log.Error("cloud storage unavailable", "error", err.Error())
		os.Exit(1)
	}
	defer bl.Close()

	ing := ingest.New(cfg, log, vx)

	pool := worker.NewPool(log, worker.DefaultConfig())
	pool.Start(ctx)
	defer pool.Stop()

	// Clear sessions abandoned by a previous, now-dead process. This instance
	// holds none of its own yet, so anything still marked live is a remnant —
	// and a remnant permanently blocks that user from starting a new session.
	if reaped, err := st.ReapStaleLiveSessions(ctx); err != nil {
		log.Warn("could not reap stale sessions", "error", err.Error())
	} else if reaped > 0 {
		log.Info("reaped stale live sessions", "count", reaped)
	}

	guard := guardrails.New(cfg, log, st)

	// Wiring the grader registers the evaluation worker handler and installs
	// the relay's turn sink. Nothing in the relay imports Firestore or the
	// evaluator; this is the seam.
	relay := live.NewRelay(cfg, log, vx)
	grader := grading.New(cfg, log, st, bl, evaluator.New(cfg, log, vx), pool, guard, relay, vx, delivery.New(cfg, log, vx), roadmap.New(cfg, log, vx), study.NewDecomposer(cfg, log, vx))

	srv := &server{
		cfg:      cfg,
		log:      log,
		vx:       vx,
		store:    st,
		guard:    guard,
		pool:     pool,
		verifier: verifier,
		relay:    relay,
		api:      httpapi.New(cfg, log, st, guard, bl, ing, grader, grader),
	}

	httpSrv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: srv.routes(),
		// No WriteTimeout: it would sever the long-lived WebSocket the live
		// interview runs over. Session duration is bounded by the guardrails
		// instead, which is where a cap belongs.
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info("http server listening", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err.Error())
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutdown signal received, draining")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err.Error())
	}
	log.Info("shutdown complete")
}

type server struct {
	cfg      *config.Config
	log      *slog.Logger
	vx       *vertexai.Client
	store    *store.Store
	guard    *guardrails.Guard
	pool     *worker.Pool
	verifier *authn.Verifier
	relay    *live.Relay
	api      *httpapi.API

	ready atomic.Bool
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	// Liveness is registered on several paths deliberately.
	//
	// Google's frontend INTERCEPTS "/healthz" on *.run.app domains: the request
	// is answered with Google's own HTML 404 and never reaches the container.
	// Verified on this deployment — "/nonexistent" returns our mux's 404 while
	// "/healthz" returns Google's error page with no trace header and no entry
	// in Cloud Run logs.
	//
	// The PRD's API contract names /healthz, and that path does work locally
	// and behind a custom domain or Firebase Hosting rewrite, so it stays
	// registered. But "/health" is the canonical one to probe against a raw
	// run.app URL, and it is what deploy.sh and any uptime check must use.
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	s.api.Register(mux, s.verifier.Middleware)

	// The live interview socket authenticates inline rather than through the
	// middleware, because it must reject BEFORE upgrading — writing an HTTP
	// error to an already-upgraded connection is not possible.
	mux.HandleFunc("GET /v1/sessions/{id}/live", s.handleLive)

	return mux
}

// handleLive authenticates, authorises, and upgrades to the interview socket.
//
// The token arrives as a query parameter because browsers cannot set headers on
// a WebSocket handshake. That exposure is mitigated by short-lived tokens and
// by never logging a full URL — note this handler logs the session ID only.
func (s *server) handleLive(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	user, err := s.verifier.VerifyRequest(r)
	if err != nil {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	ctx := logging.WithSession(logging.WithUser(r.Context(), user.UID), sessionID)
	log := logging.From(ctx, s.log)

	// The session must exist and belong to the caller. Without this check a
	// valid token for any user opens a billing connection against any session
	// ID they can guess.
	sess, err := s.store.GetSession(ctx, sessionID, user.UID)
	if err != nil {
		// Both "missing" and "someone else's" render as 404: distinguishing
		// them confirms which session IDs exist.
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrForbidden) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		log.Error("could not load session", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if sess.Status == store.StatusComplete {
		http.Error(w, "session already ended", http.StatusConflict)
		return
	}

	// Reserve a concurrency slot before upgrading, so an over-capacity service
	// returns a clean HTTP error instead of a socket that closes immediately.
	release, err := s.guard.AcquireSession(ctx, user.UID)
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, guardrails.ErrAlreadyLive) {
			status = http.StatusConflict
		}
		log.Warn("live session refused", "reason", err.Error())
		http.Error(w, err.Error(), status)
		return
	}
	defer release()

	now := time.Now()
	if err := s.store.UpdateSession(ctx, sessionID, user.UID, map[string]any{
		"status":                   string(store.StatusLive),
		"startedAt":                now,
		"liveSessionMeta.model":    s.cfg.ModelLive,
		"liveSessionMeta.location": s.cfg.LiveLocation,
	}); err != nil {
		log.Error("could not mark session live", "error", err.Error())
	}

	// Attribute every Vertex call on this connection to the session, so the
	// cost ledger can report real per-session unit economics.
	r = r.WithContext(vertexai.WithSession(ctx, sessionID))

	// A replay session serves a recording and needs no persona assembly, no
	// digest, and no Vertex connection.
	if sess.Mode == store.ModeReplay {
		log.Info("serving replay session", "fixture", sess.FixtureID)
		s.relay.Handle(w, r, live.SessionOpts{
			SessionID: sessionID,
			UID:       user.UID,
			FixtureID: sess.FixtureID,
		})
		return
	}

	// Assemble the interviewer. This is what makes the first spoken question
	// reference a real project from the candidate's own resume rather than a
	// generic opener.
	p := persona.MustGet(sess.Persona)
	roleTitle, seniority := persona.RoleFrom(sess.Digest)

	instruction, promptVersion, err := p.BuildInstruction(persona.InstructionInput{
		RoleTitle:       roleTitle,
		Seniority:       seniority,
		Digest:          sess.Digest,
		Band:            sess.DifficultyBand,
		ConceptsProven:  sess.Coverage.Proven,
		ConceptsShaky:   sess.Coverage.Shaky,
		OpeningQuestion: persona.OpeningQuestion(sess.Digest),
	})
	if err != nil {
		// Assembly failing means a prompt asset problem, which startup should
		// already have caught. Refuse rather than run an interviewer with no
		// instructions at all.
		log.Error("could not assemble persona instruction", "error", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	log.Info("interview configured",
		"persona", string(p.ID),
		"voice", p.Voice,
		"prompt_version", promptVersion,
		"band", sess.DifficultyBand,
		"has_digest", len(sess.Digest) > 0,
		"instruction_chars", len(instruction))

	// A voice override is accepted only for local A/B testing of the voice
	// roster; the persona's own voice is the default and the demo path.
	voice := p.Voice
	if override := r.URL.Query().Get("voice"); override != "" {
		voice = override
	}

	s.relay.Handle(w, r, live.SessionOpts{
		SessionID:         sessionID,
		UID:               user.UID,
		Voice:             voice,
		Temperature:       p.Temperature,
		SystemInstruction: instruction,
	})

	// The relay returns only once the session is fully torn down. Mark the
	// session no longer live so the one-live-session-per-user check does not
	// lock the user out of their next attempt.
	//
	// Uses a detached context: the request context is already cancelled by the
	// time we get here, and writing on it would silently drop this update.
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.store.UpdateSession(closeCtx, sessionID, user.UID, map[string]any{
		"status": string(store.StatusEvaluating),
	}); err != nil {
		log.Warn("could not clear live status", "error", err.Error())
		return
	}
	log.Info("session teardown complete", "status", string(store.StatusEvaluating))
}

// handleHealth is liveness: is this process alive? Unauthenticated, no
// dependencies touched, always cheap.
func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": buildVersion,
	})
}

// handleReadyz is readiness: can this process actually do its job? It issues a
// real one-token inference call, so a 200 here is proof that credentials
// resolve and Vertex answers — which is also the proof that credits are being
// drawn from Vertex rather than from a stray API key somewhere.
func (s *server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	started := time.Now()
	if err := s.vx.Ping(ctx); err != nil {
		s.log.Error("readiness probe failed", "error", err.Error())
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"reason": "vertex_unreachable",
		})
		return
	}
	s.ready.Store(true)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "ready",
		"version":            buildVersion,
		"project":            s.cfg.ProjectID,
		"live_location":      s.cfg.LiveLocation,
		"reasoning_location": s.cfg.ReasoningLocation,
		"credential_source":  s.vx.CredentialSource(),
		"models": map[string]string{
			"live":      s.cfg.ModelLive,
			"reasoning": s.cfg.ModelReasoning,
			"cheap":     s.cfg.ModelCheap,
		},
		"active_sessions":   strconv.Itoa(s.guard.ActiveCount()),
		"vertex_latency_ms": time.Since(started).Milliseconds(),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
