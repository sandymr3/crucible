package live

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestAudioFrameRoundTrip(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0xff, 0xfe, 0x00, 0x00}

	for _, seq := range []uint32{0, 1, 42, 65535, 4294967295} {
		frame := encodeAudioFrame(seq, pcm)

		gotSeq, gotPCM, err := DecodeAudioFrame(frame)
		if err != nil {
			t.Fatalf("DecodeAudioFrame(seq=%d) error: %v", seq, err)
		}
		if gotSeq != seq {
			t.Errorf("seq = %d, want %d", gotSeq, seq)
		}
		if !bytes.Equal(gotPCM, pcm) {
			t.Errorf("payload = %v, want %v", gotPCM, pcm)
		}
	}
}

func TestDecodeAudioFrameRejectsShortFrame(t *testing.T) {
	// A frame shorter than the sequence prefix would otherwise slice out of
	// range and take the whole relay down with it.
	for _, n := range []int{0, 1, 3} {
		if _, _, err := DecodeAudioFrame(make([]byte, n)); err == nil {
			t.Errorf("DecodeAudioFrame(%d bytes) succeeded, want error", n)
		}
	}
}

func TestDecodeAudioFrameAllowsEmptyPayload(t *testing.T) {
	// Exactly the prefix and nothing else is well-formed, just empty.
	seq, pcm, err := DecodeAudioFrame(encodeAudioFrame(7, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seq != 7 {
		t.Errorf("seq = %d, want 7", seq)
	}
	if len(pcm) != 0 {
		t.Errorf("payload = %v, want empty", pcm)
	}
}

// The frontend switches on Type, so omitempty must not elide it, and fields
// belonging to other frame types must not leak into every message.
func TestServerFrameEncoding(t *testing.T) {
	b, err := ServerFrame{Type: TypeState, State: StateListening}.encode()
	if err != nil {
		t.Fatalf("encode() error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if decoded["type"] != TypeState {
		t.Errorf("type = %v, want %q", decoded["type"], TypeState)
	}
	if decoded["state"] != StateListening {
		t.Errorf("state = %v, want %q", decoded["state"], StateListening)
	}
	for _, unwanted := range []string{"side", "text", "code", "message", "totalTokens"} {
		if _, present := decoded[unwanted]; present {
			t.Errorf("state frame leaked unrelated field %q", unwanted)
		}
	}
}

// An interim transcript must stay distinguishable from a final one: the UI
// renders interim at reduced opacity and replaces it, so a mislabelled frame
// would duplicate text on screen.
func TestTranscriptFinalFlagSurvivesEncoding(t *testing.T) {
	for _, final := range []bool{true, false} {
		b, err := ServerFrame{
			Type: TypeTranscript, Side: SideUser, Text: "bloom filter", Final: final,
		}.encode()
		if err != nil {
			t.Fatalf("encode() error: %v", err)
		}

		var got ServerFrame
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("round-trip failed: %v", err)
		}
		if got.Final != final {
			t.Errorf("Final = %v, want %v", got.Final, final)
		}
		if got.Text != "bloom filter" {
			t.Errorf("Text = %q, want %q", got.Text, "bloom filter")
		}
	}
}
