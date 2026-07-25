import { useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowRight, FileText, Upload as UploadIcon } from 'lucide-react'

import { Button, Label, Panel } from '../../components/primitives'
import * as api from '../../lib/api'
import { utf8ByteLength } from '../../lib/byteOffset'
import s from './Setup.module.css'
import { SetupShell } from './SetupShell'

/** The backend's ceiling, enforced there too. */
const MAX_JD_BYTES = 20_000
const MAX_RESUME_BYTES = 10 * 1024 * 1024

export default function Upload() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const input = useRef<HTMLInputElement>(null)

  const [file, setFile] = useState<File | null>(null)
  const [jd, setJd] = useState('')
  const [dragging, setDragging] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Counted in BYTES, matching the backend: its limit is len(text) in Go, so a
  // job description with accented characters reaches the ceiling sooner than a
  // character count would suggest.
  const jdBytes = utf8ByteLength(jd)
  const jdOver = jdBytes > MAX_JD_BYTES

  function choose(next: File | null) {
    setError(null)
    if (!next) return
    if (!next.name.toLowerCase().endsWith('.pdf') && next.type !== 'application/pdf') {
      setError('The resume must be a PDF.')
      return
    }
    if (next.size > MAX_RESUME_BYTES) {
      setError('That file is over 10 MB. The resume must be smaller.')
      return
    }
    setFile(next)
  }

  async function submit() {
    if (!id || !file) return
    setBusy(true)
    setError(null)
    try {
      await api.uploadResume(id, file)
      // Attached before the digest runs, because the digest reads both and its
      // whole job is to compare one against the other.
      if (jd.trim()) await api.attachJD(id, jd.trim())
      navigate(`/setup/${id}/digest`)
    } catch (err) {
      setError(err instanceof api.ApiError ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <SetupShell
      step="material"
      title="What are we working from?"
      lede="The resume is what it asks you about. The job description is what it holds you to."
    >
      <Panel title="Resume · PDF">
        <div
          className={`${s.dropzone} ${dragging ? s.dropzoneActive : ''}`}
          onClick={() => input.current?.click()}
          onDragOver={(event) => {
            event.preventDefault()
            setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(event) => {
            event.preventDefault()
            setDragging(false)
            choose(event.dataTransfer.files[0] ?? null)
          }}
          role="button"
          tabIndex={0}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') input.current?.click()
          }}
        >
          {file ? (
            <>
              <FileText size={22} strokeWidth={1.5} style={{ color: 'var(--heat-hot)' }} />
              <span className={s.dropzoneName}>{file.name}</span>
              <Label tone="quiet">{(file.size / 1024).toFixed(0)} KB · click to replace</Label>
            </>
          ) : (
            <>
              <UploadIcon size={22} strokeWidth={1.5} style={{ color: 'var(--ash)' }} />
              <span className={s.dropzoneName}>Drop a PDF, or click to choose</span>
              <Label tone="quiet">
                Text-based PDFs read best. A scan has nothing to extract.
              </Label>
            </>
          )}
        </div>
        <input
          ref={input}
          type="file"
          accept="application/pdf,.pdf"
          className={s.hiddenInput}
          onChange={(event) => choose(event.target.files?.[0] ?? null)}
        />
      </Panel>

      <Panel title="Job description" aside="optional">
        <textarea
          className={s.textarea}
          value={jd}
          onChange={(event) => setJd(event.target.value)}
          placeholder="Paste the posting. Requirements, responsibilities, the lot — it uses this to decide what you will actually be pressed on."
        />
        <Label tone="quiet" className={`${s.counter} ${jdOver ? s.counterWarn : ''}`}>
          {jdBytes.toLocaleString()} / {MAX_JD_BYTES.toLocaleString()} bytes
        </Label>
        {!jd.trim() && (
          <Label tone="quiet">
            Without one it infers a plausible target role from the resume itself.
          </Label>
        )}
      </Panel>

      {error && <p className={s.error}>{error}</p>}

      <div className={s.actions}>
        <span className={s.spacer} />
        <Button
          variant="primary"
          size="hero"
          onClick={submit}
          disabled={busy || !file || jdOver}
          icon={<ArrowRight size={20} strokeWidth={1.5} />}
        >
          {busy ? 'Uploading…' : 'Read my resume'}
        </Button>
      </div>
    </SetupShell>
  )
}
