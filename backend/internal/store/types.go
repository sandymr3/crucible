// Package store is the persistence layer.
//
// Firestore is the source of truth for everything that matters (AD-5). The
// in-memory session held by the relay is a cache, never the authority. That is
// what makes WebSocket reconnects, instance restarts, and idempotent
// finalization work without three separate mechanisms.
package store

import "time"

// --- Enumerations ---------------------------------------------------------

// Mode distinguishes the two products sharing one engine.
type Mode string

const (
	ModeInterview Mode = "interview"
	ModeStudy     Mode = "study"
	// ModeReplay serves a recorded session over the identical WebSocket
	// protocol (AD-7). It is the demo's insurance against venue wifi.
	ModeReplay Mode = "replay"
)

// SessionStatus tracks a session through its lifecycle.
type SessionStatus string

const (
	StatusConfiguring SessionStatus = "configuring"
	StatusLive        SessionStatus = "live"
	StatusEvaluating  SessionStatus = "evaluating"
	StatusComplete    SessionStatus = "complete"
	StatusAbandoned   SessionStatus = "abandoned"
)

// Persona identifies which interviewer is running the session.
type Persona string

const (
	PersonaTechLead  Persona = "tech_lead"
	PersonaArchitect Persona = "architect"
	PersonaPM        Persona = "pm"
)

// Valid reports whether p is a known persona. Callers must reject unknown
// values rather than silently defaulting, or a typo in a request body becomes
// an interview with the wrong rubric.
func (p Persona) Valid() bool {
	switch p {
	case PersonaTechLead, PersonaArchitect, PersonaPM:
		return true
	}
	return false
}

// InputMode records how an answer was given. Voice and text share the entire
// downstream path (AD-6); this exists for reporting, not for branching.
type InputMode string

const (
	InputVoice InputMode = "voice"
	InputText  InputMode = "text"
)

// GradingStatus tracks a turn's evaluation.
type GradingStatus string

const (
	GradingPending  GradingStatus = "pending"
	GradingComplete GradingStatus = "complete"
	GradingFailed   GradingStatus = "failed"
	// GradingSkipped marks turns too short to carry signal. Grading "yes" or
	// "can you repeat that" costs a model call and produces noise.
	GradingSkipped GradingStatus = "skipped"
)

// Verdict is the span-level taxonomy from PRD §12.1.
//
// Four values rather than two. Most of what looks like a wrong claim in an
// interview answer is not falsehood but unbacked assertion, and conflating them
// produces the demo-killing failure of flagging a true statement red.
type Verdict string

const (
	VerdictValidated   Verdict = "validated"   // green ✓ correct and substantive
	VerdictIncomplete  Verdict = "incomplete"  // amber ~ directionally right, thin
	VerdictUnsupported Verdict = "unsupported" // blue  ? asserted without basis
	VerdictIncorrect   Verdict = "incorrect"   // red   ! confidently false
)

// --- Documents ------------------------------------------------------------

// User mirrors an authenticated Firebase user.
type User struct {
	UID          string    `firestore:"uid" json:"uid"`
	Email        string    `firestore:"email,omitempty" json:"email,omitempty"`
	DisplayName  string    `firestore:"displayName,omitempty" json:"displayName,omitempty"`
	CreatedAt    time.Time `firestore:"createdAt" json:"createdAt"`
	SessionCount int       `firestore:"sessionCount" json:"sessionCount"`
}

// Coverage is the three concept sets the adaptive engine maintains.
//
// Without this an "adaptive" interview asks the same thing three times in
// different words, which users notice immediately.
type Coverage struct {
	// Proven concepts are never re-tested.
	Proven []string `firestore:"proven" json:"proven"`
	// Shaky concepts are re-approached from a different angle, never repeated
	// verbatim.
	Shaky []string `firestore:"shaky" json:"shaky"`
	// Missing concepts were named in the plan or the JD but never successfully
	// addressed. This set is the input to the roadmap.
	Missing []string `firestore:"missing" json:"missing"`
}

// BandChange is one entry in the difficulty history.
//
// Denormalised onto the session so the report's sparkline needs no aggregation
// query.
type BandChange struct {
	TurnIndex int       `firestore:"turnIndex" json:"turnIndex"`
	Band      int       `firestore:"band" json:"band"`
	Reason    string    `firestore:"reason" json:"reason"`
	At        time.Time `firestore:"at" json:"at"`
}

