// Command regionprobe answers one Phase 0 question that nothing else can:
// which Vertex region actually serves the Live native-audio model, and which
// serves the reasoning models.
//
// PRD R11 ("Live model unavailable in chosen region") is rated Low likelihood
// but High impact, and the only honest way to retire it is to open a real bidi
// WebSocket. A REST models.get returns 404 for Live models even where they
// work, so metadata probing produces false negatives.
//
// Throwaway diagnostic. Kept in the tree because it is the fastest way to
// re-answer the question if a model is ever swapped.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"google.golang.org/genai"

	"github.com/santh/crucible/internal/config"
)

func main() {
	var (
		model     = flag.String("model", "gemini-live-2.5-flash-native-audio", "live model id to probe")
		locations = flag.String("locations", "us-central1,us-east4,europe-west4,global", "comma-separated regions")
	)
	flag.Parse()

	_ = config.LoadDotEnv(".env")
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		fmt.Fprintln(os.Stderr, "GOOGLE_CLOUD_PROJECT is required")
		os.Exit(1)
	}

	creds, err := credentials.DetectDefault(&credentials.DetectOptions{
		CredentialsFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "credentials: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("probing live model %q across regions\n\n", *model)

	for _, loc := range strings.Split(*locations, ",") {
		loc = strings.TrimSpace(loc)
		if loc == "" {
			continue
		}
		status := probe(context.Background(), project, loc, *model, creds)
		fmt.Printf("  %-14s %s\n", loc, status)
	}
}

func probe(ctx context.Context, project, location, model string, creds *auth.Credentials) string {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:     genai.BackendVertexAI,
		Project:     project,
		Location:    location,
		Credentials: creds,
	})
	if err != nil {
		return "CLIENT ERROR: " + err.Error()
	}

	session, err := client.Live.Connect(ctx, model, &genai.LiveConnectConfig{
		ResponseModalities: []genai.Modality{genai.ModalityAudio},
	})
	if err != nil {
		return "FAIL: " + oneLine(err.Error())
	}
	defer session.Close()

	// Connect alone can succeed before the server rejects the model, so wait
	// for the setup handshake to actually land.
	msg, err := session.Receive()
	if err != nil {
		return "CONNECTED but handshake failed: " + oneLine(err.Error())
	}
	if msg != nil && msg.SetupComplete != nil {
		return "OK — setupComplete received"
	}
	return "CONNECTED, first message carried no setupComplete"
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}
