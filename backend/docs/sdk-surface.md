# Verified `google.golang.org/genai` surface — v1.65.0

Captured with `go doc` against the pinned module on **25 Jul 2026**. This file is the
authority for the Live API surface, not pkg.go.dev prose and not the PRD. Re-run the
commands in `make sdk-doc` if the pinned version ever changes.

## Corrections this pass produced

| Assumed (PRD / pkg.go.dev prose) | Actually true in v1.65.0 |
|---|---|
| `genai.BackendEnterprise` | **`genai.BackendVertexAI`** — the PRD was right, the rendered docs were wrong |
| `Session` is an interface | `*genai.Session` is a **struct** |
| `Receive() iter.Seq2[*LiveServerMessage, error]` | **`Receive() (*LiveServerMessage, error)`** — a blocking call you drive in a `for` loop |
| `SendRealtimeInput(*LiveSendRealtimeInputParameters)` | Takes a **value**: `SendRealtimeInput(LiveRealtimeInput)` |
| `SendClientContent(*LiveClientContentInput)` | Takes a **value**: `SendClientContent(LiveClientContentInput)` |
| `LiveConnectConfig.SystemInstruction string` | **`*Content`** |
| `ServerContent.TurnComplete *TurnCompleteReason` | **`bool`** (separate `TurnCompleteReason` field exists alongside it) |
| `ServerContent.UsageMetadata` | Lives on the **top-level `LiveServerMessage`**, not on `ServerContent` |
| `ClientConfig.HTTPOptions *HTTPOptions` | **Value** type, not a pointer |

`Live`, `Session`, and the `Live*` input aliases are all marked **Preview** in the SDK
doc comments. PRD risk R1 stands, but only for this surface — everything else is stable.

## Client construction

```go
type ClientConfig struct {
    APIKey      string          // BackendGeminiAPI only — we never set this
    Backend     Backend         // BackendVertexAI
    Project     string          // required for Vertex
    Location    string          // required for Vertex
    Credentials *auth.Credentials // nil => Application Default Credentials
    HTTPClient  *http.Client
    HTTPOptions HTTPOptions     // value, not pointer
}

func NewClient(ctx context.Context, cc *ClientConfig) (*Client, error)
func (cc *ClientConfig) UseDefaultCredentials() error
```

`Backend` constants: `BackendUnspecified` (iota), `BackendGeminiAPI`, `BackendVertexAI`.
`BackendUnspecified` resolves from `GOOGLE_GENAI_USE_VERTEXAI`. **We always set it
explicitly** so a stray env var can never silently route us to the Gemini API and bill
against something other than Vertex.

## Live

```go
func (r *Live) Connect(ctx context.Context, model string, config *LiveConnectConfig) (*Session, error)

type Session struct {
    SetupComplete *LiveServerSetupComplete
}
func (s *Session) Close() error
func (s *Session) Receive() (*LiveServerMessage, error)          // blocking, loop it
func (s *Session) SendClientContent(input LiveClientContentInput) error
func (s *Session) SendRealtimeInput(input LiveRealtimeInput) error
func (s *Session) SendToolResponse(input LiveToolResponseInput) error
```

### `LiveConnectConfig` — fields we use

| Field | Type | Use |
|---|---|---|
| `ResponseModalities` | `[]Modality` | `[ModalityAudio]` |
| `SpeechConfig` | `*SpeechConfig` | `.VoiceConfig` → per-persona voice; `.LanguageCode` |
| `SystemInstruction` | `*Content` | assembled persona instruction |
| `InputAudioTranscription` | `*AudioTranscriptionConfig` | enable (empty struct) |
| `OutputAudioTranscription` | `*AudioTranscriptionConfig` | enable — sources the on-screen question text |
| `RealtimeInputConfig` | `*RealtimeInputConfig` | AD-2 manual activity detection |
| `SessionResumption` | `*SessionResumptionConfig` | reconnect handle (Phase 5) |
| `ContextWindowCompression` | `*ContextWindowCompressionConfig` | long-session safety |
| `Temperature` | `*float32` | persona register |

Also present and worth knowing about: `EnableAffectiveDialog *bool`, `Proactivity
*ProactivityConfig`, `ExplicitVADSignal *bool`, `ThinkingConfig`, `Tools`.

### AD-2 — manual activity detection is fully supported

```go
RealtimeInputConfig: &genai.RealtimeInputConfig{
    AutomaticActivityDetection: &genai.AutomaticActivityDetection{Disabled: true},
}
```

```go
type LiveSendRealtimeInputParameters struct {  // alias: LiveRealtimeInput
    Media          *Blob
    Audio          *Blob          // dedicated audio channel — prefer over Media
    AudioStreamEnd bool           // ONLY valid when automatic detection is enabled
    Video          *Blob
    Text           string
    ActivityStart  *ActivityStart // manual boundary open
    ActivityEnd    *ActivityEnd   // manual boundary close
}
```

`AutomaticActivityDetection` also exposes `StartOfSpeechSensitivity`,
`EndOfSpeechSensitivity`, `PrefixPaddingMs`, `SilenceDurationMs` — the tuning knobs for
`LIVE_ACTIVITY_MODE=auto` if we ever demo that path.

### Injection (AD-3)

```go
type LiveSendClientContentParameters struct {  // alias: LiveClientContentInput
    Turns        []*Content
    TurnComplete *bool          // nil defaults to true; true triggers generation
}
```

### Receiving

```go
type LiveServerMessage struct {
    SetupComplete           *LiveServerSetupComplete
    ServerContent           *LiveServerContent
    ToolCall                *LiveServerToolCall
    ToolCallCancellation    *LiveServerToolCallCancellation
    UsageMetadata           *UsageMetadata               // top level — cost ledger reads this
    GoAway                  *LiveServerGoAway
    SessionResumptionUpdate *LiveServerSessionResumptionUpdate
    VoiceActivityDetectionSignal *VoiceActivityDetectionSignal
    VoiceActivity           *VoiceActivity
}

type LiveServerContent struct {
    ModelTurn                 *Content        // audio parts live here
    TurnComplete              bool
    Interrupted               bool
    GenerationComplete        bool
    WaitingForInput           bool
    InputTranscription        *Transcription  // finalized user speech
    InterimInputTranscription *Transcription  // low-latency, updates while speaking
    OutputTranscription       *Transcription  // what the AI said
    TurnCompleteReason        TurnCompleteReason
    GroundingMetadata         *GroundingMetadata
    URLContextMetadata        *URLContextMetadata
}
```

Two notes that shape the relay:

- **A message is a bag of parts.** Several of these fields can be populated on the same
  message. Never write this as a switch that handles one field and continues.
- **`InterimInputTranscription` is a gift.** PRD §9.3 asks for interim transcript at
  reduced opacity finalizing as it stabilises. That is this field plus
  `InputTranscription`, with no extra work.

## Audio format

| Direction | Format |
|---|---|
| In | PCM16, **16 kHz**, mono — `Blob{MIMEType: "audio/pcm;rate=16000"}` |
| Out | PCM16, **24 kHz**, mono |

The sample-rate asymmetry is real; do not resample one to match the other by accident.
