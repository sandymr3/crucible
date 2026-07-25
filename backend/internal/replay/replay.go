// Package replay serves a recorded session over the live WebSocket protocol.
//
// This is AD-7, the "Ghost Session", and its entire purpose is demo insurance.
// PRD risk R3 rates venue wifi degrading the audio stream as Medium likelihood
// and Critical impact, and the only mitigation the PRD offers is a recorded
// video. A video is visibly an admission of failure. A replayed session is
// indistinguishable from a live one to everyone in the room: same frames, same
// timings, same UI, same everything — because it drives the identical protocol.
//
// It also touches Vertex zero times, so it costs nothing and cannot be broken
// by a rate limit, an outage, or a hotel network.
package replay

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed fixtures/*.json
var fixtureFS embed.FS

// EventKind distinguishes what a recorded event carries.
type EventKind string

const (
	// KindJSON is a control or content frame, replayed verbatim.
	KindJSON EventKind = "json"
	// KindAudio is a PCM chunk. Stored base64 so a fixture stays a single
	// self-contained JSON file with no companion binaries to lose.
	KindAudio EventKind = "audio"
)

// Event is one recorded frame plus when it happened.
type Event struct {
	// OffsetMs is milliseconds since the session began. Replaying against
	// these rather than against fixed gaps is what preserves the feel — the
	// pauses, the streaming cadence of the transcript, the length of each
	// answer.
	OffsetMs int64           `json:"offsetMs"`
	Kind     EventKind       `json:"kind"`
	Frame    json.RawMessage `json:"frame,omitempty"`
	Audio    string          `json:"audio,omitempty"`
}

// Fixture is one recorded session.
type Fixture struct {
	ID          string  `json:"id"`
	Description string  `json:"description"`
	Persona     string  `json:"persona"`
	RecordedAt  string  `json:"recordedAt"`
	Events      []Event `json:"events"`
}

// DurationMs returns the fixture's wall-clock length.
func (f *Fixture) DurationMs() int64 {
	if len(f.Events) == 0 {
		return 0
	}
	return f.Events[len(f.Events)-1].OffsetMs
}

var (
	loadOnce sync.Once
	fixtures map[string]*Fixture
	loadErr  error
)

// Load reads every embedded fixture. Safe to call repeatedly.
func Load() error {
	loadOnce.Do(func() {
		fixtures = make(map[string]*Fixture)

		entries, err := fs.ReadDir(fixtureFS, "fixtures")
		if err != nil {
			// No fixtures directory is not fatal: replay is optional, and the
			// product works without it.
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			b, err := fixtureFS.ReadFile("fixtures/" + e.Name())
			if err != nil {
				loadErr = fmt.Errorf("replay: reading %s: %w", e.Name(), err)
				return
			}
			var f Fixture
			if err := json.Unmarshal(b, &f); err != nil {
				loadErr = fmt.Errorf("replay: parsing %s: %w", e.Name(), err)
				return
			}
			if f.ID == "" {
				f.ID = strings.TrimSuffix(e.Name(), ".json")
			}
			// Events must be ordered for the timing loop to work; a recorder
			// writing them out of order would replay the session scrambled.
			sort.SliceStable(f.Events, func(i, j int) bool {
				return f.Events[i].OffsetMs < f.Events[j].OffsetMs
			})
			fixtures[f.ID] = &f
		}
	})
	return loadErr
}

// Get returns a fixture by ID.
func Get(id string) (*Fixture, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	f, ok := fixtures[id]
	if !ok {
		return nil, fmt.Errorf("replay: no fixture named %q", id)
	}
	return f, nil
}

// List returns every available fixture, for a picker.
func List() []*Fixture {
	if err := Load(); err != nil {
		return nil
	}
	out := make([]*Fixture, 0, len(fixtures))
	for _, f := range fixtures {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Emitter receives replayed frames. The relay implements it.
type Emitter interface {
	EmitJSON(payload []byte)
	EmitAudio(pcm []byte)
}

// Play streams a fixture at its original pace.
//
// speed scales playback: 1.0 is real time, 2.0 is twice as fast. Useful for
// rehearsal, and for a test that does not want to wait ninety seconds.
//
// Returns when the fixture ends or the stop channel closes.
func Play(f *Fixture, out Emitter, speed float64, stop <-chan struct{}) error {
	if speed <= 0 {
		speed = 1.0
	}
	started := time.Now()

	for _, e := range f.Events {
		// Sleep until this event's scheduled moment, measured from the start
		// rather than from the previous event — so a slow emit does not push
		// every later frame further out of time.
		target := time.Duration(float64(e.OffsetMs)/speed) * time.Millisecond
		if wait := target - time.Since(started); wait > 0 {
			select {
			case <-stop:
				return nil
			case <-time.After(wait):
			}
		}

		select {
		case <-stop:
			return nil
		default:
		}

		switch e.Kind {
		case KindAudio:
			pcm, err := base64.StdEncoding.DecodeString(e.Audio)
			if err != nil {
				return fmt.Errorf("replay: decoding audio at %dms: %w", e.OffsetMs, err)
			}
			out.EmitAudio(pcm)
		default:
			out.EmitJSON(e.Frame)
		}
	}
	return nil
}
