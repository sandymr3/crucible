package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Collection names. Centralised so a typo is a compile error somewhere rather
// than a silently empty query.
const (
	colUsers    = "users"
	colSessions = "sessions"
	colTurns    = "turns"
	colUsage    = "usage"
	colCounters = "counters"
	colReport   = "report"
	colRoadmap  = "roadmap"

	docSummary = "summary"
	docPlan    = "plan"
)

// ErrNotFound is returned when a document does not exist. Callers map this to
// a 404 rather than a 500.
var ErrNotFound = errors.New("store: not found")

// ErrForbidden is returned when a document exists but belongs to another user.
//
// Deliberately distinct from ErrNotFound internally so the cause is legible in
// logs, but handlers must render BOTH as 404 to the client: a 403 confirms the
// session ID exists, which is an enumeration oracle.
var ErrForbidden = errors.New("store: forbidden")

// Store wraps Firestore.
type Store struct {
	fs *firestore.Client
}

// New connects to Firestore using the same credential resolution as everything
// else: a key file locally, the attached service account on Cloud Run.
func New(ctx context.Context, projectID string) (*Store, error) {
	fs, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: connecting to firestore: %w", err)
	}
	return &Store{fs: fs}, nil
}

// Close releases the underlying client.
func (s *Store) Close() error { return s.fs.Close() }

// Raw exposes the Firestore client for operations this package does not wrap.
func (s *Store) Raw() *firestore.Client { return s.fs }

// --- Users ----------------------------------------------------------------

// UpsertUser records a user on first sight and refreshes their profile after.
func (s *Store) UpsertUser(ctx context.Context, u User) error {
	if u.UID == "" {
		return errors.New("store: UpsertUser requires a uid")
	}
	doc := s.fs.Collection(colUsers).Doc(u.UID)

	// MergeAll rather than Set: a returning user must not have their
	// sessionCount reset by a login.
	_, err := doc.Set(ctx, map[string]any{
		"uid":         u.UID,
		"email":       u.Email,
		"displayName": u.DisplayName,
		"lastSeenAt":  time.Now(),
	}, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("store: upserting user: %w", err)
	}

	// createdAt is set only if absent, so it records genuine first contact.
	_, _ = doc.Update(ctx, []firestore.Update{
		{Path: "createdAt", Value: firestore.ServerTimestamp},
	}, firestore.LastUpdateTime(time.Time{}))
	return nil
}

// --- Sessions -------------------------------------------------------------

// CreateSession writes a new session and returns it with its generated ID.
func (s *Store) CreateSession(ctx context.Context, sess *Session) (*Session, error) {
	if sess.UID == "" {
		return nil, errors.New("store: CreateSession requires a uid")
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now()
	}
	if sess.Status == "" {
		sess.Status = StatusConfiguring
	}
	// Non-nil slices so JSON renders [] rather than null, which saves the
	// frontend a null check on every list.
	if sess.Coverage.Proven == nil {
		sess.Coverage.Proven = []string{}
	}
	if sess.Coverage.Shaky == nil {
		sess.Coverage.Shaky = []string{}
	}
	if sess.Coverage.Missing == nil {
		sess.Coverage.Missing = []string{}
	}
	if sess.BandHistory == nil {
		sess.BandHistory = []BandChange{}
	}

	doc := s.fs.Collection(colSessions).NewDoc()
	sess.ID = doc.ID
	if _, err := doc.Set(ctx, sess); err != nil {
		return nil, fmt.Errorf("store: creating session: %w", err)
	}

	// Best-effort: the counter is a display nicety, not a guardrail. The real
	// daily cap lives in IncrementDailySessions and is transactional.
	_, _ = s.fs.Collection(colUsers).Doc(sess.UID).
		Update(ctx, []firestore.Update{{Path: "sessionCount", Value: firestore.Increment(1)}})

	return sess, nil
}

// GetSession fetches a session, enforcing ownership.
//
// uid is required rather than optional: making the caller pass it means an
// ownership check cannot be forgotten at a call site.
func (s *Store) GetSession(ctx context.Context, sessionID, uid string) (*Session, error) {
	snap, err := s.fs.Collection(colSessions).Doc(sessionID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: getting session: %w", err)
	}

	var sess Session
	if err := snap.DataTo(&sess); err != nil {
		return nil, fmt.Errorf("store: decoding session: %w", err)
	}
	sess.ID = snap.Ref.ID

	if sess.UID != uid {
		return nil, ErrForbidden
	}
	return &sess, nil
}

