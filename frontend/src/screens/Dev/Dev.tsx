import { useEffect, useMemo, useState } from 'react'
import { Lightbulb, CornerDownLeft, Keyboard, Square } from 'lucide-react'

import { Orb, useAmplitude } from '../../components/orb'
import { PERSONA_FALLBACK_NAME, PERSONA_IDS, type PersonaId } from '../../lib/persona'
import type { LiveState } from '../../lib/protocol'

import {
  Button,
  ButtonGroup,
  Chip,
  Label,
  Panel,
  Popover,
  StatusLabel,
} from '../../components/primitives'
import { HeatmapReveal } from '../../components/verdict'
import { BAND_NAMES, currentBand, setBand, type Band } from '../../lib/band'
import { resolveSpanRanges } from '../../lib/byteOffset'
import {
  DEMO_SPANS,
  DEMO_TRANSCRIPT,
  DEMO_UNICODE_BYTE_SPANS,
  DEMO_UNICODE_TRANSCRIPT,
} from '../../lib/fixtures'
import { VERDICTS, VERDICT_DEFINITION, VERDICT_NAME, verdictColor } from '../../lib/verdict'
import { pushToast } from '../../store/toasts'
import s from './Dev.module.css'

/**
 * Development-only specimen. Not routed in production builds.
 *
 * It exists to prove the ambient field and the token system by hand, before any
 * WebSocket exists to drive them. Switching bands here should produce the same
 * 1800ms room-temperature change a real promotion produces, because it goes
 * through the identical setBand() path.
 */

const BANDS: Band[] = [1, 2, 3, 4, 5]

const LIVE_STATES: LiveState[] = [
  'CONNECTING',
  'ASKING',
  'LISTENING',
  'CLOSING',
  'EVALUATING',
  'SETTLED',
  'ERROR',
]

const RAMP = [
  '--t-quench',
  '--t-cool',
  '--t-temper',
  '--t-warm',
  '--t-ember',
  '--t-assay',
  '--t-flare',
]

const STRUCTURE = ['--void', '--vessel', '--vessel-high', '--rim', '--rim-lit', '--bone', '--ash', '--dust']

const SPECIMEN: { token: string; label: string; sample: string }[] = [
  { token: '--fs-display', label: 'display', sample: 'This one talks back.' },
  { token: '--fs-h1', label: 'h1', sample: 'The panel' },
  { token: '--fs-h2', label: 'h2', sample: 'The Tech Lead' },
  { token: '--fs-h3', label: 'h3', sample: 'Sub-heading' },
  { token: '--fs-body', label: 'body', sample: 'Upload your resume and the job you are actually chasing.' },
  { token: '--fs-small', label: 'small', sample: 'Free. No card. About ten minutes.' },
  {
    token: '--fs-transcript',
    label: 'transcript',
    sample: 'So the ingestion layer used a Kafka topic per source, and we deduplicated downstream using a bloom filter — naïve, but it held at 2000 req/s.',
  },
  { token: '--fs-quote', label: 'quote', sample: 'And what happens when that fails?' },
  { token: '--fs-metric', label: 'metric', sample: '148' },
  { token: '--fs-label', label: 'label', sample: 'LIVE TRANSCRIPT' },
  { token: '--fs-mono-sm', label: 'mono-sm', sample: '09:42 · turn 3 of ~6' },
]

