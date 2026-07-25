import { useCallback, useEffect, useRef, useState } from 'react'

import { Orb, useAmplitude } from '../../components/orb'
import { Button, Label, Panel } from '../../components/primitives'
import * as api from '../../lib/api'
import { AudioPipeline, type PlaybackStats } from '../../lib/audio'
import { getIdToken } from '../../lib/firebase'
import type { LiveState, ServerFrame } from '../../lib/protocol'
import { LiveSocket, type SocketStats } from '../../lib/socket'
import s from './Dev.module.css'

/**
 * The audio spike (FRONTEND-PRD §22, phase F1) — the highest-risk part of the
 * frontend, deliberately built before any real UI depends on it.
 *
 * It runs against a REPLAY session, which drives the identical protocol with
 * zero Vertex calls and no daily-cap consumption. So this can be re-run as many
 * times as it takes to get right, for free, and it needs no microphone: a
 * replay only plays.
 *
 * Exit criterion: the recorded interviewer is audible, transcripts arrive, and
 * sequence gaps stay at zero.
 */
export function AudioSpike() {
  const [running, setRunning] = useState(false)
  const [state, setState] = useState<LiveState | '—'>('—')
  const [log, setLog] = useState<string[]>([])
  const [socketStats, setSocketStats] = useState<SocketStats | null>(null)
  const [playbackStats, setPlaybackStats] = useState<PlaybackStats | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [transcript, setTranscript] = useState({ ai: '', user: '' })

  const pipelineRef = useRef<AudioPipeline | null>(null)
  const socketRef = useRef<LiveSocket | null>(null)
  const amplitude = useAmplitude({ gain: 3 })

  const note = useCallback((line: string) => {
    setLog((prev) => [...prev.slice(-14), line])
  }, [])

  const stop = useCallback(async () => {
    socketRef.current?.close()
    socketRef.current = null
    await pipelineRef.current?.stop()
    pipelineRef.current = null
    setRunning(false)
    setState('—')
  }, [])

  useEffect(() => () => void stop(), [stop])

  const { push } = amplitude

  async function run() {
    setError(null)
    setLog([])
    setTranscript({ ai: '', user: '' })
    setRunning(true)

    try {
      // Playback FIRST. `begin` must not be sent until the page can actually
      // play — the opening question is the strongest moment in the product and
      // must never be spoken into a page that cannot hear it.
      const pipeline = new AudioPipeline()
      pipelineRef.current = pipeline
      await pipeline.startPlayback()
      note('playback ready @ ' + pipeline.actualPlaybackRate + ' Hz')

      pipeline.onPlaybackStats((stats) => {
        setPlaybackStats(stats)
        push(stats.rms)
      })

      const session = await api.createSession({
        mode: 'replay',
        fixtureId: 'demo-ml-engineer',
      })
      note(`session ${session.id} (${session.mode})`)

      const token = await getIdToken()
      if (!token) throw new Error('Not signed in.')

      const socket = new LiveSocket(api.liveSocketUrl(session.id, token), {
        onOpen: () => note('socket open'),
        onFrame: (frame) => handleFrame(frame, socket, pipeline),
        // Straight from the socket into the ring buffer. React is not on this
        // path — a render between receiving a chunk and queueing it is exactly
        // how playback starts stuttering.
        onAudio: (pcm) => {
          pipeline.play(pcm)
          setSocketStats(socket.getStats())
        },
        onClose: (event) => {
          note(`socket closed (${event.code})`)
          setSocketStats(socket.getStats())
          setRunning(false)
        },
        onSocketError: () => note('socket error'),
      })
      socketRef.current = socket
      socket.connect()
    } catch (err) {
      const detail =
        err instanceof api.ApiError ? `${err.status} ${err.code}: ${err.message}` : String(err)
      setError(detail)
      note('✗ ' + detail)
      await stop()
    }
  }

  function handleFrame(frame: ServerFrame, socket: LiveSocket, pipeline: AudioPipeline) {
    switch (frame.type) {
      case 'state': {
        const next = frame.state as LiveState
        setState(next)
        note(`state → ${next}`)
        // The interviewer never speaks unprompted in manual activity mode: it
        // waits for a turn boundary that, at session start, has not happened.
        if (next === 'LISTENING') {
          socket.signal('begin')
          note('sent begin')
        }
        break
      }
      case 'transcript': {
        // `text` is a DELTA. Replacing instead of appending shows only the last
        // word — the most common integration bug against this protocol.
        const side = frame.side === 'ai' ? 'ai' : 'user'
        setTranscript((prev) => ({ ...prev, [side]: prev[side] + (frame.text ?? '') }))
        break
      }
      case 'interrupted':
        // Discard queued playback immediately; draining it would have the
        // model keep talking for two seconds after being cut off.
        pipeline.flush()
        note('interrupted → flushed')
        break
      case 'turn_complete':
        note('turn complete')
        break
      case 'error':
        // `recoverable` is omitempty: absent means false.
        note(`error ${frame.code} recoverable=${frame.recoverable === true}`)
        setError(frame.message ?? frame.code ?? 'error')
        break
      default:
        note(`frame: ${frame.type}`)
    }
  }

  const gapsOk = socketStats ? socketStats.sequenceGaps === 0 : null

  return (
    <Panel title="Audio spike · replay" aside={state}>
      <div className={s.controls}>
        <Button variant={running ? 'danger' : 'primary'} onClick={running ? stop : run}>
          {running ? 'Stop' : 'Run replay spike'}
        </Button>
        <Label tone="quiet">zero Vertex calls · no daily cap · no microphone</Label>
      </div>

      <div className={s.orbRow} style={{ marginTop: 'var(--s-5)' }}>
        <Orb ref={amplitude.ref} state={state === '—' ? 'CONNECTING' : state} size={96} />
        <div className={s.spikeStats}>
          <Stat label="audio frames" value={socketStats?.audioFramesReceived ?? 0} />
          <Stat
            label="sequence gaps"
            value={socketStats?.sequenceGaps ?? 0}
            tone={gapsOk === false ? 'bad' : 'good'}
          />
          <Stat label="bytes" value={socketStats?.audioBytesReceived ?? 0} />
          <Stat label="ping ms" value={socketStats?.latencyMs ?? '—'} />
          <Stat
            label="underruns"
            value={playbackStats?.underruns ?? 0}
            tone={(playbackStats?.underruns ?? 0) > 0 ? 'bad' : 'good'}
          />
          <Stat
            label="overruns"
            value={playbackStats?.overruns ?? 0}
            tone={(playbackStats?.overruns ?? 0) > 0 ? 'bad' : 'good'}
          />
          <Stat label="buffered" value={playbackStats?.buffered ?? 0} />
        </div>
      </div>

      {error && (
        <p className={s.spikeError}>{error}</p>
      )}

      {(transcript.ai || transcript.user) && (
        <div className={s.spikeTranscript}>
          {transcript.ai && (
            <p>
              <Label tone="quiet">interviewer</Label> {transcript.ai}
            </p>
          )}
          {transcript.user && (
            <p>
              <Label tone="quiet">candidate</Label> {transcript.user}
            </p>
          )}
        </div>
      )}

      {log.length > 0 && <pre className={s.probe}>{log.join('\n')}</pre>}
    </Panel>
  )
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string
  value: number | string
  tone?: 'good' | 'bad'
}) {
  return (
    <div className={s.stat} data-tone={tone}>
      <span className={s.statValue}>{value}</span>
      <Label tone="quiet">{label}</Label>
    </div>
  )
}
