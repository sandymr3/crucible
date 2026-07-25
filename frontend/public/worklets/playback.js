/**
 * Playback worklet.
 *
 * Output from the Live API is 24 kHz — note the asymmetry with 16 kHz capture.
 * Treating one as the other produces audio at the wrong speed rather than
 * failing loudly, which is a genuinely confusing thing to debug.
 *
 * A ring buffer consumed by a worklet, NOT an AudioBufferSourceNode per chunk.
 * Per-chunk source nodes produce an audible click at every boundary — fifty
 * clicks a second — because each node starts and stops its own envelope.
 * Nothing here allocates inside process().
 */

const RING_SECONDS = 10
const SAMPLE_RATE = 24000

/**
 * Samples that must accumulate before playback begins, and again after a flush.
 *
 * Without priming, every AI turn starts by underrunning: the first render
 * quantum arrives before the first chunk does, which stutters the opening
 * syllable and floods the underrun counter with events that say nothing about
 * network health. 100 ms is enough to absorb ordinary jitter without adding
 * latency anyone can perceive.
 */
const PRIME_SAMPLES = SAMPLE_RATE / 10

/** Report roughly every 21 ms, which is what the orb's amplitude wants. */
const STATS_EVERY_QUANTA = 4

class PlaybackProcessor extends AudioWorkletProcessor {
  constructor() {
    super()
    this.ring = new Float32Array(SAMPLE_RATE * RING_SECONDS)
    this.read = 0
    this.write = 0
    this.underruns = 0
    this.overruns = 0
    this.priming = true
    this.quanta = 0

    this.port.onmessage = (event) => {
      const data = event.data

      // The model was interrupted. Discard everything queued rather than
      // letting the buffer drain: a voice that keeps talking for two seconds
      // after being cut off feels broken.
      if (data === 'flush') {
        this.read = 0
        this.write = 0
        this.priming = true
        return
      }

      const pcm = data
      const size = this.ring.length
      for (let i = 0; i < pcm.length; i++) {
        const next = (this.write + 1) % size
        if (next === this.read) {
          // The ring is full — ten seconds behind real time. Drop rather than
          // overwrite unplayed audio, and count it: this means the client is
          // receiving faster than it can play, which is a real fault.
          this.overruns++
          break
        }
        this.ring[this.write] = pcm[i] / 32768
        this.write = next
      }
    }
  }

  available() {
    return this.write >= this.read
      ? this.write - this.read
      : this.ring.length - this.read + this.write
  }

  process(_inputs, outputs) {
    const out = outputs[0][0]
    if (!out) return true

    const ready = this.available()
    if (this.priming) {
      if (ready < PRIME_SAMPLES) {
        out.fill(0)
        this.report(0, ready)
        return true
      }
      this.priming = false
    }

    let sum = 0
    for (let i = 0; i < out.length; i++) {
      if (this.read === this.write) {
        out[i] = 0
        this.underruns++
      } else {
        const sample = this.ring[this.read]
        out[i] = sample
        sum += sample * sample
        this.read = (this.read + 1) % this.ring.length
      }
    }

    this.report(Math.sqrt(sum / out.length), this.available())
    return true
  }

  /**
   * RMS is computed here rather than on the main thread because this is the
   * only place that knows exactly which samples were emitted. It drives the
   * orb, so the visualiser follows real output amplitude rather than a
   * decorative loop.
   */
  report(rms, buffered) {
    this.quanta++
    if (this.quanta % STATS_EVERY_QUANTA !== 0) return
    this.port.postMessage({
      rms,
      buffered,
      underruns: this.underruns,
      overruns: this.overruns,
    })
  }
}

registerProcessor('playback', PlaybackProcessor)