// UpdateSession applies a field-level patch to a session the user owns.
func (s *Store) UpdateSession(ctx context.Context, sessionID, uid string, fields map[string]any) error {
	if _, err := s.GetSession(ctx, sessionID, uid); err != nil {
		return err
	}
	updates := make([]firestore.Update, 0, len(fields))
	for k, v := range fields {
		updates = append(updates, firestore.Update{Path: k, Value: v})
	}
	if _, err := s.fs.Collection(colSessions).Doc(sessionID).Update(ctx, updates); err != nil {
		return fmt.Errorf("store: updating session: %w", err)
	}
	return nil
}

// UpdateSessionUnchecked patches a session without an ownership check.
//
// For background workers only. They act on jobs the relay created, and the
// relay verified ownership before opening the socket — there is no request user
// to check against on this path. Never call this from an HTTP handler.
func (s *Store) UpdateSessionUnchecked(ctx context.Context, sessionID string, fields map[string]any) error {
	updates := make([]firestore.Update, 0, len(fields))
	for k, v := range fields {
		updates = append(updates, firestore.Update{Path: k, Value: v})
	}
	if _, err := s.fs.Collection(colSessions).Doc(sessionID).Update(ctx, updates); err != nil {
		return fmt.Errorf("store: updating session: %w", err)
	}
	return nil
}

// ListSessions returns a user's sessions, newest first.
func (s *Store) ListSessions(ctx context.Context, uid string, limit int) ([]*Session, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	it := s.fs.Collection(colSessions).
		Where("uid", "==", uid).
		OrderBy("createdAt", firestore.Desc).
		Limit(limit).
		Documents(ctx)
	defer it.Stop()

	var out []*Session
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: listing sessions: %w", err)
		}
		var sess Session
		if err := snap.DataTo(&sess); err != nil {
			return nil, fmt.Errorf("store: decoding session %s: %w", snap.Ref.ID, err)
		}
		sess.ID = snap.Ref.ID
		out = append(out, &sess)
	}
	return out, nil
}

// StaleLiveAfter is how long a session may claim to be live before we stop
// believing it.
//
// A session is marked live on connect and cleared on teardown. If the instance
// dies in between — a crash, a SIGKILL, a Cloud Run revision swap — the
// document is left saying "live" forever, and the one-live-session-per-user
// rule then locks that user out permanently with no way to recover.
//
// The relay enforces a hard 12-minute cap, so a session still claiming to be
// live past that plus a grace margin is provably stale. This is a
// deterministic rule rather than a heartbeat: no extra writes, nothing to keep
// alive, and it cannot itself get stuck.
//
// Kept tight on purpose. Every minute here is a minute a user who hit a crash
// is locked out of their own account, which on demo day is indistinguishable
// from the product being broken.
const StaleLiveAfter = 15 * time.Minute

// CountActiveSessions reports how many of a user's sessions are genuinely live,
// ignoring ones abandoned by a dead instance.
func (s *Store) CountActiveSessions(ctx context.Context, uid string) (int, error) {
	it := s.fs.Collection(colSessions).
		Where("uid", "==", uid).
		Where("status", "==", string(StatusLive)).
		Documents(ctx)
	defer it.Stop()

	cutoff := time.Now().Add(-StaleLiveAfter)
	n := 0
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("store: counting active sessions: %w", err)
		}

		var sess Session
		if err := snap.DataTo(&sess); err != nil {
			// Undecodable document: count it rather than ignore it. Failing
			// closed here only costs the user one retry, whereas failing open
			// on a genuinely live session doubles their burn rate.
			n++
			continue
		}

		// A session that never recorded a start time and is still "live" is
		// almost certainly a crash remnant.
		if sess.StartedAt == nil || sess.StartedAt.Before(cutoff) {
			continue
		}
		n++
	}
	return n, nil
}

// ReapStaleLiveSessions marks abandoned live sessions as complete.
//
// Runs at startup: an instance that has just come up knows that any session
// still marked live belongs to a previous, now-dead process, since it holds no
// live sessions itself.
func (s *Store) ReapStaleLiveSessions(ctx context.Context) (int, error) {
	it := s.fs.Collection(colSessions).
		Where("status", "==", string(StatusLive)).
		Limit(200).
		Documents(ctx)
	defer it.Stop()

	cutoff := time.Now().Add(-StaleLiveAfter)
	reaped := 0
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return reaped, fmt.Errorf("store: reaping stale sessions: %w", err)
		}

		var sess Session
		if err := snap.DataTo(&sess); err != nil {
			continue
		}
		if sess.StartedAt != nil && sess.StartedAt.After(cutoff) {
			continue
		}

		if _, err := snap.Ref.Update(ctx, []firestore.Update{
			{Path: "status", Value: string(StatusAbandoned)},
		}); err == nil {
			reaped++
		}
	}
	return reaped, nil
}