export default function Dev() {
  const [band, setLocalBand] = useState<Band>(currentBand)
  const [highContrast, setHighContrast] = useState(false)
  const [revealNonce, setRevealNonce] = useState(0)
  const [orbState, setOrbState] = useState<LiveState>('LISTENING')
  const [persona, setPersona] = useState<PersonaId>('tech_lead')
  const [speaking, setSpeaking] = useState(false)
  const amplitude = useAmplitude()

  // Stands in for the audio pipeline until it exists. Shaped like speech —
  // syllables inside phrases, with pauses — rather than noise, because a
  // random walk hides exactly the responsiveness this is meant to prove.
  const { push } = amplitude
  useEffect(() => {
    if (!speaking) return
    let t = 0
    const id = setInterval(() => {
      t += 0.02
      const syllable = Math.max(0, Math.sin(t * Math.PI * 2 * 3.5))
      const inPhrase = Math.sin(t * Math.PI * 2 * 0.22) > -0.4 ? 1 : 0
      push(0.05 + syllable * inPhrase * 0.18 * (0.7 + Math.random() * 0.6))
    }, 20)
    return () => clearInterval(id)
  }, [speaking, push])

  // The byte→character conversion, exercised through the real component. Every
  // span in this fixture sits after at least one multi-byte character, so a
  // regression slides the highlights off their words visibly.
  const unicodeRanges = useMemo(
    () => resolveSpanRanges(DEMO_UNICODE_TRANSCRIPT, DEMO_UNICODE_BYTE_SPANS),
    [],
  )

  function pick(next: Band) {
    setBand(next)
    setLocalBand(next)
    // Mirrors the real band choreography: the toast carries the backend's own
    // `message` copy and is accented with the new band's ramp colour.
    pushToast({
      title: `Band ${next} — ${BAND_NAMES[next]}`,
      message:
        next > band
          ? "Difficulty raised — you've proven the fundamentals."
          : "Easing off — let's rebuild from the mechanism.",
      accent: 'var(--heat-hot)',
    })
  }

  function toggleContrast() {
    const next = !highContrast
    setHighContrast(next)
    if (next) document.documentElement.dataset.contrast = 'high'
    else delete document.documentElement.dataset.contrast
  }

  return (
    <div className={s.page}>
      <section className={s.section}>
        <h2 className={s.sectionHead}>Thermal field · band</h2>
        <div className={s.controls}>
          {BANDS.map((b) => (
            <button
              key={b}
              type="button"
              onClick={() => pick(b)}
              className={`${s.bandBtn} ${b === band ? s.bandBtnOn : ''}`}
              aria-pressed={b === band}
            >
              Band {b}
            </button>
          ))}
          <button type="button" onClick={toggleContrast} className={s.bandBtn} aria-pressed={highContrast}>
            {highContrast ? 'Contrast: high' : 'Contrast: normal'}
          </button>
        </div>
        <div className={s.panel}>
          <div className={s.specimenLabel}>Width axis · wdth {`{`}--band-width{`}`}</div>
          <div className={s.bandNumeral}>{band}</div>
          <div className={s.specimenLabel}>{BAND_NAMES[band]}</div>
        </div>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Thermal ramp</h2>
        <div className={s.ramp}>
          {RAMP.map((token) => (
            <div key={token} className={s.swatch}>
              <div className={s.swatchChip} style={{ background: `var(${token})` }} />
              <div className={s.swatchName}>{token}</div>
            </div>
          ))}
        </div>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Structure</h2>
        <div className={s.ramp}>
          {STRUCTURE.map((token) => (
            <div key={token} className={s.swatch}>
              <div className={s.swatchChip} style={{ background: `var(${token})` }} />
              <div className={s.swatchName}>{token}</div>
            </div>
          ))}
        </div>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Orb</h2>
        <div className={s.controls}>
          {PERSONA_IDS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => setPersona(p)}
              className={`${s.bandBtn} ${p === persona ? s.bandBtnOn : ''}`}
              aria-pressed={p === persona}
            >
              {PERSONA_FALLBACK_NAME[p].replace('The ', '')}
            </button>
          ))}
          <Button
            variant={speaking ? 'primary' : 'secondary'}
            onClick={() => setSpeaking((v) => !v)}
          >
            {speaking ? 'Stop audio' : 'Simulate speech'}
          </Button>
        </div>

        <div className={s.orbRow}>
          <div className={s.orbStage}>
            <Orb ref={amplitude.ref} state={orbState} persona={persona} size={132} />
            <Label tone="quiet">{orbState} · driven</Label>
          </div>

          <div className={s.orbGrid}>
            {LIVE_STATES.map((state) => (
              <button
                key={state}
                type="button"
                className={s.orbCell}
                onClick={() => setOrbState(state)}
                aria-pressed={state === orbState}
              >
                <Orb state={state} persona={persona} size={56} />
                <Label tone={state === orbState ? 'loud' : 'quiet'}>{state}</Label>
              </button>
            ))}
          </div>
        </div>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Heatmap reveal</h2>
        <div className={s.controls}>
          <Button variant="secondary" onClick={() => setRevealNonce((n) => n + 1)}>
            Replay reveal
          </Button>
          <Label tone="quiet">hover or tab a span for its explanation · esc closes</Label>
        </div>
        <Panel flush>
          <div style={{ padding: 'var(--s-5)' }}>
            <HeatmapReveal
              text={DEMO_TRANSCRIPT}
              ranges={DEMO_SPANS}
              revealKey={revealNonce}
            />
          </div>
        </Panel>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Heatmap · UTF-8 byte offsets</h2>
        <Label tone="quiet">
          accent, em dash, curly quote and emoji — spans arrive as Go byte offsets
        </Label>
        <Panel flush>
          <div style={{ padding: 'var(--s-5)' }}>
            <HeatmapReveal
              text={DEMO_UNICODE_TRANSCRIPT}
              ranges={unicodeRanges}
              revealKey={`unicode-${revealNonce}`}
            />
          </div>
        </Panel>
        <Label tone="quiet">
          resolved {unicodeRanges.length} of {DEMO_UNICODE_BYTE_SPANS.length} spans
        </Label>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Buttons</h2>
        <div className={s.controls}>
          <Button variant="primary" size="hero">
            Start a session
          </Button>
          <Button variant="secondary">Type instead</Button>
          <Button variant="ghost">How it works</Button>
          <Button variant="danger">End interview</Button>
          <Button variant="secondary" size="compact">
            Compact
          </Button>
          <Button variant="primary" disabled>
            Disabled
          </Button>
        </div>
        <div style={{ maxWidth: 300 }}>
          <ButtonGroup>
            <Button variant="ghost" icon={<Lightbulb size={20} strokeWidth={1.5} />}>
              Request a hint
            </Button>
            <Button variant="ghost" icon={<CornerDownLeft size={20} strokeWidth={1.5} />}>
              I&rsquo;m done answering
            </Button>
            <Button variant="ghost" icon={<Keyboard size={20} strokeWidth={1.5} />}>
              Type instead
            </Button>
            <Button variant="danger" icon={<Square size={20} strokeWidth={1.5} />}>
              End interview
            </Button>
          </ButtonGroup>
        </div>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Panels, labels, status</h2>
        <div className={s.ramp}>
          <Panel title="Interview session">
            <Label tone="quiet">The Tech Lead</Label>
          </Panel>
          <Panel title="Live transcript" aside="Q3 / ~6">
            <Label>evaluating technical depth</Label>
          </Panel>
          <Panel title="Delivery">
            <StatusLabel color="var(--state-live)" pulse>
              Listening
            </StatusLabel>
          </Panel>
          <Panel title="Progress">
            <StatusLabel color="var(--state-thinking)">Evaluating</StatusLabel>
          </Panel>
        </div>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Chips · concept heatmap</h2>
        <div className={s.controls}>
          <Chip>queue depth</Chip>
          <Chip tone="validated">bloom filter</Chip>
          <Chip tone="incomplete">cache eviction</Chip>
          <Chip tone="unsupported">2000 req/s</Chip>
          <Chip tone="incorrect">backpressure</Chip>
        </div>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Verdict popovers</h2>
        <div className={s.controls} style={{ alignItems: 'flex-start' }}>
          {VERDICTS.map((verdict) => (
            <Popover
              key={verdict}
              verdict={verdict}
              concept={VERDICT_NAME[verdict].toLowerCase()}
              explanation={VERDICT_DEFINITION[verdict]}
              correction={
                verdict === 'incorrect' || verdict === 'incomplete'
                  ? 'A bigger buffer delays the problem. It is not flow control.'
                  : undefined
              }
              style={{ color: verdictColor(verdict) }}
            />
          ))}
        </div>
      </section>

      <section className={s.section}>
        <h2 className={s.sectionHead}>Type scale</h2>
        <div className={s.specimen}>
          {SPECIMEN.map(({ token, label, sample }) => (
            <div key={token} className={s.specimenRow}>
              <div className={s.specimenLabel}>{label}</div>
              <div
                style={{
                  font: `var(${token})`,
                  letterSpacing: `var(${token.replace('--fs-', '--tr-')})`,
                  color: 'var(--bone)',
                }}
              >
                {sample}
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
