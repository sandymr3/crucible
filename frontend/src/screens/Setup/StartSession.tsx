import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, AudioLines, BookOpen, PlayCircle } from 'lucide-react'

import { Button, Label } from '../../components/primitives'
import * as api from '../../lib/api'
import type { PersonaId } from '../../lib/persona'
import type { Mode } from '../../lib/types'
import { useAuth } from '../../store/auth'
import { PersonaChooser } from './PersonaChooser'
import s from './Setup.module.css'
import { SetupShell } from './SetupShell'

const MODES: {
  mode: Mode
  accent: string
  icon: typeof AudioLines
  name: string
  body: string
}[] = [
  {
    mode: 'interview',
    accent: 'var(--t-ember)',
    icon: AudioLines,
    name: 'Interview',
    body: 'Upload a resume and the job you want. Ten minutes, out loud, with an interviewer that follows up.',
  },
  {
    mode: 'study',
    accent: 'var(--t-cool)',
    icon: BookOpen,
    name: 'Study a topic',
    body: 'Name a topic. It decomposes into a dependency-ordered syllabus and drills you until you can teach it back.',
  },
  {
    mode: 'replay',
    accent: 'var(--t-quench)',
    icon: PlayCircle,
    name: 'Watch a recording',
    body: 'A real recorded session, played back exactly as it happened. Costs nothing and needs no microphone.',
  },
]

export default function StartSession() {
  const navigate = useNavigate()
  const auth = useAuth()
  const [mode, setMode] = useState<Mode>('interview')
  const [persona, setPersona] = useState<PersonaId>('tech_lead')
  const [topic, setTopic] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function create() {
    setBusy(true)
    setError(null)
    try {
      if (!auth.user && auth.configured) await auth.signInGuest()

      const session = await api.createSession({
        mode,
        // Set HERE and nowhere else: no backend route updates a session's
        // persona afterwards, so this is the only moment it can be chosen.
        ...(mode === 'interview' ? { persona } : {}),
        ...(mode === 'study' ? { topic: topic.trim() } : {}),
        // The one recorded session the backend ships.
        ...(mode === 'replay' ? { fixtureId: 'demo-ml-engineer' } : {}),
      })

      if (mode === 'replay') navigate(`/room/${session.id}`)
      else if (mode === 'study') navigate(`/study/${session.id}`)
      else navigate(`/setup/${session.id}`)
    } catch (err) {
      setError(
        err instanceof api.ApiError
          ? err.isDailyCap
            ? 'You have used all of today’s sessions. Watching a recording is always free.'
            : err.message
          : String(err),
      )
      setBusy(false)
    }
  }

  const ready = mode !== 'study' || topic.trim().length > 1

  return (
    <SetupShell
      step="mode"
      title="What are we doing?"
      lede="Pick how you want to be tested, and who is testing you."
      wide
    >
      <div className={s.modes}>
        {MODES.map((entry) => {
          const Icon = entry.icon
          return (
            <button
              key={entry.mode}
              type="button"
              onClick={() => setMode(entry.mode)}
              aria-pressed={mode === entry.mode}
              className={`${s.mode} ${mode === entry.mode ? s.modeSelected : ''}`}
              style={{ ['--mode-accent' as string]: entry.accent }}
            >
              <Icon size={20} strokeWidth={1.5} style={{ color: entry.accent }} />
              <span className={s.modeName}>{entry.name}</span>
              <span className={s.modeBody}>{entry.body}</span>
            </button>
          )
        })}
      </div>

      {mode === 'study' && (
        <div>
          <Label tone="quiet">Topic</Label>
          <input
            className={s.textarea}
            style={{ minHeight: 'auto', height: 48 }}
            value={topic}
            onChange={(event) => setTopic(event.target.value)}
            placeholder="Transformer attention, CAP theorem, TCP congestion control…"
          />
        </div>
      )}

      {mode === 'interview' && (
        <>
          <div>
            <h2 className={s.modeName}>Who is in the room?</h2>
            <p className={s.lede}>
              Each one weighs your answer differently. Pick the one that scares you —
              that is the one worth rehearsing against.
            </p>
          </div>
          <PersonaChooser value={persona} onChange={setPersona} />
        </>
      )}

      {error && <p className={s.error}>{error}</p>}

      <div className={s.actions}>
        <span className={s.spacer} />
        <Button
          variant="primary"
          size="hero"
          onClick={create}
          disabled={busy || !ready}
          icon={<ArrowRight size={20} strokeWidth={1.5} />}
        >
          {busy ? 'Setting up…' : 'Continue'}
        </Button>
      </div>
    </SetupShell>
  )
}