// --- Turns ----------------------------------------------------------------

// CreateTurn writes a turn under a session and bumps the session's turn count.
func (s *Store) CreateTurn(ctx context.Context, sessionID string, turn *Turn) (*Turn, error) {
	if turn.GradingStatus == "" {
		turn.GradingStatus = GradingPending
	}
	if turn.AskedAt.IsZero() {
		turn.AskedAt = time.Now()
	}

	doc := s.fs.Collection(colSessions).Doc(sessionID).Collection(colTurns).NewDoc()
	turn.ID = doc.ID
	if _, err := doc.Set(ctx, turn); err != nil {
		return nil, fmt.Errorf("store: creating turn: %w", err)
	}

	_, _ = s.fs.Collection(colSessions).Doc(sessionID).
		Update(ctx, []firestore.Update{{Path: "turnCount", Value: firestore.Increment(1)}})

	return turn, nil
}

// UpdateTurn applies a field-level patch to a turn.
func (s *Store) UpdateTurn(ctx context.Context, sessionID, turnID string, fields map[string]any) error {
	updates := make([]firestore.Update, 0, len(fields))
	for k, v := range fields {
		updates = append(updates, firestore.Update{Path: k, Value: v})
	}
	_, err := s.fs.Collection(colSessions).Doc(sessionID).
		Collection(colTurns).Doc(turnID).Update(ctx, updates)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		return fmt.Errorf("store: updating turn: %w", err)
	}
	return nil
}

// GetTurn fetches a single turn.
func (s *Store) GetTurn(ctx context.Context, sessionID, turnID string) (*Turn, error) {
	snap, err := s.fs.Collection(colSessions).Doc(sessionID).
		Collection(colTurns).Doc(turnID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: getting turn: %w", err)
	}
	var t Turn
	if err := snap.DataTo(&t); err != nil {
		return nil, fmt.Errorf("store: decoding turn: %w", err)
	}
	t.ID = snap.Ref.ID
	return &t, nil
}

// ListTurns returns every turn in a session, in order.
func (s *Store) ListTurns(ctx context.Context, sessionID string) ([]*Turn, error) {
	it := s.fs.Collection(colSessions).Doc(sessionID).Collection(colTurns).
		OrderBy("index", firestore.Asc).Documents(ctx)
	defer it.Stop()

	var out []*Turn
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: listing turns: %w", err)
		}
		var t Turn
		if err := snap.DataTo(&t); err != nil {
			return nil, fmt.Errorf("store: decoding turn %s: %w", snap.Ref.ID, err)
		}
		t.ID = snap.Ref.ID
		out = append(out, &t)
	}
	return out, nil
}

// --- Report and roadmap ---------------------------------------------------

// PutReport writes the finalized report summary.
func (s *Store) PutReport(ctx context.Context, sessionID string, report any) error {
	_, err := s.fs.Collection(colSessions).Doc(sessionID).
		Collection(colReport).Doc(docSummary).Set(ctx, report)
	if err != nil {
		return fmt.Errorf("store: writing report: %w", err)
	}
	return nil
}

// GetReport reads the report summary into out.
func (s *Store) GetReport(ctx context.Context, sessionID string, out any) error {
	snap, err := s.fs.Collection(colSessions).Doc(sessionID).
		Collection(colReport).Doc(docSummary).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		return fmt.Errorf("store: reading report: %w", err)
	}
	return snap.DataTo(out)
}

// PutRoadmap writes the generated roadmap.
func (s *Store) PutRoadmap(ctx context.Context, sessionID string, roadmap any) error {
	_, err := s.fs.Collection(colSessions).Doc(sessionID).
		Collection(colRoadmap).Doc(docPlan).Set(ctx, roadmap)
	if err != nil {
		return fmt.Errorf("store: writing roadmap: %w", err)
	}
	return nil
}

// GetRoadmap reads the roadmap into out.
func (s *Store) GetRoadmap(ctx context.Context, sessionID string, out any) error {
	snap, err := s.fs.Collection(colSessions).Doc(sessionID).
		Collection(colRoadmap).Doc(docPlan).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		return fmt.Errorf("store: reading roadmap: %w", err)
	}
	return snap.DataTo(out)
}

// --- Guardrail counters ---------------------------------------------------

