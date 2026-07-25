import {
  decodeAudioFrame,
  type ClientFrame,
  type ClientFrameType,
  type ServerFrame,
} from './protocol'

/**
 * Cloud Run closes idle connections, and a demo that dies during a thoughtful
 * pause is a demo that dies. 20 s is comfortably inside any idle window while
 * costing almost nothing.
 */
const PING_INTERVAL_MS = 20_000

export interface SocketStats {
  audioFramesReceived: number
  audioBytesReceived: number
  /** Missing sequence numbers. The earliest warning of network trouble. */
  sequenceGaps: number
  lastSeq: number
  /** Round trip of the most recent ping, in ms. */
  latencyMs: number | null
}

export interface SocketHandlers {
  onFrame?: (frame: ServerFrame) => void
  onAudio?: (pcm: Int16Array, seq: number) => void
  onOpen?: () => void
  onClose?: (event: CloseEvent) => void
  onSocketError?: () => void
}

/**
 * The interview socket.
 *
 * Deliberately dumb about product state — it moves frames and counts what
 * arrives. The one exception is the audio gate below, which is enforced here
 * because getting it wrong is expensive and invisible.
 */
export class LiveSocket {
  private ws: WebSocket | null = null
  private pingTimer: ReturnType<typeof setInterval> | null = null
  private handlers: SocketHandlers
  private readonly url: string

  /**
   * The relay upgrades in milliseconds but the Vertex session behind it takes
   * roughly two seconds. A client that streams on connect fills the socket
   * buffer during that window; the relay then drains it in a burst, and Vertex
   * is still ingesting when the turn closes — so the delay lands entirely on
   * turn-boundary latency, the one number a judge can feel. Measured at 1.8x
   * real-time upload and about 800 ms of added latency.
   *
   * Enforced here rather than trusted to callers, because the failure is
   * silent: everything works, it is just mysteriously slow.
   */
  private sawListening = false

  private stats: SocketStats = {
    audioFramesReceived: 0,
    audioBytesReceived: 0,
    sequenceGaps: 0,
    lastSeq: 0,
    latencyMs: null,
  }

  constructor(url: string, handlers: SocketHandlers = {}) {
    this.url = url
    this.handlers = handlers
  }

  get isOpen(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  get canSendAudio(): boolean {
    return this.isOpen && this.sawListening
  }

  getStats(): SocketStats {
    return { ...this.stats }
  }

  connect(): void {
    if (this.ws) return

    const ws = new WebSocket(this.url)
    ws.binaryType = 'arraybuffer'
    this.ws = ws

    ws.onopen = () => {
      this.pingTimer = setInterval(() => {
        this.send({ type: 'ping', t: Date.now() })
      }, PING_INTERVAL_MS)
      this.handlers.onOpen?.()
    }

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        this.receiveAudio(event.data)
        return
      }
      this.receiveText(event.data as string)
    }

    ws.onerror = () => this.handlers.onSocketError?.()

    ws.onclose = (event) => {
      this.clearPing()
      this.ws = null
      this.handlers.onClose?.(event)
    }
  }

  private receiveAudio(buffer: ArrayBuffer) {
    const decoded = decodeAudioFrame(buffer)
    if (!decoded) return

    const { seq, pcm } = decoded
    this.stats.audioFramesReceived++
    this.stats.audioBytesReceived += buffer.byteLength

    // Sequence numbers start at 1, so the first frame establishes the baseline
    // rather than being counted as a gap from zero.
    if (this.stats.lastSeq !== 0 && seq > this.stats.lastSeq + 1) {
      this.stats.sequenceGaps += seq - this.stats.lastSeq - 1
    }
    this.stats.lastSeq = seq

    this.handlers.onAudio?.(pcm, seq)
  }

  private receiveText(raw: string) {
    let frame: ServerFrame
    try {
      frame = JSON.parse(raw) as ServerFrame
    } catch {
      // A frame we cannot parse must not kill the session.
      return
    }

    if (frame.type === 'state' && frame.state === 'LISTENING') {
      this.sawListening = true
    }
    if (frame.type === 'pong' && typeof frame.t === 'number') {
      this.stats.latencyMs = Date.now() - frame.t
    }

    this.handlers.onFrame?.(frame)
  }

  /** Sends a control frame. Silently ignored when the socket is not open. */
  send(frame: ClientFrame): void {
    if (!this.isOpen) return
    this.ws?.send(JSON.stringify(frame))
  }

  /**
   * Sends one 20 ms PCM16 frame.
   *
   * Returns false when refused, which happens only before LISTENING has been
   * seen. Callers should pace at real time: Vertex ingests at approximately
   * real time regardless, so sending faster buys nothing and costs latency —
   * verified by blasting a file with no pacing, which made latency worse.
   */
  sendAudio(pcm: Int16Array): boolean {
    if (!this.canSendAudio) return false
    this.ws?.send(pcm.buffer as ArrayBuffer)
    return true
  }

  /** Convenience for the frames that carry no payload. */
  signal(type: Extract<ClientFrameType, 'begin' | 'activity_start' | 'activity_end' | 'request_hint' | 'end_session'>): void {
    // Activity signals are subject to the same gate as audio: an activity_start
    // that arrives before the Vertex session exists is simply lost.
    if ((type === 'activity_start' || type === 'activity_end') && !this.sawListening) return
    this.send({ type })
  }

  sendTextAnswer(text: string): void {
    this.send({ type: 'text_answer', text })
  }

  /**
   * Closes cleanly, telling the relay first so it can tear the Vertex session
   * down rather than waiting for a read error.
   */
  close(): void {
    this.clearPing()
    if (this.isOpen) {
      this.send({ type: 'end_session' })
      this.ws?.close(1000, 'client ended session')
    } else {
      this.ws?.close()
    }
    this.ws = null
  }

  private clearPing() {
    if (this.pingTimer) {
      clearInterval(this.pingTimer)
      this.pingTimer = null
    }
  }
}