// LiveMeta records what the live connection actually used, so a session can be
// diagnosed after the fact without guessing which model or voice was live.
type LiveMeta struct {
	Model       string `firestore:"model,omitempty" json:"model,omitempty"`
	Voice       string `firestore:"voice,omitempty" json:"voice,omitempty"`
	Location    string `firestore:"location,omitempty" json:"location,omitempty"`
	ResumeCount int    `firestore:"resumeCount" json:"resumeCount"`
}

// CostEstimate accumulates token usage for one session.
//
// Audio and text are tracked separately because audio tokens cost
// substantially more and a live session consumes them continuously in both
// directions — a single total would hide where the money actually goes.
type CostEstimate struct {
	TotalTokens         int64 `firestore:"totalTokens" json:"totalTokens"`
	PromptAudioTokens   int64 `firestore:"promptAudioTokens" json:"promptAudioTokens"`
	ResponseAudioTokens int64 `firestore:"responseAudioTokens" json:"responseAudioTokens"`
	PromptTextTokens    int64 `firestore:"promptTextTokens" json:"promptTextTokens"`
	ResponseTextTokens  int64 `firestore:"responseTextTokens" json:"responseTextTokens"`
	Calls               int64 `firestore:"calls" json:"calls"`
}

// AdaptState is the difficulty engine's running state.
//
// This MUST be persisted. The engine is pure and holds nothing between turns,
// and each turn is graded by a worker that rebuilds its input from Firestore —
// possibly on a different instance. Omitting these fields silently disables
// adaptation entirely: the streak resets to zero every turn and can never reach
// the two consecutive turns a band change requires. Unit tests do not catch it,
// because they keep one State in memory across calls.
type AdaptState struct {
	// LastScore backs the rolling average.
	LastScore    float64 `firestore:"lastScore" json:"lastScore"`
	HasLastScore bool    `firestore:"hasLastScore" json:"hasLastScore"`
	// StrongStreak and WeakStreak count consecutive turns past a threshold.
	StrongStreak int `firestore:"strongStreak" json:"strongStreak"`
	WeakStreak   int `firestore:"weakStreak" json:"weakStreak"`
	// LastChangeTurn drives the anti-oscillation cooldown.
	LastChangeTurn int `firestore:"lastChangeTurn" json:"lastChangeTurn"`
	// TurnIndex counts graded turns, which is not the same as turnCount:
	// skipped and ungraded turns do not advance the ladder.
	TurnIndex int `firestore:"turnIndex" json:"turnIndex"`
}

// Session is the root document for one interview or study run.
type Session struct {
	ID     string        `firestore:"-" json:"id"`
	UID    string        `firestore:"uid" json:"uid"`
	Mode   Mode          `firestore:"mode" json:"mode"`
	Status SessionStatus `firestore:"status" json:"status"`

	Persona Persona `firestore:"persona,omitempty" json:"persona,omitempty"`
	Topic   string  `firestore:"topic,omitempty" json:"topic,omitempty"`

	CreatedAt  time.Time  `firestore:"createdAt" json:"createdAt"`
	StartedAt  *time.Time `firestore:"startedAt,omitempty" json:"startedAt,omitempty"`
	EndedAt    *time.Time `firestore:"endedAt,omitempty" json:"endedAt,omitempty"`
	DurationMs int64      `firestore:"durationMs" json:"durationMs"`

	// DifficultyBand is 1–5. Entry is 3 for mid-level and 2 for entry-level;
	// never 1 for a candidate with a real resume — it is insulting and it
	// wastes the opening of a short demo.
	DifficultyBand int          `firestore:"difficultyBand" json:"difficultyBand"`
	BandHistory    []BandChange `firestore:"bandHistory" json:"bandHistory"`

	Coverage Coverage   `firestore:"coverage" json:"coverage"`
	Adapt    AdaptState `firestore:"adapt" json:"adapt"`

	// Digest is the structured resume + JD extraction (PRD §6.1). Held as a
	// map until Phase 3 defines its schema, so the store does not need to
	// change when it lands.
	Digest map[string]any `firestore:"digest,omitempty" json:"digest,omitempty"`

	ResumeGCSURI string `firestore:"resumeGcsUri,omitempty" json:"resumeGcsUri,omitempty"`
	JDText       string `firestore:"jdText,omitempty" json:"jdText,omitempty"`

	LiveMeta  LiveMeta     `firestore:"liveSessionMeta" json:"liveSessionMeta"`
	Cost      CostEstimate `firestore:"costEstimate" json:"costEstimate"`
	TurnCount int          `firestore:"turnCount" json:"turnCount"`

	// FixtureID names the recording to replay when Mode is ModeReplay.
	FixtureID string `firestore:"fixtureId,omitempty" json:"fixtureId,omitempty"`
}

