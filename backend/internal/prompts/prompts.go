// Package prompts holds every prompt as a versioned, embedded asset.
//
// No prompt text appears in Go source anywhere in this project. You will
// iterate on these more than on any code here, and a prompt buried in a string
// literal is a prompt nobody edits at 2 a.m.
//
// Each file is content-hashed at startup and the short hash travels with every
// call that uses it, so when an evaluation looks wrong you can tell which
// prompt version produced it (AD-8).
package prompts

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
	"sync"
)

//go:embed assets/*.md
var assets embed.FS

// Name identifies a prompt asset.
type Name string

const (
	Digest            Name = "digest"
	PersonaTechLead   Name = "persona_tech_lead"
	PersonaArchitect  Name = "persona_architect"
	PersonaPM         Name = "persona_pm"
	EvaluateTurn      Name = "evaluate_turn"
	InjectionState    Name = "injection_state"
	HintSocratic      Name = "hint_socratic"
	DeliveryAnalysis  Name = "delivery_analysis"
	RoadmapBuild      Name = "roadmap_build"
	SyllabusDecompose Name = "syllabus_decompose"
	StudyQuestion     Name = "study_question"
)

// Prompt is one loaded asset.
type Prompt struct {
	Name Name
	Text string
	// Version is the first 8 hex characters of the content hash. Short enough
	// to read in a log line, long enough not to collide in practice.
	Version string
}

var (
	loadOnce sync.Once
	loaded   map[Name]*Prompt
	loadErr  error
)

// Load reads and hashes every embedded prompt. Safe to call repeatedly; the
// work happens once.
func Load() error {
	loadOnce.Do(func() {
		loaded = make(map[Name]*Prompt)

		entries, err := fs.ReadDir(assets, "assets")
		if err != nil {
			loadErr = fmt.Errorf("prompts: reading assets: %w", err)
			return
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			b, err := assets.ReadFile("assets/" + e.Name())
			if err != nil {
				loadErr = fmt.Errorf("prompts: reading %s: %w", e.Name(), err)
				return
			}
			sum := sha256.Sum256(b)
			name := Name(strings.TrimSuffix(e.Name(), ".md"))
			loaded[name] = &Prompt{
				Name:    name,
				Text:    string(b),
				Version: hex.EncodeToString(sum[:])[:8],
			}
		}
	})
	return loadErr
}

// Get returns a prompt by name.
func Get(name Name) (*Prompt, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	p, ok := loaded[name]
	if !ok {
		return nil, fmt.Errorf("prompts: no asset named %q", name)
	}
	return p, nil
}

// MustGet returns a prompt or panics.
//
// Panicking is correct here and only here: prompts are compiled into the
// binary, so a missing one is a build defect that cannot be recovered from at
// runtime and must not be discovered mid-interview.
func MustGet(name Name) *Prompt {
	p, err := Get(name)
	if err != nil {
		panic(err)
	}
	return p
}

// Render substitutes {{key}} placeholders.
//
// Deliberately not text/template: these prompts contain JSON examples full of
// braces, and every one would need escaping. A literal {{key}} scan has no such
// problem and the substitution needs are trivial.
func (p *Prompt) Render(vars map[string]string) string {
	out := p.Text
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

// Versions returns every loaded prompt's version, for the startup log.
func Versions() map[string]string {
	if err := Load(); err != nil {
		return nil
	}
	out := make(map[string]string, len(loaded))
	for name, p := range loaded {
		out[string(name)] = p.Version
	}
	return out
}
