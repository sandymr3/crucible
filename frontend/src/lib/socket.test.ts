import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { decodeAudioFrame, AUDIO_SEQ_PREFIX_LEN } from './protocol'
import { LiveSocket } from './socket'

/** Builds a downstream audio frame the way the relay does. */
function audioFrame(seq: number, samples: number[]): ArrayBuffer {
  const buffer = new ArrayBuffer(AUDIO_SEQ_PREFIX_LEN + samples.length * 2)
  new DataView(buffer).setUint32(0, seq, false) // big-endian
  new Int16Array(buffer, AUDIO_SEQ_PREFIX_LEN).set(samples)
  return buffer
}

const sockets: FakeWebSocket[] = []

class FakeWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3

  readyState = FakeWebSocket.CONNECTING
  binaryType = ''
  sent: unknown[] = []
  closedWith: { code?: number; reason?: string } | null = null

  onopen: (() => void) | null = null
  onmessage: ((event: { data: unknown }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null

  readonly url: string

  constructor(url: string) {
    this.url = url
    sockets.push(this)
  }

  send(data: unknown) {
    this.sent.push(data)
  }

  close(code?: number, reason?: string) {
    this.closedWith = { code, reason }
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.({ code, reason } as CloseEvent)
  }

  // — test drivers —
  open() {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.()
  }
  text(frame: unknown) {
    this.onmessage?.({ data: JSON.stringify(frame) })
  }
  raw(data: string) {
    this.onmessage?.({ data })
  }
  binary(buffer: ArrayBuffer) {
    this.onmessage?.({ data: buffer })
  }

  get jsonSent(): Record<string, unknown>[] {
    return this.sent
      .filter((s): s is string => typeof s === 'string')
      .map((s) => JSON.parse(s) as Record<string, unknown>)
  }
  get binarySent(): ArrayBuffer[] {
    return this.sent.filter((s): s is ArrayBuffer => s instanceof ArrayBuffer)
  }
}

beforeEach(() => {
  sockets.length = 0
  vi.stubGlobal('WebSocket', FakeWebSocket)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

/** Connects and drives the socket to the point where audio is permitted. */
function connected(handlers = {}) {
  const socket = new LiveSocket('ws://test/v1/sessions/s1/live?token=t', handlers)
  socket.connect()
  const ws = sockets[0]
  ws.open()
  return { socket, ws }
}

function listening(handlers = {}) {
  const ctx = connected(handlers)
  ctx.ws.text({ type: 'state', state: 'LISTENING' })
  return ctx
}

describe('decodeAudioFrame', () => {
  it('reads the sequence number BIG-endian', () => {
    // Little-endian would read 0x01000000 and every gap check would be nonsense.
    const decoded = decodeAudioFrame(audioFrame(1, [0]))
    expect(decoded?.seq).toBe(1)
    expect(decodeAudioFrame(audioFrame(258, [0]))?.seq).toBe(258)
  })

  it('returns the PCM payload after the 4-byte prefix', () => {
    const decoded = decodeAudioFrame(audioFrame(7, [-32768, -1, 0, 1, 32767]))
    expect(Array.from(decoded!.pcm)).toEqual([-32768, -1, 0, 1, 32767])
  })

  it('rejects a frame too short to hold a sequence number', () => {
    expect(decodeAudioFrame(new ArrayBuffer(3))).toBeNull()
  })

  it('accepts a header-only frame as an empty chunk', () => {
    expect(decodeAudioFrame(new ArrayBuffer(4))?.pcm.length).toBe(0)
  })
})

describe('the audio gate', () => {
  it('refuses audio before LISTENING has been seen', () => {
    // Streaming on connect costs ~800ms of turn-boundary latency, and the
    // failure is silent: everything works, it is just mysteriously slow.
    const { socket, ws } = connected()
    expect(socket.canSendAudio).toBe(false)
    expect(socket.sendAudio(new Int16Array(320))).toBe(false)
    expect(ws.binarySent).toHaveLength(0)
  })

  it('allows audio once LISTENING arrives', () => {
    const { socket, ws } = listening()
    expect(socket.sendAudio(new Int16Array(320))).toBe(true)
    expect(ws.binarySent).toHaveLength(1)
    expect(ws.binarySent[0].byteLength).toBe(640) // 20 ms at 16 kHz
  })

  it('keeps allowing audio after the state moves on', () => {
    // The gate is "has ever been ready", not "is currently listening" — the
    // tail of an answer still has to reach Vertex after CLOSING.
    const { socket, ws } = listening()
    ws.text({ type: 'state', state: 'CLOSING' })
    expect(socket.sendAudio(new Int16Array(320))).toBe(true)
  })

  it('drops activity signals sent before LISTENING', () => {
    // An activity_start that arrives before the Vertex session exists is lost.
    const { socket, ws } = connected()
    socket.signal('activity_start')
    expect(ws.jsonSent.filter((f) => f.type === 'activity_start')).toHaveLength(0)
  })

  it('lets `begin` through, since it is what starts the interview', () => {
    const { socket, ws } = listening()
    socket.signal('begin')
    expect(ws.jsonSent).toContainEqual({ type: 'begin' })
  })
})

describe('sequence tracking', () => {
  it('counts no gap for a contiguous run', () => {
    const { socket, ws } = listening()
    for (let seq = 1; seq <= 5; seq++) ws.binary(audioFrame(seq, [0, 0]))
    expect(socket.getStats()).toMatchObject({ audioFramesReceived: 5, sequenceGaps: 0, lastSeq: 5 })
  })

  it('does not count the first frame as a gap from zero', () => {
    // Relay sequence numbers start at 1.
    const { socket, ws } = listening()
    ws.binary(audioFrame(1, [0]))
    expect(socket.getStats().sequenceGaps).toBe(0)
  })

  it('counts every missing frame, not just the event', () => {
    const { socket, ws } = listening()
    ws.binary(audioFrame(1, [0]))
    ws.binary(audioFrame(5, [0])) // 2, 3 and 4 never arrived
    expect(socket.getStats().sequenceGaps).toBe(3)
  })

  it('ignores a reordered frame rather than reporting a negative gap', () => {
    const { socket, ws } = listening()
    ws.binary(audioFrame(5, [0]))
    ws.binary(audioFrame(4, [0]))
    expect(socket.getStats().sequenceGaps).toBe(0)
  })

  it('accumulates received bytes', () => {
    const { socket, ws } = listening()
    ws.binary(audioFrame(1, new Array(240).fill(0)))
    expect(socket.getStats().audioBytesReceived).toBe(4 + 480)
  })
})

describe('frame handling', () => {
  it('forwards parsed frames', () => {
    const onFrame = vi.fn()
    const { ws } = connected({ onFrame })
    ws.text({ type: 'transcript', side: 'user', text: 'a bloom filter', final: true })
    expect(onFrame).toHaveBeenCalledWith({
      type: 'transcript',
      side: 'user',
      text: 'a bloom filter',
      final: true,
    })
  })

  it('survives an unparseable frame rather than killing the session', () => {
    const onFrame = vi.fn()
    const { ws } = connected({ onFrame })
    ws.raw('not json at all')
    ws.text({ type: 'turn_complete' })
    expect(onFrame).toHaveBeenCalledTimes(1)
  })

  it('records pong round-trip latency', () => {
    const { socket, ws } = connected()
    ws.text({ type: 'pong', t: Date.now() - 42 })
    expect(socket.getStats().latencyMs).toBeGreaterThanOrEqual(42)
  })

  it('leaves latency null when a pong carries no timestamp', () => {
    // `t` is omitempty, so a zero timestamp arrives as no key at all.
    const { socket, ws } = connected()
    ws.text({ type: 'pong' })
    expect(socket.getStats().latencyMs).toBeNull()
  })
})

describe('keepalive', () => {
  it('pings every 20s, because Cloud Run closes idle connections', () => {
    vi.useFakeTimers()
    const { ws } = connected()
    expect(ws.jsonSent.filter((f) => f.type === 'ping')).toHaveLength(0)

    vi.advanceTimersByTime(20_000)
    vi.advanceTimersByTime(20_000)
    expect(ws.jsonSent.filter((f) => f.type === 'ping')).toHaveLength(2)
  })

  it('stops pinging once closed', () => {
    vi.useFakeTimers()
    const { socket, ws } = connected()
    socket.close()
    vi.advanceTimersByTime(60_000)
    expect(ws.jsonSent.filter((f) => f.type === 'ping')).toHaveLength(0)
  })
})

describe('shutdown', () => {
  it('tells the relay before closing, so Vertex is torn down promptly', () => {
    const { socket, ws } = listening()
    socket.close()
    expect(ws.jsonSent).toContainEqual({ type: 'end_session' })
    expect(ws.closedWith?.code).toBe(1000)
  })

  it('reports the close event', () => {
    const onClose = vi.fn()
    const { ws } = connected({ onClose })
    ws.close(1006, 'abnormal')
    expect(onClose).toHaveBeenCalled()
  })
})

describe('text answers', () => {
  it('sends the accessibility and fallback path', () => {
    // text_answer travels the identical downstream path as speech, which is
    // what makes it a real degradation target when the socket drops.
    const { socket, ws } = listening()
    socket.sendTextAnswer('We used a bounded queue.')
    expect(ws.jsonSent).toContainEqual({
      type: 'text_answer',
      text: 'We used a bounded queue.',
    })
  })
})