// Span is one highlighted stretch of the candidate's answer.
type Span struct {
	// Excerpt is copied verbatim from the transcript. We never ask the model
	// for character offsets — roughly a third come back off by a few
	// characters and highlights land mid-word. Anchoring happens server-side.
	Excerpt     string  `firestore:"excerpt" json:"excerpt"`
	Verdict     Verdict `firestore:"verdict" json:"verdict"`
	Concept     string  `firestore:"concept" json:"concept"`
	Explanation string  `firestore:"explanation" json:"explanation"`
	Correction  string  `firestore:"correction,omitempty" json:"correction,omitempty"`

	// Confidence drives the server-side downgrade rule (AD-4): an "incorrect"
	// verdict below the configured threshold is rewritten to "unsupported".
	// Falsely flagging a correct answer red is the most damaging thing this
	// product can do, and a prompt instruction alone does not reliably
	// prevent it.
	Confidence float64 `firestore:"confidence" json:"confidence"`

	// Start and End are resolved by the anchoring pass, not by the model.
	// A span that cannot be anchored is dropped: a missing highlight is
	// invisible, a wrong one is a bug the judge sees.
	Start int `firestore:"start" json:"start"`
	End   int `firestore:"end" json:"end"`
}

// Scores are the four rubric dimensions, each 1–10.
type Scores struct {
	TechnicalAccuracy    int `firestore:"technical_accuracy" json:"technical_accuracy"`
	CommunicationClarity int `firestore:"communication_clarity" json:"communication_clarity"`
	Depth                int `firestore:"depth" json:"depth"`
	Structure            int `firestore:"structure" json:"structure"`
}

// Evaluation is the graded result for one turn (PRD §11.2). Embedded in the
// turn document rather than kept in a subcollection: one read renders a turn.
type Evaluation struct {
	TurnID               string   `firestore:"turnId" json:"turn_id"`
	QuestionIntent       string   `firestore:"questionIntent" json:"question_intent"`
	Scores               Scores   `firestore:"scores" json:"scores"`
	VerdictSummary       string   `firestore:"verdictSummary" json:"verdict_summary"`
	Spans                []Span   `firestore:"spans" json:"spans"`
	ConceptsDemonstrated []string `firestore:"conceptsDemonstrated" json:"concepts_demonstrated"`
	ConceptsMissing      []string `firestore:"conceptsMissing" json:"concepts_missing"`
	IdealAnswerOutline   []string `firestore:"idealAnswerOutline" json:"ideal_answer_outline"`

	// FollowupProbe becomes the next question via the injection loop. The
	// grader saw exactly where the answer thinned out, so its question is
	// sharper than anything the interviewer would improvise — this is where
	// adaptation stops being a checkbox.
	FollowupProbe string `firestore:"followupProbe" json:"followup_probe"`

	DifficultyRecommendation string `firestore:"difficultyRecommendation" json:"difficulty_recommendation"`

	// Diagnostics, not model output.
	TurnScore      float64   `firestore:"turnScore" json:"turnScore"`
	SpansDropped   int       `firestore:"spansDropped" json:"spansDropped"`
	RedsDowngraded int       `firestore:"redsDowngraded" json:"redsDowngraded"`
	PromptVersion  string    `firestore:"promptVersion,omitempty" json:"promptVersion,omitempty"`
	Model          string    `firestore:"model,omitempty" json:"model,omitempty"`
	GradedAt       time.Time `firestore:"gradedAt" json:"gradedAt"`
	DurationMs     int64     `firestore:"durationMs" json:"durationMs"`
}

