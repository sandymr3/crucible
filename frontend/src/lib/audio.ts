/**
 * The audio pipeline.
 *
 * The Live API is asymmetric and it matters: it accepts 16 kHz mono PCM16 and
 * emits 24 kHz. Treating one as the other produces audio at the wrong speed
 * rather than failing loudly.
 *
 * Two AudioContexts, one per rate, so nothing is ever resampled by accident.
 * Capture and playback initialise independently — a replay session plays a
 * recording and needs no microphone at all, which makes it a verification path
 * that costs nothing and asks for no permissions.
 */

export const CAPTURE_RATE = 16000
export const PLAYBACK_RATE = 24000
export const FRAME_SAMPLES = 320 // 20 ms at 16 kHz
export const FRAME_BYTES = FRAME_SAMPLES * 2

export interface PlaybackStats {
  /** RMS of the samples actually emitted, 0..1. Drives the orb. */
  rms: number
  /** Samples queued and not yet played. */
  buffered: number
  /** Ran dry. The earliest measurable sign of a network problem. */
  underruns: number
  /** Ring full — receiving faster than playing. Should always be zero. */
  overruns: number
}

export interface CaptureFrame {
  pcm: Int16Array
  rms: number
}

export class AudioError extends Error {
  readonly cause?: unknown
  constructor(message: string, cause?: unknown) {
    super(message)
    this.name = 'AudioError'
    this.cause = cause
  }
}

type StatsHandler = (stats: PlaybackStats) => void
type FrameHandler = (frame: CaptureFrame) => void

export class AudioPipeline {
  private playbackCtx: AudioContext | null = null
  private playbackNode: AudioWorkletNode | null = null

  private captureCtx: AudioContext | null = null
  private captureNode: AudioWorkletNode | null = null
  private micStream: MediaStream | null = null

  private onStats: StatsHandler | null = null
  private onFrame: FrameHandler | null = null

  get playbackReady(): boolean {
    return this.playbackNode !== null
  }

  get captureReady(): boolean {
    return this.captureNode !== null
  }

  /** Latest known playback rate, for diagnostics. */
  get actualPlaybackRate(): number | null {
    return this.playbackCtx?.sampleRate ?? null
  }

  onPlaybackStats(handler: StatsHandler | null) {
    this.onStats = handler
  }

  onCaptureFrame(handler: FrameHandler | null) {
    this.onFrame = handler
  }

  /**
   * Brings up playback. Must be called from a user gesture — browsers refuse to
   * start an AudioContext otherwise, and a suspended context plays silence
   * without erroring.
   *
   * `begin` must not be sent until this resolves: the opening question is the
   * strongest moment in the product, and it must never be spoken into a page
   * that cannot play it.
   */
  async startPlayback(): Promise<void> {
    if (this.playbackNode) return

    let ctx: AudioContext
    try {
      ctx = new AudioContext({ sampleRate: PLAYBACK_RATE, latencyHint: 'interactive' })
    } catch (error) {
      throw new AudioError('Could not open an audio output context.', error)
    }

    // A browser that silently substitutes its own rate would play everything at
    // the wrong speed — the exact failure this file exists to prevent. Better
    // to refuse than to sound broken.
    if (ctx.sampleRate !== PLAYBACK_RATE) {
      const actual = ctx.sampleRate
      await ctx.close()
      throw new AudioError(
        `This browser opened audio output at ${actual} Hz instead of ${PLAYBACK_RATE} Hz, ` +
          'so playback would run at the wrong speed.',
      )
    }

    try {
      await ctx.audioWorklet.addModule('/worklets/playback.js')
    } catch (error) {
      await ctx.close()
      throw new AudioError('Could not load the playback worklet.', error)
    }

    const node = new AudioWorkletNode(ctx, 'playback', {
      numberOfInputs: 0,
      numberOfOutputs: 1,
      outputChannelCount: [1],
    })
    node.port.onmessage = (event) => this.onStats?.(event.data as PlaybackStats)
    node.connect(ctx.destination)

    if (ctx.state === 'suspended') await ctx.resume()

    this.playbackCtx = ctx
    this.playbackNode = node
  }

  /**
   * Brings up the microphone.
   *
   * echoCancellation is NOT optional. Without it the model hears its own voice
   * through the speakers, treats it as the candidate speaking, and the session
   * degrades into the interviewer interrupting itself. Use headphones anyway.
   */
  async startCapture(): Promise<void> {
    if (this.captureNode) return

    let stream: MediaStream
    try {
      stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: 1,
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
      })
    } catch (error) {
      throw new AudioError('Microphone permission was denied or no device is available.', error)
    }

    const ctx = new AudioContext({ sampleRate: CAPTURE_RATE, latencyHint: 'interactive' })
    try {
      await ctx.audioWorklet.addModule('/worklets/capture.js')
    } catch (error) {
      stream.getTracks().forEach((t) => t.stop())
      await ctx.close()
      throw new AudioError('Could not load the capture worklet.', error)
    }

    const source = ctx.createMediaStreamSource(stream)
    const node = new AudioWorkletNode(ctx, 'capture', {
      numberOfInputs: 1,
      numberOfOutputs: 0,
    })
    node.port.onmessage = (event) => this.onFrame?.(event.data as CaptureFrame)
    source.connect(node)

    if (ctx.state === 'suspended') await ctx.resume()

    this.micStream = stream
    this.captureCtx = ctx
    this.captureNode = node
  }

  /**
   * Opens and closes the microphone gate.
   *
   * Frames are produced ONLY between activity_start and activity_end. That is
   * the cost optimisation manual activity detection buys: a ten-minute session
   * is mostly silence, live audio is the dominant cost in the system, and none
   * of that silence is transmitted.
   *
   * Note what is deliberately NOT done: silence is not gated WITHIN an answer.
   * The backend derives words-per-minute and the delivery WAV from the audio it
   * receives, so dropping quiet frames mid-answer would shorten the recording,
   * inflate WPM, and hand the delivery analyser a chopped file.
   */
  setMicHot(hot: boolean) {
    this.captureNode?.port.postMessage(hot ? 'start' : 'stop')
  }

  /** Queues a decoded PCM chunk for playback. */
  play(pcm: Int16Array) {
    this.playbackNode?.port.postMessage(pcm, [pcm.buffer])
  }

  /** Discards queued audio. Call on `interrupted`, immediately. */
  flush() {
    this.playbackNode?.port.postMessage('flush')
  }

  async stop(): Promise<void> {
    this.setMicHot(false)

    this.micStream?.getTracks().forEach((track) => track.stop())
    this.micStream = null

    if (this.captureNode) {
      this.captureNode.port.onmessage = null
      this.captureNode.disconnect()
      this.captureNode = null
    }
    if (this.playbackNode) {
      this.playbackNode.port.onmessage = null
      this.playbackNode.disconnect()
      this.playbackNode = null
    }

    await Promise.all([this.captureCtx?.close(), this.playbackCtx?.close()].filter(Boolean))
    this.captureCtx = null
    this.playbackCtx = null
  }
}

/** RMS of a PCM16 buffer, normalised to 0..1. */
export function rms(pcm: Int16Array): number {
  if (pcm.length === 0) return 0
  let sum = 0
  for (let i = 0; i < pcm.length; i++) {
    const sample = pcm[i] / 32768
    sum += sample * sample
  }
  return Math.sqrt(sum / pcm.length)
}

/** Milliseconds of audio in a PCM16 mono buffer at the given rate. */
export function durationMs(samples: number, sampleRate: number): number {
  return sampleRate > 0 ? (samples * 1000) / sampleRate : 0
}
