import { Route, Routes } from 'react-router-dom'

import { ToastHost } from './components/primitives'
import { ThermalField } from './components/thermal/ThermalField'
import Dev from './screens/Dev/Dev'
import History from './screens/History/History'
import Home from './screens/Home/Home'
import LiveRoom from './screens/LiveRoom/LiveRoom'
import Study from './screens/Study/Study'
import Report from './screens/Report/Report'
import Roadmap from './screens/Roadmap/Roadmap'
import DigestReveal from './screens/Setup/DigestReveal'
import PlanEditor from './screens/Setup/PlanEditor'
import StartSession from './screens/Setup/StartSession'
import Upload from './screens/Setup/Upload'

/**
 * Route map from the screen spec. Screens land one per build step; until then a
 * route renders a marker so the shape of the app is visible and navigable.
 */
function Placeholder({ name }: { name: string }) {
  // A bare route marker reads as a broken page to anyone testing the app.
  // Say plainly that the screen is unbuilt, and offer the way back.
  return (
    <main style={{ padding: 'var(--s-8)', maxWidth: '32rem' }}>
      <h1 style={{ marginBottom: 'var(--s-3)' }}>{name}</h1>
      <p style={{ color: 'var(--ink-muted, #9da1b2)', marginBottom: 'var(--s-4)' }}>
        This screen is not built yet — the backend for it is complete, the UI is
        on its way. Nothing is broken.
      </p>
      <a href="/">← Back to home</a>
    </main>
  )
}

export default function App() {
  return (
    <>
      <ThermalField />
      {/* Content sits above the fixed field. #root isolates the stacking
          context, so this is the only z-index the app needs. */}
      <div style={{ position: 'relative', zIndex: 1 }}>
        <Routes>
          <Route path="/" element={<Home />} />
          {/* Persona is chosen at /setup rather than after the digest: the
              backend writes it only at session creation, and uploading a resume
              needs a session to attach it to. */}
          <Route path="/setup" element={<StartSession />} />
          <Route path="/setup/:id" element={<Upload />} />
          <Route path="/setup/:id/digest" element={<DigestReveal />} />
          <Route path="/setup/:id/plan" element={<PlanEditor />} />
          <Route path="/room/:id" element={<LiveRoom />} />
          <Route path="/report/:id" element={<Report />} />
          <Route path="/roadmap/:id" element={<Roadmap />} />
          <Route path="/study/:id" element={<Study />} />
          <Route path="/history" element={<History />} />
          {import.meta.env.DEV && <Route path="/dev" element={<Dev />} />}
          <Route path="*" element={<Placeholder name="Not found" />} />
        </Routes>
      </div>
      <ToastHost />
    </>
  )
}
