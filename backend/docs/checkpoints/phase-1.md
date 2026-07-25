# Phase 1 checkpoint — The Live Spike

**Status: complete.** The riskiest phase in the plan is done, and the risk is retired.

> **Exit criterion:** hold a spoken conversation with Gemini through our own
> stack, with both transcripts rendering. ✅ Met, locally and through the full
> browser protocol.

## What exists now

| Component | Purpose |
|---|---|
| `internal/audio` | PCM16 helpers: WAV read/write, resampling, RMS, frame splitting. 8 unit tests. |
| `cmd/livespike` | Standalone Vertex Live proof. `-mode=speak` (text→audio), `-mode=listen` (audio→audio). |
| `internal/live` | The relay: WS upgrade, ordered upstream queue, dispatch, idle/duration guards. 5 unit tests. |
| `cmd/wsprobe` | CLI stand-in for the browser. Streams a WAV at wall-clock pace, prints every frame, writes received audio to a WAV. |

`go vet` and `go test ./...` clean. Deployed as revision `crucible-backend-00003-hb4`.

## Measured results

Same 5.2 s audio fixture throughout.

| Path | Turn-boundary latency | Notes |
|---|---|---|
| Direct to Vertex (`livespike`) | 892 / 892 / 1052 / 1109 ms | The floor. Tight variance. |
| Through the relay, **final** | 966 / 1130 / 1213 / 1420 ms | Matches the floor. PRD §4.4 target is < 1200 ms. |
| Through the relay, before the fix below | 1548 – 2309 ms | ~800 ms of self-inflicted latency. |

Also verified: **zero audio sequence gaps** across every run, input transcription
returns verbatim, output transcription streams word-by-word, token usage splits
audio from text correctly, and the text-answer path produces an identical turn
in 924 ms.

## The bug worth remembering

**A client must not send audio until the server reports `LISTENING`.**

The WebSocket upgrade completes in milliseconds; establishing the Vertex Live
session behind it takes ~2 s. A client that streams on connect fills the socket
buffer during that window. The relay then drains it in a burst and hands Vertex
several seconds of audio compressed into an instant — and Vertex is still
ingesting that backlog when the turn closes, so the whole delay lands on
turn-boundary latency.

The instrumentation that found it compares the wall-clock upload window against
the true duration of the audio received:

```
before:  upload_ms=2846  audio_ms=5200  drift_ms=-2354   ← 1.8x real time
after:   upload_ms=5200  audio_ms=5200  drift_ms=0
```

This is now a documented protocol requirement in `protocol.go` and the frontend
must honour it. It would otherwise have presented on stage as "the first
question takes forever", with nothing in the logs pointing at a cause.

### Two hypotheses that were wrong

Recorded because both are plausible and both cost time:

1. *"Per-frame API overhead accumulates."* Coalescing 20 ms frames into 100 ms
   chunks changed latency by nothing measurable.
2. *"The upstream queue delays activity_end."* Instrumented it directly:
   `activity_end_queue_lag_ms` is **0**. The relay forwards the turn boundary
   instantly.

Both changes were kept anyway — the decoupled send pump prevents a slow client
from stalling the billing connection, and coalescing cuts API call volume
fivefold — but neither was the cause. The lesson: **instrument before
optimising.** The first baseline (a single 1093 ms sample) was also too noisy to
draw a conclusion from, and re-baselining over four runs was what made the gap
real rather than imagined.

## Architecture notes

- **Sending faster than real time makes latency worse, not better.** Blasting
  the fixture with no pacing measured 6821 ms. Vertex paces its own ingestion at
  roughly real time, so there is nothing to gain and a backlog to lose.
- **Audio and control share one upstream queue.** Their order is load-bearing:
  an `activity_end` that overtook the tail of the audio would close the turn on
  a truncated answer. Two channels would race.
- **One writer goroutine per direction.** gorilla permits a single concurrent
  writer; several goroutines produce frames, so all of them funnel through a
  channel.
- **A server message is a bag of parts.** Audio, input transcript, and output
  transcript arrive together on one message. `dispatch` uses consecutive `if`s,
  never a `switch` — this is the single easiest way to silently drop transcript
  data.
- **Backpressure drops the client, not the model.** A wedged client must never
  apply backpressure all the way to a billing connection.

## Guardrails already live

Two of the nine credit guardrails are enforced in the relay because they are
intrinsic to its lifecycle rather than bolt-on policy:

- **Hard session cap** (12 min) via context timeout on the handler.
- **Idle timeout** (90 s) — the most important one. A forgotten open tab is a
  continuous drain on the most expensive component in the system.

The Live session is closed on every teardown path via `defer`, never left to GC.

## Security note

The deployed service does **not** set `DEV_ALLOW_ANON`. The live socket returns
**401** in production, verified against the deployed URL. An unauthenticated
public WebSocket onto a billing API is a credit risk that does not need to exist
for one phase, so the relay was proven locally instead. Phase 2 adds Firebase ID
token verification and re-tests the socket deployed.

## Notes for Phase 2

- `SessionOpts{Voice, SystemInstruction}` are the seams Phase 3 fills from the
  persona config and resume digest. Nothing else needs to change to make the
  interviewer persona-driven.
- `InterimInputTranscription` did not fire in manual activity mode during these
  runs — only final `InputTranscription` did. Worth confirming whether that is
  inherent to manual mode before the frontend depends on interim rendering for
  the reduced-opacity effect. `dispatch` already forwards it if it appears.
- The relay counts audio frames and bytes per session; that accounting is what
  Phase 4 needs to flush turn audio to GCS as a WAV.
- Connect time to the Live session is consistently ~2 s. Phase 3 should start
  that connection while the user is still on the persona-selection screen, so
  the 2 s is spent behind a screen the user is reading rather than in front of
  a "connecting" spinner. PRD §4.4 budgets < 2.5 s to first audio.
