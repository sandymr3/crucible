import { Route, Routes } from 'react-router-dom'

/**
 * Route map from the screen spec. Screens land one per build step; until then a
 * route renders a marker so the shape of the app is visible and navigable.
 */
function Placeholder({ name }: { name: string }) {
  return <main style={{ padding: 32 }}>{name}</main>
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Placeholder name="Home" />} />
      <Route path="/setup/:id" element={<Placeholder name="Setup" />} />
      <Route path="/setup/:id/digest" element={<Placeholder name="Digest" />} />
      <Route path="/setup/:id/persona" element={<Placeholder name="Persona" />} />
      <Route path="/setup/:id/plan" element={<Placeholder name="Plan" />} />
      <Route path="/room/:id" element={<Placeholder name="Live Room" />} />
      <Route path="/report/:id" element={<Placeholder name="Report" />} />
      <Route path="/roadmap/:id" element={<Placeholder name="Roadmap" />} />
      <Route path="/study/:id" element={<Placeholder name="Study" />} />
      <Route path="/history" element={<Placeholder name="History" />} />
      <Route path="*" element={<Placeholder name="Not found" />} />
    </Routes>
  )
}
