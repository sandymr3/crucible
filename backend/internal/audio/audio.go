// Package audio handles the PCM16 plumbing between the browser, the relay, and
// the Vertex Live API.
//
// The Live API is asymmetric and it matters: it accepts 16 kHz mono PCM16 and
// emits 24 kHz mono PCM16. Accidentally treating output as input format
// produces audio that plays at the wrong speed rather than failing loudly,
// which is a genuinely confusing thing to debug at 3 a.m.
//
// Everything here operates on raw little-endian PCM16 bytes, because that is
// what both the WebSocket frames and the SDK Blob carry.
package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	// SampleRateIn is what the Live API expects from us.
	SampleRateIn = 16000
	// SampleRateOut is what the Live API sends back.
	SampleRateOut = 24000

	// BytesPerSample is PCM16 mono: 2 bytes, 1 channel.
	BytesPerSample = 2

	// FrameDuration is the capture granularity. 20 ms is the standard
	// real-time voice frame: small enough that latency is imperceptible, large
	// enough that per-frame overhead stays negligible.
	FrameDurationMs = 20
)

// FrameSize returns the byte length of one FrameDurationMs frame at the given
// sample rate. At 16 kHz that is 320 samples / 640 bytes.
func FrameSize(sampleRate int) int {
	return sampleRate * FrameDurationMs / 1000 * BytesPerSample
}

// Duration returns how long a PCM16 mono buffer plays for, in milliseconds.
// Used for words-per-minute, which PRD §13.2 insists must be computed rather
// than inferred from a model.
func Duration(pcm []byte, sampleRate int) int64 {
	if sampleRate <= 0 {
		return 0
	}
	samples := int64(len(pcm) / BytesPerSample)
	return samples * 1000 / int64(sampleRate)
}

// RMS returns the root-mean-square amplitude of a PCM16 buffer, normalised to
// 0..1.
//
// Two uses: driving the Live Room visualizer from real output amplitude rather
// than a decorative loop, and gating silent frames out of the upstream so we do
// not pay live-audio token rates to transmit a quiet room.
func RMS(pcm []byte) float64 {
	n := len(pcm) / BytesPerSample
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i+1 < len(pcm); i += 2 {
		s := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
		sum += s * s
	}
	return math.Sqrt(sum/float64(n)) / 32768.0
}

// Resample converts PCM16 mono between sample rates using linear
// interpolation.
//
// Linear interpolation is not high fidelity — it does not low-pass filter, so
// downsampling can alias. That is acceptable here because this exists for test
// fixtures and diagnostics, not for the production audio path: the browser
// captures at 16 kHz directly and playback consumes 24 kHz directly, so
// neither ever passes through this function.
func Resample(pcm []byte, fromRate, toRate int) []byte {
	if fromRate == toRate || fromRate <= 0 || toRate <= 0 {
		return pcm
	}
	inSamples := len(pcm) / BytesPerSample
	if inSamples == 0 {
		return nil
	}

	outSamples := int(int64(inSamples) * int64(toRate) / int64(fromRate))
	out := make([]byte, outSamples*BytesPerSample)

	ratio := float64(fromRate) / float64(toRate)
	for i := 0; i < outSamples; i++ {
		pos := float64(i) * ratio
		idx := int(pos)
		frac := pos - float64(idx)

		s0 := sampleAt(pcm, idx, inSamples)
		s1 := sampleAt(pcm, idx+1, inSamples)
		v := float64(s0)*(1-frac) + float64(s1)*frac

		binary.LittleEndian.PutUint16(out[i*BytesPerSample:], uint16(clampInt16(v)))
	}
	return out
}

func sampleAt(pcm []byte, idx, total int) int16 {
	if idx >= total {
		idx = total - 1
	}
	if idx < 0 {
		idx = 0
	}
	return int16(binary.LittleEndian.Uint16(pcm[idx*BytesPerSample:]))
}

func clampInt16(v float64) int16 {
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return int16(v)
}

// --- WAV ------------------------------------------------------------------