// Delivery holds how the answer sounded (PRD §13).
//
// The deterministic fields are computed in Go. Never ask a model for
// arithmetic that Go can do — and never count fillers from the transcript,
// because speech recognition normalises disfluencies out and the counter would
// permanently read zero.
type Delivery struct {
	WPM             float64  `firestore:"wpm" json:"wpm"`
	SpeakingTimeMs  int64    `firestore:"speakingTimeMs" json:"speakingTimeMs"`
	WordCount       int      `firestore:"wordCount" json:"wordCount"`
	FillerCount     int      `firestore:"fillerCount" json:"fillerCount"`
	FillerInstances []string `firestore:"fillerInstances,omitempty" json:"fillerInstances,omitempty"`
	HesitationScore float64  `firestore:"hesitationScore" json:"hesitationScore"`
	Observation     string   `firestore:"observation,omitempty" json:"observation,omitempty"`
	Drill           string   `firestore:"drill,omitempty" json:"drill,omitempty"`
}

// Hint is one Socratic nudge and its score penalty.
type Hint struct {
	Text        string    `firestore:"text" json:"text"`
	RequestedAt time.Time `firestore:"requestedAt" json:"requestedAt"`
	Penalty     float64   `firestore:"penalty" json:"penalty"`
}

// Turn is one question-and-answer exchange.
type Turn struct {
	ID    string `firestore:"-" json:"id"`
	Index int    `firestore:"index" json:"index"`

	QuestionText     string   `firestore:"questionText" json:"questionText"`
	QuestionConcepts []string `firestore:"questionConcepts" json:"questionConcepts"`
	QuestionBand     int      `firestore:"questionBand" json:"questionBand"`

	AskedAt         time.Time  `firestore:"askedAt" json:"askedAt"`
	AnswerStartedAt *time.Time `firestore:"answerStartedAt,omitempty" json:"answerStartedAt,omitempty"`
	AnswerEndedAt   *time.Time `firestore:"answerEndedAt,omitempty" json:"answerEndedAt,omitempty"`

	UserTranscript      string    `firestore:"userTranscript" json:"userTranscript"`
	UserTranscriptFinal bool      `firestore:"userTranscriptFinal" json:"userTranscriptFinal"`
	InputMode           InputMode `firestore:"inputMode" json:"inputMode"`

	AudioGCSURI     string `firestore:"audioGcsUri,omitempty" json:"audioGcsUri,omitempty"`
	AudioDurationMs int64  `firestore:"audioDurationMs" json:"audioDurationMs"`

	HintsUsed int    `firestore:"hintsUsed" json:"hintsUsed"`
	Hints     []Hint `firestore:"hints,omitempty" json:"hints,omitempty"`

	Evaluation *Evaluation `firestore:"evaluation,omitempty" json:"evaluation,omitempty"`
	Delivery   *Delivery   `firestore:"delivery,omitempty" json:"delivery,omitempty"`

	GradingStatus GradingStatus `firestore:"gradingStatus" json:"gradingStatus"`
	GradingError  string        `firestore:"gradingError,omitempty" json:"gradingError,omitempty"`
}

// WordCount returns the number of whitespace-separated words in the answer.
// Used to skip grading trivial turns and to compute WPM.
func (t *Turn) WordCount() int {
	return len(fields(t.UserTranscript))
}

// DailyUsage is the per-day token ledger, broken down by model.
//
// "Here are our actual per-session unit economics" is a genuinely strong answer
// to a judge asking about viability, and it is only available if the numbers
// were recorded from day one.
type DailyUsage struct {
	Date       string                  `firestore:"date" json:"date"`
	ByModel    map[string]CostEstimate `firestore:"byModel" json:"byModel"`
	TotalCalls int64                   `firestore:"totalCalls" json:"totalCalls"`
	UpdatedAt  time.Time               `firestore:"updatedAt" json:"updatedAt"`
}

// DailyCounter enforces the per-user daily session cap, which prevents a single
// enthusiastic tester from burning the demo budget.
type DailyCounter struct {
	Date      string    `firestore:"date" json:"date"`
	Sessions  int       `firestore:"sessions" json:"sessions"`
	UpdatedAt time.Time `firestore:"updatedAt" json:"updatedAt"`
}
