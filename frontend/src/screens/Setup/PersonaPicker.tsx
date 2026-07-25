import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowRight } from 'lucide-react'

import { PersonaMark } from '../../components/persona/PersonaMark'
import { Button, Label } from '../../components/primitives'
import * as api from '../../lib/api'
import { PERSONA_FALLBACK_NAME, PERSONA_IDS, personaAccent, type PersonaId } from '../../lib/persona'
import type { PersonaCard } from '../../lib/types'
import s from './Setup.module.css'
import { SetupShell } from './SetupShell'

const WEIGHT_LABELS: [keyof PersonaCard['weights'], string][] = [
  ['technicalAccuracy', 'Accuracy'],
  ['depth', 'Depth'],
  ['structure', 'Structure'],
  ['communicationClarity', 'Clarity'],
]

export default function PersonaPicker() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [cards, setCards] = useState<PersonaCard[]>([])
  const [chosen, setChosen] = useState<PersonaId>('tech_lead')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .listPersonas()
      .then(setCards)
      .catch(() => setCards([]))
  }, [])

  useEffect(() => {
    if (!id) return
    api
      .getSession(id)
      .then((session) => session.persona && setChosen(session.persona))
      .catch(() => {})
  }, [id])

  /**
   * The persona is not written by its own endpoint — the backend takes it at
   * session creation. It reaches the interview through the plan step, which is
   * the last thing to run before the room.
   */
  function next() {
    if (!id) return
    setError(null)
    navigate(`/setup/${id}/plan?persona=${chosen}`)
  }

  const shown: PersonaCard[] =
    cards.length > 0
      ? cards
      : // The catalogue requires auth and can fail; the three interviewers are
        // fixed, so falling back to their names beats an empty screen.
        PERSONA_IDS.map((persona) => ({
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
    <SetupShell
      step="persona"
      title="Who is in the room?"
      lede="Each one weighs your answer differently. Pick the one that scares you — that is the one worth rehearsing against."
      wide
    >
      <div className={s.personaGrid}>
        {shown.map((card) => (
          <button
            key={card.id}
            type="button"
            onClick={() => setChosen(card.id)}
            aria-pressed={card.id === chosen}
            className={`${s.persona} ${card.id === chosen ? s.personaSelected : ''}`}
            style={{ ['--persona-accent' as string]: personaAccent(card.id) }}
          >
            <PersonaMark persona={card.id} size={72} />
            <Label className={s.personaName}>{card.name.toUpperCase()}</Label>
            {card.tagline && <p className={s.personaTagline}>{card.tagline}</p>}

            {card.punishes && (
              <div className={s.personaPunishes}>
                {/* The field that makes someone pick the one that scares them,
                    so it is given real weight rather than buried. */}
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

      {error && <p className={s.error}>{error}</p>}

      <div className={s.actions}>
        <span className={s.spacer} />
        <Button
          variant="primary"
          size="hero"
          onClick={next}
          icon={<ArrowRight size={20} strokeWidth={1.5} />}
        >
          Continue
        </Button>
      </div>
    </SetupShell>
  )
}
