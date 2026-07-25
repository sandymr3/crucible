import { useEffect, useState, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ArrowRight, AudioLines, FileText, Route } from 'lucide-react'

import { BandSparkline, Radar } from '../../components/charts'
import { PersonaMark } from '../../components/persona/PersonaMark'
import { Button, Label, Panel } from '../../components/primitives'
import { VerdictSpan } from '../../components/verdict'
import { BAND_NAMES } from '../../lib/band'
import { VERDICT_EXAMPLES } from '../../lib/fixtures'
import { PERSONA_FALLBACK_NAME, PERSONA_IDS, personaAccent } from '../../lib/persona'
import { useInView } from '../../lib/useInView'
import { VERDICTS, VERDICT_DEFINITION, VERDICT_NAME, verdictColor } from '../../lib/verdict'
import { useAuth } from '../../store/auth'
import { HeroDemo } from './HeroDemo'
import s from './Home.module.css'

/**
 * The home page.
 *
 * Every string here was read aloud and rewritten if it sounded like marketing.
 * "Ten minutes, out loud, before someone else asks" is a sentence a person
 * wrote; "revolutionise your interview prep with cutting-edge AI" is not.
 */

/** Fades up once on entry. The only scroll animation on the page. */
function Reveal({ children, delay = 0 }: { children: ReactNode; delay?: number }) {
  const { ref, inView } = useInView<HTMLDivElement>()
  return (
    <div
      ref={ref}
      className={`${s.reveal} ${inView ? s.revealIn : ''}`}
      style={{ transitionDelay: `${delay}ms` }}
    >
      {children}
    </div>
  )
}

/**
 * Numbered because this genuinely IS a sequence in time — before, during,
 * after. The personas are alternatives rather than steps, so they are not
 * numbered.
 *
 * Left to right the accents get hotter: the ladder metaphor, restated quietly.
 */
const STEPS = [
  {
    n: '01',
    accent: 'var(--t-quench)',
    icon: <FileText size={20} strokeWidth={1.5} />,
    title: 'Configure',
    body: 'Upload your resume and paste the job you want. It reads both and finds the claims you would struggle to defend.',
    footer: 'RESUME · JD · PLAN',
  },
  {
    n: '02',
    accent: 'var(--t-warm)',
    icon: <AudioLines size={20} strokeWidth={1.5} />,
    title: 'Interrogate',
    body: 'The AI speaks. You answer out loud. It follows up on the vague parts, and gets harder when you are doing well.',
    footer: 'VOICE · ADAPTIVE',
  },
  {
    n: '03',
    accent: 'var(--t-ember)',
    icon: <Route size={20} strokeWidth={1.5} />,
    title: 'Close the gap',
    body: 'A report scored on accuracy, clarity, depth and structure — then a day-by-day plan ordered by what you need first.',
    footer: 'SCORED · GROUNDED',
  },
]

const PERSONA_QUOTES: Record<string, string> = {
  tech_lead: '“And what happens when that fails?”',
  architect: '“You picked X. Argue for Y, then tell me why you still prefer X.”',
  pm: '“Explain that to a customer who is angry and non-technical.”',
}

const PERSONA_PUNISHES: Record<string, string> = {
  tech_lead: 'Hand-waving. Buzzwords with no mechanism.',
  architect: 'Premature detail. Unexamined defaults.',
  pm: 'Jargon with no translation. A story with no user in it.',
}

const LADDER = [
  { band: 1, color: 'var(--t-quench)', desc: 'What is it' },
  { band: 2, color: 'var(--t-cool)', desc: 'When would you use it' },
  { band: 3, color: 'var(--t-temper)', desc: 'How does it actually work' },
  { band: 4, color: 'var(--t-warm)', desc: 'Argue against yourself' },
  { band: 5, color: 'var(--t-ember)', desc: 'Underspecified on purpose' },
]

const BUILT_ON = [
  { name: 'Vertex AI', detail: 'Gemini Live API — native audio, speech to speech' },
  { name: 'Go', detail: 'Cloud Run WebSocket relay, 125 tests' },
  { name: 'React', detail: 'AudioWorklet — 16 kHz capture, 24 kHz playback' },
]