// IncrementDailySessions atomically bumps a user's daily count and reports
// whether they are within the cap.
//
// Runs in a transaction because two browser tabs starting a session at the same
// instant would otherwise both read the old value and both be allowed through —
// which is exactly how a "cap" of five becomes a cap of nine.
func (s *Store) IncrementDailySessions(ctx context.Context, uid string, cap int) (allowed bool, count int, err error) {
	date := time.Now().UTC().Format("2006-01-02")
	ref := s.fs.Collection(colUsers).Doc(uid).Collection(colCounters).Doc(date)

	err = s.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		current := 0
		snap, err := tx.Get(ref)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		if err == nil && snap.Exists() {
			if v, ok := snap.Data()["sessions"].(int64); ok {
				current = int(v)
			}
		}

		if current >= cap {
			allowed, count = false, current
			return nil
		}

		count = current + 1
		allowed = true
		return tx.Set(ref, map[string]any{
			"date":      date,
			"sessions":  count,
			"updatedAt": time.Now(),
		})
	})
	if err != nil {
		return false, 0, fmt.Errorf("store: incrementing daily sessions: %w", err)
	}
	return allowed, count, nil
}

// --- Usage ledger ---------------------------------------------------------

// RecordUsage accumulates token consumption for a model into the day's ledger,
// and onto the session when one is named.
//
// Increment operations rather than read-modify-write: usage arrives
// concurrently from several goroutines and any read-modify-write would lose
// updates under exactly the load we care about measuring.
func (s *Store) RecordUsage(ctx context.Context, sessionID, model string, c CostEstimate) error {
	date := time.Now().UTC().Format("2006-01-02")

	// Firestore field paths cannot contain dots; model IDs do not either, but
	// normalise defensively so "gemini-3.6-flash" can never split a path.
	safeModel := strings.ReplaceAll(model, ".", "_")

	usageRef := s.fs.Collection(colUsage).Doc(date)
	if _, err := usageRef.Set(ctx, map[string]any{
		"date":      date,
		"updatedAt": time.Now(),
	}, firestore.MergeAll); err != nil {
		return fmt.Errorf("store: initialising usage doc: %w", err)
	}

	updates := []firestore.Update{
		{FieldPath: []string{"byModel", safeModel, "totalTokens"}, Value: firestore.Increment(c.TotalTokens)},
		{FieldPath: []string{"byModel", safeModel, "promptAudioTokens"}, Value: firestore.Increment(c.PromptAudioTokens)},
		{FieldPath: []string{"byModel", safeModel, "responseAudioTokens"}, Value: firestore.Increment(c.ResponseAudioTokens)},
		{FieldPath: []string{"byModel", safeModel, "promptTextTokens"}, Value: firestore.Increment(c.PromptTextTokens)},
		{FieldPath: []string{"byModel", safeModel, "responseTextTokens"}, Value: firestore.Increment(c.ResponseTextTokens)},
		{FieldPath: []string{"byModel", safeModel, "calls"}, Value: firestore.Increment(1)},
		{Path: "totalCalls", Value: firestore.Increment(1)},
	}
	if _, err := usageRef.Update(ctx, updates); err != nil {
		return fmt.Errorf("store: recording usage: %w", err)
	}

	if sessionID == "" {
		return nil
	}
	_, err := s.fs.Collection(colSessions).Doc(sessionID).Update(ctx, []firestore.Update{
		{FieldPath: []string{"costEstimate", "totalTokens"}, Value: firestore.Increment(c.TotalTokens)},
		{FieldPath: []string{"costEstimate", "promptAudioTokens"}, Value: firestore.Increment(c.PromptAudioTokens)},
		{FieldPath: []string{"costEstimate", "responseAudioTokens"}, Value: firestore.Increment(c.ResponseAudioTokens)},
		{FieldPath: []string{"costEstimate", "promptTextTokens"}, Value: firestore.Increment(c.PromptTextTokens)},
		{FieldPath: []string{"costEstimate", "responseTextTokens"}, Value: firestore.Increment(c.ResponseTextTokens)},
		{FieldPath: []string{"costEstimate", "calls"}, Value: firestore.Increment(1)},
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("store: recording session cost: %w", err)
	}
	return nil
}

// GetDailyUsage reads one day's ledger.
func (s *Store) GetDailyUsage(ctx context.Context, date string) (*DailyUsage, error) {
	snap, err := s.fs.Collection(colUsage).Doc(date).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: reading usage: %w", err)
	}
	var u DailyUsage
	if err := snap.DataTo(&u); err != nil {
		return nil, fmt.Errorf("store: decoding usage: %w", err)
	}
	return &u, nil
}

// fields splits on whitespace. Local helper so types.go does not import strings.
func fields(s string) []string { return strings.Fields(s) }
