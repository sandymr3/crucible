import { useEffect, useState } from 'react'

import { PersonaMark } from '../../components/persona/PersonaMark'
import { Label } from '../../components/primitives'
import * as api from '../../lib/api'
import { PERSONA_FALLBACK_NAME, PERSONA_IDS, personaAccent, type PersonaId } from '../../lib/persona'
import type { PersonaCard } from '../../lib/types'
import s from './Setup.module.css'

const WEIGHT_LABELS: [keyof PersonaCard['weights'], string][] = [
  ['technicalAccuracy', 'Accuracy'],
  ['depth', 'Depth'],
  ['structure', 'Structure'],
  ['communicationClarity', 'Clarity'],
]

/**
 * The three interviewers.
 *
 * ⚠️ This sits BEFORE session creation, not after the digest, and that is
 * forced by the backend rather than chosen. The persona is written only by
 * POST /v1/sessions — no route updates it afterwards — and uploading a resume
 * requires a session to attach it to. So the choice has to precede the upload.
 * Putting it later would mean creating a throwaway session and burning one of
 * the five daily allocations to change one field.
 *
 * The rubric weights are shown because they are the substance of the choice:
 * they are what make the same answer score differently for a tech lead than for
 * a product manager, which is the difference between three characters and three
 * costumes.
 */
export function PersonaChooser({
  value,
  onChange,
}: {
  value: PersonaId
  onChange: (persona: PersonaId) => void
}) {
  const [cards, setCards] = useState<PersonaCard[]>([])

  useEffect(() => {
    api
      .listPersonas()
      .then(setCards)
      .catch(() => setCards([]))
  }, [])

  // The catalogue requires auth and can fail. The three interviewers are fixed,
  // so falling back to their names beats an empty screen.
  const shown: PersonaCard[] =
    cards.length > 0
      ? cards
      : PERSONA_IDS.map((persona) => ({
          id: persona,
          name: PERSONA_FALLBACK_NAME[persona],
          tagline: '',
          punishes: '',
          weights: {
            technicalAccuracy: 0,
            communicationClarity: 0,
            depth: 0,
            structure: 0,
          },
        }))

  return (
    <div className={s.personaGrid}>
      {shown.map((card) => (
        <button
          key={card.id}
          type="button"
          onClick={() => onChange(card.id)}
          aria-pressed={card.id === value}
          className={`${s.persona} ${card.id === value ? s.personaSelected : ''}`}
          style={{ ['--persona-accent' as string]: personaAccent(card.id) }}
        >
          <PersonaMark persona={card.id} size={72} />
          <Label className={s.personaName}>{card.name.toUpperCase()}</Label>
          {card.tagline && <p className={s.personaTagline}>{card.tagline}</p>}

          {card.punishes && (
            <div className={s.personaPunishes}>
              {/* The field that makes someone pick the one that scares them,
                  so it gets real weight rather than being buried. */}
              <Label className={s.punishesLabel}>Punishes</Label>
              <p className={s.punishesBody}>{card.punishes}</p>
            </div>
          )}

          {card.weights.technicalAccuracy > 0 && (
            <div className={s.weights}>
              {WEIGHT_LABELS.map(([key, label]) => (
                <div className={s.weightRow} key={key}>
                  <div className={s.weightTrack}>
                    <div
                      className={s.weightFill}
                      style={{ width: `${card.weights[key] * 100}%` }}
                    />
                  </div>
                  <Label tone="quiet">{label}</Label>
                </div>
              ))}
            </div>
          )}
        </button>
      ))}
    </div>
  )
}
