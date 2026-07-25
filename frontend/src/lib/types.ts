import type { PersonaId } from './persona'
import type { Verdict } from './verdict'

/**
 * The backend contract, transcribed from the Go struct tags.
 *
 * ── Read the casing carefully ────────────────────────────────────────────
 *
 * The wire format mixes camelCase and snake_case, and it is deliberate rather
 * than sloppy: anything mirroring a model's response schema keeps the schema's
 * snake_case, and anything the backend owns is camelCase. Getting it wrong
 * produces `undefined` rather than a type error, because JSON arriving over the
 * wire is unchecked at runtime.
 *
 * The two that catch people:
 *   - Evaluation is MIXED. Model output is snake_case (question_intent,
 *     concepts_missing, followup_probe); backend diagnostics are camelCase
 *     (turnScore, spansDropped, gradedAt).
 *   - Scores is entirely snake_case, INCLUDING inside report.aggregateScores.
 */

export type Mode = 'interview' | 'study' | 'replay'
export type SessionStatus = 'configuring' | 'live' | 'evaluating' | 'complete' | 'abandoned'
export type InputMode = 'voice' | 'text'
export type GradingStatus = 'pending' | 'complete' | 'failed' | 'skipped'
export type DifficultyRecommendation = 'raise' | 'hold' | 'lower'

// ── Session ───────────────────────────────────────────────────────────────

export interface Coverage {
  /** Never re-tested. */
  proven: string[]
  /** Re-approached from a different angle, never repeated verbatim. */
  shaky: string[]
  /** Named in the plan or the JD but never addressed. Drives the roadmap. */
  missing: string[]
}

export interface BandChange {
  turnIndex: number
  band: number
  /** Written as an explanation, not a log line — it is the toast's copy. */
  reason: string
  at: string
}

export interface CostEstimate {
  totalTokens: number
  /** Audio and text are tracked apart because audio tokens cost far more. */
  promptAudioTokens: number
  responseAudioTokens: number
  promptTextTokens: number
  responseTextTokens: number
  calls: number
}

export interface AdaptState {
  lastScore: number
  hasLastScore: boolean
  strongStreak: number
  weakStreak: number
  lastChangeTurn: number
  /** Counts GRADED turns, which is not the same as turnCount. */
  turnIndex: number
}

export interface LiveMeta {
  model?: string
  voice?: string
  location?: string
  resumeCount: number
}

export interface Session {
  id: string
  uid: string
  mode: Mode
  status: SessionStatus
  persona?: PersonaId
  topic?: string

  createdAt: string
  startedAt?: string
  endedAt?: string
  durationMs: number

  /** 1-5 on paper; the backend's ladder clamps to 2-5. */
  difficultyBand: number
  bandHistory: BandChange[]

  coverage: Coverage
  adapt: AdaptState

  /** Untyped by design — the digest's shape is owned by the ingest schema. */
  digest?: Record<string, unknown>
  resumeGcsUri?: string
  jdText?: string

  liveSessionMeta: LiveMeta
  costEstimate: CostEstimate
  turnCount: number
  fixtureId?: string
}

// ── Digest ────────────────────────────────────────────────────────────────

export interface DigestClaim {
  id: string
  text: string
  artifact: string
  verifiable_depth: 'high' | 'medium' | 'low'
  /** The questions that test whether the candidate actually did this. */
  probe_angles: string[]
}

export interface DigestPlanArea {
  area: string
  why: string
  opening_question_seed: string
  target_band: number
  /** Set by PATCH /plan. Areas are marked, never removed. */
  dropped?: boolean
}

export interface Digest {
  candidate?: {
    seniority_estimate?: 'entry' | 'junior' | 'mid' | 'senior'
    primary_stack?: string[]
    claims?: DigestClaim[]
    gaps_vs_jd?: string[]
  }
  role?: {
    title?: string
    must_haves?: string[]
    nice_to_haves?: string[]
    implied_seniority?: string
    /** These become the radar chart's axes. */
    domain_areas?: string[]
  }
  interview_plan?: DigestPlanArea[]
}

export interface DigestResponse {
  digest: Digest
  meta: {
    model: string
    promptVersion: string
    durationMs: number
    claims: number
    planAreas: number
  }
}

// ── Turns and evaluation ──────────────────────────────────────────────────

export interface Span {
  /** The transcript's own wording at [start,end) — not the model's quote. */
  excerpt: string
  verdict: Verdict
  concept: string
  explanation: string
  correction?: string
  /** Drove the server-side downgrade of a low-confidence `incorrect`. */
  confidence: number
  /** UTF-8 BYTE offsets. Convert before slicing — see lib/byteOffset. */
  start: number
  end: number
}

/** Entirely snake_case, including inside report.aggregateScores. */
export interface Scores {
  technical_accuracy: number
  communication_clarity: number
  depth: number
  structure: number
}

export interface Evaluation {
  // Model output — snake_case.
  turn_id: string
  question_intent: string
  scores: Scores
  verdict_summary: string
  spans: Span[]
  concepts_demonstrated: string[]
  concepts_missing: string[]
  ideal_answer_outline: string[]
  /** Becomes the next question via the injection loop. */
  followup_probe: string
  difficulty_recommendation: DifficultyRecommendation

  // Backend diagnostics — camelCase.
  /** Persona-weighted, 0-10, with the hint penalty already applied. */
  turnScore: number
  spansDropped: number
  redsDowngraded: number
  promptVersion?: string
  model?: string
  gradedAt: string
  durationMs: number
}

export interface Delivery {
  wpm: number
  speakingTimeMs: number
  wordCount: number
  /** Counted from the ANSWER AUDIO — transcripts have disfluencies removed. */
  fillerCount: number
  fillerInstances?: string[]
  hesitationScore: number
  observation?: string
  drill?: string
}

