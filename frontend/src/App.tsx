import { Route, Routes } from 'react-router-dom'

import { ToastHost } from './components/primitives'
import { ThermalField } from './components/thermal/ThermalField'
import Dev from './screens/Dev/Dev'
import LiveRoom from './screens/LiveRoom/LiveRoom'

/**
 * Route map from the screen spec. Screens land one per build step; until then a
 * route renders a marker so the shape of the app is visible and navigable.
 */
function Placeholder({ name }: { name: string }) {
  return <main style={{ padding: 'var(--s-8)' }}>{name}</main>
}

export default function App() {
  return (
    <>
      <ThermalField />
      {/* Content sits above the fixed field. #root isolates the stacking
          context, so this is the only z-index the app needs. */}
      <div style={{ position: 'relative', zIndex: 1 }}>
        <Routes>
          <Route path="/" element={<Placeholder name="Home" />} />
          <Route path="/setup/:id" element={<Placeholder name="Setup" />} />
          <Route path="/setup/:id/digest" element={<Placeholder name="Digest" />} />
          <Route path="/setup/:id/persona" element={<Placeholder name="Persona" />} />
          <Route path="/setup/:id/plan" element={<Placeholder name="Plan" />} />
          <Route path="/room/:id" element={<LiveRoom />} />
          <Route path="/report/:id" element={<Placeholder name="Report" />} />
          <Route path="/roadmap/:id" element={<Placeholder name="Roadmap" />} />
          <Route path="/study/:id" element={<Placeholder name="Study" />} />
          <Route path="/history" element={<Placeholder name="History" />} />
          {import.meta.env.DEV && <Route path="/dev" element={<Dev />} />}
          <Route path="*" element={<Placeholder name="Not found" />} />
        </Routes>
      </div>
      <ToastHost />
    </>
  )
}