export default function Home() {
  const navigate = useNavigate()
  const auth = useAuth()
  const [scrolled, setScrolled] = useState(false)

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 40)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  /**
   * One click to start. A candidate should not have to hand over an identity to
   * rehearse an interview, so an unauthenticated visitor is signed in as a
   * guest — the backend treats an anonymous session identically.
   */
  async function begin() {
    if (!auth.user && auth.configured) await auth.signInGuest()
    navigate('/setup')
  }

  return (
    <div className={s.page}>
      <nav className={`${s.nav} ${scrolled ? s.navScrolled : ''}`}>
        <div className={s.navInner}>
          <Link to="/" className={s.wordmark}>
            <span className={s.mark} aria-hidden="true" />
            CRUCIBLE
          </Link>
          <div className={s.navRight}>
            <Button variant="ghost" size="compact" onClick={() => scrollTo('how-it-works')}>
              How it works
            </Button>
            {auth.user ? (
              <Button variant="ghost" size="compact" onClick={() => navigate('/history')}>
                My sessions
              </Button>
            ) : (
              <Button variant="ghost" size="compact" onClick={() => auth.signInGoogle()}>
                Sign in
              </Button>
            )}
          </div>
        </div>
      </nav>

      {/* ── hero ─────────────────────────────────────────────────────── */}
      <header className={`${s.shell} ${s.hero}`}>
        <div className={s.heroGrid}>
          <div className={s.heroCopy}>
            <Label tone="quiet" className={s.eyebrow}>
              Voice-native · Adaptive
            </Label>
            <h1 className={s.headline}>
              Most interview prep is a quiz. This one talks back.
            </h1>
            <p className={s.subcopy}>
              Upload your resume and the job you are actually chasing. Pick who grills
              you. Then talk — out loud, in real time. Your answer gets graded sentence
              by sentence.
            </p>
            <div className={s.ctaRow}>
              <Button
                variant="primary"
                size="hero"
                onClick={begin}
                icon={<ArrowRight size={20} strokeWidth={1.5} />}
              >
                Start a session
              </Button>
              <span className={s.ctaNote}>Free · No card · About ten minutes</span>
            </div>
          </div>

          <div className={s.heroDemo}>
            <HeroDemo />
          </div>
        </div>
      </header>

      {/* ── how it works ─────────────────────────────────────────────── */}
      <section className={`${s.shell} ${s.section}`} id="how-it-works">
        <h2 className={s.sectionHead}>How it works</h2>
        <div className={s.cards3}>
          {STEPS.map((step, i) => (
            <Reveal key={step.n} delay={i * 90}>
              <article
                className={s.card}
                style={{ ['--card-accent' as string]: step.accent, height: '100%' }}
              >
                <span className={s.cardNumber}>{step.n}</span>
                <div className={s.cardIcon}>{step.icon}</div>
                <h3 className={s.cardTitle}>{step.title}</h3>
                <p className={s.cardBody}>{step.body}</p>
                <div className={s.cardFooter}>
                  <Label tone="quiet">{step.footer}</Label>
                </div>
              </article>
            </Reveal>
          ))}
        </div>
      </section>

      {/* ── the panel ────────────────────────────────────────────────── */}
      <section className={`${s.shell} ${s.section}`}>
        <h2 className={s.sectionHead}>The panel</h2>
        <p className={s.sectionLede}>
          You pick who is in the room. Each one grades you differently — the same
          answer scores differently for a tech lead than for a product manager.
        </p>
        <div className={s.cards3}>
          {PERSONA_IDS.map((persona, i) => (
            <Reveal key={persona} delay={i * 90}>
              <article
                className={`${s.card} ${s.personaCard}`}
                style={{ ['--card-accent' as string]: personaAccent(persona), height: '100%' }}
              >
                <PersonaMark persona={persona} />
                <Label className={s.personaName}>
                  {PERSONA_FALLBACK_NAME[persona].toUpperCase()}
                </Label>
                <p className={s.personaQuote}>{PERSONA_QUOTES[persona]}</p>
                <div className={s.punishes}>
                  <Label className={s.punishesLabel}>Punishes</Label>
                  <p className={s.punishesBody}>{PERSONA_PUNISHES[persona]}</p>
                </div>
              </article>
            </Reveal>
          ))}
        </div>
      </section>

      {/* ── verdict scale ────────────────────────────────────────────── */}
      <section className={`${s.shell} ${s.section}`}>
        <h2 className={s.sectionHead}>Four verdicts, not two</h2>
        <p className={s.sectionLede}>
          Most of what looks wrong in an interview answer is not falsehood — it is a
          claim with nothing behind it. Those are different problems, and they get
          different colours.
        </p>
        <div className={s.cards4}>
          {VERDICTS.map((verdict, i) => (
            <Reveal key={verdict} delay={i * 90}>
              <article
                className={s.verdictCard}
                style={{ ['--card-accent' as string]: verdictColor(verdict), height: '100%' }}
              >
                <span className={s.verdictRule} aria-hidden="true" />
                <Label className={s.verdictName}>{VERDICT_NAME[verdict].toUpperCase()}</Label>
                <p className={s.verdictDef}>{VERDICT_DEFINITION[verdict]}</p>
                <p className={s.verdictExample}>
                  {/* A real span with the real treatment, not a mock-up of one. */}
                  <VerdictSpan verdict={verdict} concept={VERDICT_NAME[verdict]}>
                    {VERDICT_EXAMPLES[verdict]}
                  </VerdictSpan>
                </p>
              </article>
            </Reveal>
          ))}
        </div>
      </section>

      {/* ── heat ladder ──────────────────────────────────────────────── */}
      <section className={`${s.shell} ${s.section}`}>
        <h2 className={s.sectionHead}>It gets harder when you are doing well</h2>
        <p className={s.ladderQuote}>
          Two good answers and it moves up. Two weak ones and it backs off. The
          difficulty is not a setting you chose at the start — it is a response to how
          you are actually doing, and you can watch it move.
        </p>
        <div className={s.ladder}>
          {LADDER.map((rung, i) => (
            <Reveal key={rung.band} delay={i * 90}>
              <div className={s.ladderSegment}>
                <Label tone="quiet">Band {rung.band}</Label>
                <div
                  className={s.ladderBar}
                  style={{ ['--ladder-color' as string]: rung.color }}
                />
                <div>
                  <Label>{BAND_NAMES[rung.band as 1 | 2 | 3 | 4 | 5]}</Label>
                  <p className={s.ladderDesc}>{rung.desc}</p>
                </div>
              </div>
            </Reveal>
          ))}
        </div>
      </section>

      {/* ── what you get back ────────────────────────────────────────── */}
      <section className={`${s.shell} ${s.section}`}>
        <h2 className={s.sectionHead}>What you get back</h2>
        <div className={s.getBack}>
          <Reveal>
            <Panel title="The report" style={{ height: '100%' }}>
              <div className={s.previewGrid}>
                <Radar
                  axes={[
                    { label: 'Accuracy', value: 7.4 },
                    { label: 'Depth', value: 5.1 },
                    { label: 'Structure', value: 8.2 },
                    { label: 'Clarity', value: 6.3 },
                  ]}
                  turnsGraded={5}
                  size={190}
                />
                <div className={s.previewPanel}>
                  <div>
                    <Label tone="quiet">Difficulty</Label>
                    <BandSparkline trajectory={[3, 3, 4, 4, 5]} height={48} />
                  </div>
                  <p className={s.cardBody}>
                    Scored on the four things an interviewer is actually weighing, with
                    every sentence you said marked up underneath.
                  </p>
                </div>
              </div>
            </Panel>
          </Reveal>

          <Reveal delay={90}>
            <Panel title="The roadmap" style={{ height: '100%' }}>
              <div className={s.roadmapDay}>
                <Label tone="quiet">Day 1 · 45 min</Label>
                <strong style={{ color: 'var(--bone)' }}>Backpressure and flow control</strong>
                <p className={s.verdictDef}>
                  You called a bigger buffer backpressure. Start where the two diverge.
                </p>
                <span className={s.roadmapLink}>kafka.apache.org/documentation →</span>
              </div>
              <p className={s.cardBody} style={{ marginTop: 'var(--s-4)' }}>
                Ordered by what you need first, not by what scored worst. Every link is
                fetched and checked before you see it.
              </p>
            </Panel>
          </Reveal>
        </div>
      </section>

      {/* ── built on ─────────────────────────────────────────────────── */}
      <section className={s.shell}>
        <div className={s.builtOn}>
          {BUILT_ON.map((item) => (
            <div className={s.builtOnItem} key={item.name}>
              <span className={s.builtOnName}>{item.name}</span>
              <span className={s.builtOnDetail}>{item.detail}</span>
            </div>
          ))}
        </div>
      </section>

      {/* ── close ────────────────────────────────────────────────────── */}
      <section className={`${s.shell} ${s.close}`}>
        <p className={s.closeLine}>Ten minutes, out loud, before someone else asks.</p>
        <Button
          variant="primary"
          size="hero"
          onClick={begin}
          icon={<ArrowRight size={20} strokeWidth={1.5} />}
        >
          Start a session
        </Button>
      </section>

      <footer className={`${s.shell} ${s.footer}`}>
        <span className={s.wordmark}>
          <span className={s.mark} aria-hidden="true" />
          CRUCIBLE
        </span>
        <div className={s.footerRight}>
          <a
            className={s.footerLink}
            href="https://github.com/sandymr3/crucible"
            target="_blank"
            rel="noopener noreferrer"
          >
            GitHub
          </a>
          <span className={s.footerLink}>InnovaHack · Gen AI</span>
        </div>
      </footer>
    </div>
  )
}

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