export interface Hint {
  text: string
  requestedAt: string
  penalty: number
}

export interface Turn {
  id: string
  index: number

  questionText: string
  questionConcepts: string[]
  questionBand: number

  askedAt: string
  answerStartedAt?: string
  answerEndedAt?: string

  userTranscript: string
  userTranscriptFinal: boolean
  inputMode: InputMode

  audioGcsUri?: string
  audioDurationMs: number

  hintsUsed: number
  hints?: Hint[]

  /** Absent until graded. Absent forever if skipped or failed. */
  evaluation?: Evaluation
  /** Absent for typed answers — there is no audio to listen to. */
  delivery?: Delivery

  gradingStatus: GradingStatus
  gradingError?: string
}

// ── Report ────────────────────────────────────────────────────────────────

export interface DomainScore {
  domain: string
  score: number
  turnCount: number
}

export interface TurnSummary {
  turnId: string
  index: number
  question: string
  score: number
  hintsUsed: number
  band: number
  graded: boolean
  /** Only verdicts with a non-zero count are present. */
  spanCounts: Partial<Record<Verdict, number>>
}

export type PaceBand = 'hesitant' | 'optimal' | 'rushed' | 'too fast'

export interface DeliveryAggregate {
  wpm: number
  paceBand: PaceBand
  fillerTotal: number
  fillerPerMinute: number
  speakingTimeMs: number
  hesitationScore: number
  observation?: string
  drill?: string
  turnsWithAudio: number
}

export interface Report {
  sessionId: string
  status: 'generating' | 'ready' | 'failed'
  aggregateScores: Scores
  overallScore: number
  /** The radar chart's axes. */
  domainScores: DomainScore[]
  /** The sparkline. */
  bandTrajectory: number[]
  startBand: number
  endBand: number
  /** Capped at 6. */
  strengths: string[]
  /** Capped at 5 — a list of nineteen weaknesses is discouraging, not useful. */
  gaps: string[]
  turns: TurnSummary[]
  delivery: DeliveryAggregate
  turnsGraded: number
  durationMs: number
  generatedAt: string
}

// ── Roadmap ───────────────────────────────────────────────────────────────

export interface RoadmapResource {
  title: string
  url: string
  type: string
  minutes: number
  /**
   * Always true on anything you receive: every URL is fetched server-side and
   * dead links are dropped before the plan is stored.
   */
  verified: boolean
}

export interface RoadmapDay {
  day: number
  focus_concept: string
  why_this_matters: string
  estimated_minutes: number
  resources: RoadmapResource[]
  practice_task: string
  self_check: string
}

export interface RetestPlan {
  after_day: number
  focus_areas: string[]
  recommended_persona: PersonaId
  recommended_band: number
}

export interface Roadmap {
  session_id: string
  horizon_days: number
  summary: string
  days: RoadmapDay[]
  retest_plan: RetestPlan
  /** False when Search grounding failed. The plan is still useful; say so. */
  grounded: boolean
  note?: string
  linksFound: number
  linksDropped: number
  generatedAt: string
}

export interface RetestResponse {
  sessionId: string
  persona: PersonaId
  band: number
  focusAreas: string[]
}

// ── Personas ──────────────────────────────────────────────────────────────

export interface PersonaCard {
  id: PersonaId
  name: string
  tagline: string
  /** The field that makes a user pick the one that scares them. */
  punishes: string
  weights: {
    technicalAccuracy: number
    communicationClarity: number
    depth: number
    structure: number
  }
}

// ── Study ─────────────────────────────────────────────────────────────────

export type StudyDepth = 'survey' | 'exam_ready' | 'interview_ready'
export type Mastery = 'unseen' | 'attempted' | 'shaky' | 'solid'
export type Archetype = 'recall' | 'application' | 'edge_case' | 'teach_back'

export interface Subtopic {
  id: string
  name: string
  /** Real edges from the decomposition, not a heuristic. */
  prereqs: string[]
  depth: number
  why: string
  mastery: Mastery
  archetype: Archetype
  attempts: number
  /** The only route to `solid`. */
  teachBackPassed: boolean
}

export interface Syllabus {
  topic: string
  depth: StudyDepth
  subtopics: Subtopic[]
  createdAt: string
}

export interface MasteryStats {
  total: number
  unseen: number
  attempted: number
  shaky: number
  solid: number
}

export interface StudyQuestion {
  complete: boolean
  subtopicId?: string
  subtopic?: string
  archetype?: Archetype
  archetypeLabel?: string
  question?: string
  mastery: MasteryStats
  band?: number
  message?: string
}

export interface StudyAnswerResult {
  evaluation: Evaluation
  masteryFrom: Mastery
  masteryTo: Mastery
  passed: boolean
  nextArchetype: Archetype
  /** Subtopics whose only blocker was the one just answered. A real moment. */
  unlocked: string[] | null
  mastery: MasteryStats
  complete: boolean
}

export interface MasteryMap {
  topic: string
  subtopics: Subtopic[]
  mastery: MasteryStats
  complete: boolean
}

// ── Misc ──────────────────────────────────────────────────────────────────

export interface Me {
  uid: string
  email?: string
  displayName?: string
  anonymous: boolean
}

export interface SessionUsage {
  sessionId: string
  cost: CostEstimate
  turnCount: number
}

/** GET /report and /roadmap answer 202 until the worker has written them. */
export type Pending<T> =
  | { ready: true; value: T }
  | { ready: false; status: 'generating' | 'not_started'; sessionStatus: SessionStatus }
