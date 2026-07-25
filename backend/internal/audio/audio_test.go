package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// tone generates a sine wave as PCM16 mono, so round-trip tests operate on
// something with real structure rather than zeros.
func tone(sampleRate, durationMs int, freq float64, amp float64) []byte {
	n := sampleRate * durationMs / 1000
	pcm := make([]byte, n*BytesPerSample)
	for i := 0; i < n; i++ {
		v := amp * math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)) * math.MaxInt16
		binary.LittleEndian.PutUint16(pcm[i*BytesPerSample:], uint16(int16(v)))
	}
	return pcm
}

func TestWAVRoundTrip(t *testing.T) {
	original := tone(SampleRateIn, 250, 440, 0.5)

	var buf bytes.Buffer
	if err := WriteWAV(&buf, original, SampleRateIn); err != nil {
		t.Fatalf("WriteWAV() error: %v", err)
	}

	got, rate, err := ReadWAV(buf.Bytes())
	if err != nil {
		t.Fatalf("ReadWAV() error: %v", err)
	}
	if rate != SampleRateIn {
		t.Errorf("sample rate = %d, want %d", rate, SampleRateIn)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("PCM round-trip mismatch: got %d bytes, want %d", len(got), len(original))
	}
}

// A parser that hardcodes offset 44 reads metadata as audio. Real files from
// real tools carry extra chunks before "data", so walk them.
func TestReadWAVSkipsExtraChunks(t *testing.T) {
	pcm := tone(SampleRateIn, 50, 440, 0.5)

	var base bytes.Buffer
	if err := WriteWAV(&base, pcm, SampleRateIn); err != nil {
		t.Fatalf("WriteWAV() error: %v", err)
	}
	b := base.Bytes()

	// Splice a LIST chunk between "fmt " and "data".
	list := []byte("LIST")
	list = binary.LittleEndian.AppendUint32(list, 4)
	list = append(list, "INFO"...)

	dataIdx := bytes.Index(b, []byte("data"))
	if dataIdx < 0 {
		t.Fatal("no data chunk in generated wav")
	}
	spliced := make([]byte, 0, len(b)+len(list))
	spliced = append(spliced, b[:dataIdx]...)
	spliced = append(spliced, list...)
	spliced = append(spliced, b[dataIdx:]...)
	// Fix the RIFF size so the file stays well-formed.
	binary.LittleEndian.PutUint32(spliced[4:], uint32(len(spliced)-8))

	got, rate, err := ReadWAV(spliced)
	if err != nil {
		t.Fatalf("ReadWAV() with LIST chunk error: %v", err)
	}
	if rate != SampleRateIn {
		t.Errorf("sample rate = %d, want %d", rate, SampleRateIn)
	}
	if !bytes.Equal(got, pcm) {
		t.Error("PCM mismatch after skipping LIST chunk — parser likely assumed a fixed offset")
	}
}

func TestReadWAVRejectsStereo(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteWAV(&buf, tone(SampleRateIn, 20, 440, 0.5), SampleRateIn); err != nil {
		t.Fatalf("WriteWAV() error: %v", err)
	}
	b := buf.Bytes()
	// Patch numChannels (offset 22 in the canonical header) to stereo.
	binary.LittleEndian.PutUint16(b[22:], 2)

	if _, _, err := ReadWAV(b); err == nil {
		t.Error("ReadWAV() accepted a stereo file; the Live API requires mono")
	}
}

func TestResampleChangesLengthProportionally(t *testing.T) {
	// The exact conversion the spike performs: Live output (24k) reused as
	// Live input (16k).
	in := tone(SampleRateOut, 300, 440, 0.5)
	out := Resample(in, SampleRateOut, SampleRateIn)

	wantSamples := (len(in) / BytesPerSample) * SampleRateIn / SampleRateOut
	gotSamples := len(out) / BytesPerSample
	if gotSamples != wantSamples {
		t.Errorf("resampled length = %d samples, want %d", gotSamples, wantSamples)
	}

	// Duration must be preserved within a millisecond of rounding.
	inMs := Duration(in, SampleRateOut)
	outMs := Duration(out, SampleRateIn)
	if diff := inMs - outMs; diff > 1 || diff < -1 {
		t.Errorf("duration changed across resample: %d ms -> %d ms", inMs, outMs)
	}
}

func TestResampleIsNoOpAtSameRate(t *testing.T) {
	in := tone(SampleRateIn, 40, 440, 0.5)
	if out := Resample(in, SampleRateIn, SampleRateIn); !bytes.Equal(out, in) {
		t.Error("Resample() modified data despite identical rates")
	}
}

func TestRMSDistinguishesSilenceFromSpeech(t *testing.T) {
	// This is the threshold that decides whether we pay live-audio token rates
	// to transmit a quiet room, so the gap needs to be unambiguous.
	silence := make([]byte, FrameSize(SampleRateIn))
	loud := tone(SampleRateIn, FrameDurationMs, 440, 0.8)

	if got := RMS(silence); got != 0 {
		t.Errorf("RMS(silence) = %v, want 0", got)
	}
	if got := RMS(loud); got < 0.3 {
		t.Errorf("RMS(loud tone) = %v, want well above a silence threshold", got)
	}
}

func TestFrameSizeIs20msOf16k(t *testing.T) {
	// 16000 Hz * 0.020 s * 2 bytes = 640.
	if got := FrameSize(SampleRateIn); got != 640 {
		t.Errorf("FrameSize(16k) = %d, want 640", got)
	}
}

func TestSplitFramesPadsFinalFrame(t *testing.T) {
	frameSize := FrameSize(SampleRateIn)
	// One and a half frames.
	pcm := make([]byte, frameSize+frameSize/2)

	frames := SplitFrames(pcm, frameSize)
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	for i, f := range frames {
		if len(f) != frameSize {
			t.Errorf("frame %d has %d bytes, want %d (final frame must be padded)", i, len(f), frameSize)
		}
	}
}
