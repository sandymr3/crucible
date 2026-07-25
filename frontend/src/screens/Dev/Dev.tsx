import { useState } from 'react'

import { BAND_NAMES, currentBand, setBand, type Band } from '../../lib/band'
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

  function pick(next: Band) {
    setBand(next)
    setLocalBand(next)
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