// WriteWAV writes PCM16 mono samples as a RIFF/WAVE file.
//
// Turn audio is persisted as WAV rather than raw PCM so the delivery-metrics
// call can hand Gemini a file it understands without a conversion step, and so
// a human can double-click the artifact when a grade looks wrong.
func WriteWAV(w io.Writer, pcm []byte, sampleRate int) error {
	const (
		headerSize    = 44
		fmtChunkSize  = 16
		pcmFormat     = 1 // uncompressed
		numChannels   = 1
		bitsPerSample = 16
	)

	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := len(pcm)

	buf := make([]byte, 0, headerSize)
	buf = append(buf, "RIFF"...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(36+dataSize))
	buf = append(buf, "WAVE"...)

	buf = append(buf, "fmt "...)
	buf = binary.LittleEndian.AppendUint32(buf, fmtChunkSize)
	buf = binary.LittleEndian.AppendUint16(buf, pcmFormat)
	buf = binary.LittleEndian.AppendUint16(buf, numChannels)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(sampleRate))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(byteRate))
	buf = binary.LittleEndian.AppendUint16(buf, uint16(blockAlign))
	buf = binary.LittleEndian.AppendUint16(buf, bitsPerSample)

	buf = append(buf, "data"...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(dataSize))

	if _, err := w.Write(buf); err != nil {
		return fmt.Errorf("audio: writing wav header: %w", err)
	}
	if _, err := w.Write(pcm); err != nil {
		return fmt.Errorf("audio: writing wav data: %w", err)
	}
	return nil
}

// ErrNotPCM16Mono is returned for WAV files this package cannot consume.
var ErrNotPCM16Mono = errors.New("audio: wav must be uncompressed PCM16 mono")

// ReadWAV parses a RIFF/WAVE file and returns its PCM16 mono samples and
// sample rate.
//
// Chunks are walked rather than assumed to sit at fixed offsets: real files
// written by real tools carry LIST/INFO and fact chunks before the data, and a
// parser that hardcodes offset 44 silently reads metadata as audio.
func ReadWAV(data []byte) (pcm []byte, sampleRate int, err error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, errors.New("audio: not a RIFF/WAVE file")
	}

	var (
		channels  uint16
		bits      uint16
		format    uint16
		foundFmt  bool
		foundData bool
	)

	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if size < 0 || body+size > len(data) {
			// Truncated final chunk: take what is actually there rather than
			// panicking on a file that is 99% usable.
			size = len(data) - body
		}

		switch id {
		case "fmt ":
			if size < 16 {
				return nil, 0, ErrNotPCM16Mono
			}
			format = binary.LittleEndian.Uint16(data[body:])
			channels = binary.LittleEndian.Uint16(data[body+2:])
			sampleRate = int(binary.LittleEndian.Uint32(data[body+4:]))
			bits = binary.LittleEndian.Uint16(data[body+14:])
			foundFmt = true
		case "data":
			pcm = data[body : body+size]
			foundData = true
		}

		// Chunks are word-aligned; odd-sized ones carry a pad byte.
		pos = body + size + (size & 1)
	}

	if !foundFmt || !foundData {
		return nil, 0, errors.New("audio: wav missing fmt or data chunk")
	}
	if format != 1 || channels != 1 || bits != 16 {
		return nil, 0, fmt.Errorf("%w (got format=%d channels=%d bits=%d)", ErrNotPCM16Mono, format, channels, bits)
	}
	return pcm, sampleRate, nil
}

// SplitFrames chops a PCM buffer into fixed-size frames. The final frame is
// zero-padded so every frame is a whole number of samples, which keeps the
// receiver's arithmetic simple.
func SplitFrames(pcm []byte, frameSize int) [][]byte {
	if frameSize <= 0 || len(pcm) == 0 {
		return nil
	}
	frames := make([][]byte, 0, len(pcm)/frameSize+1)
	for off := 0; off < len(pcm); off += frameSize {
		end := off + frameSize
		if end > len(pcm) {
			last := make([]byte, frameSize)
			copy(last, pcm[off:])
			frames = append(frames, last)
			break
		}
		frames = append(frames, pcm[off:end])
	}
	return frames
}
