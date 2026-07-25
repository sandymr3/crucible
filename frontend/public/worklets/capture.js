/**
 * Microphone capture worklet.
 *
 * Plain JS, served as a static asset — an AudioWorklet runs on the audio render
 * thread in its own global scope and cannot be bundled with the app.
 *
 * Produces exactly what the relay forwards to Vertex: PCM16, 16 kHz, mono, in
 * 20 ms frames of 320 samples (640 bytes). 20 ms is the standard real-time
 * voice frame — small enough that latency is imperceptible, large enough that
 * per-frame overhead stays negligible.
 *
 * An AudioWorkletProcessor is used rather than the deprecated ScriptProcessor,
 * which ran on the main thread and glitched under any UI work.
 */

const FRAME_SAMPLES = 320 // 20 ms at 16 kHz

class CaptureProcessor extends AudioWorkletProcessor {
  constructor() {
    super()
    this.buffer = new Int16Array(FRAME_SAMPLES)
    this.count = 0
    this.muted = true
    this.port.onmessage = (event) => {
      if (event.data === 'start') this.muted = false
      else if (event.data === 'stop') {
        this.muted = true
        this.count = 0 // drop a partial frame rather than leaking it into the next turn
      }
    }
  }

  process(inputs) {
    const channel = inputs[0] && inputs[0][0]
    // Returning true with no input keeps the node alive; returning false would
    // let the browser garbage-collect the processor mid-session.
    if (!channel || this.muted) return true

    for (let i = 0; i < channel.length; i++) {
      // Clamp BEFORE scaling. Float samples can exceed [-1,1] after gain, and
      // the multiply would wrap catastrophically into loud noise rather than
      // clipping.
      const sample = Math.max(-1, Math.min(1, channel[i]))
      // Asymmetric because two's complement is: -32768 to +32767.
      this.buffer[this.count++] = sample < 0 ? sample * 0x8000 : sample * 0x7fff

      if (this.count === FRAME_SAMPLES) {
        let sum = 0
        for (let n = 0; n < FRAME_SAMPLES; n++) {
          const s = this.buffer[n] / 32768
          sum += s * s
        }

        const frame = this.buffer.slice()
        // Transferred rather than copied: this runs fifty times a second and a
        // structured clone of every frame is pure waste on the audio thread.
        this.port.postMessage(
          { pcm: frame, rms: Math.sqrt(sum / FRAME_SAMPLES) },
          [frame.buffer],
        )
        this.count = 0
      }
    }
    return true
  }
}

registerProcessor('capture', CaptureProcessor)
