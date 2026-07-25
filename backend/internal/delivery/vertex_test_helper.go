//go:build integration

package delivery

import (
	"context"
	"log/slog"

	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/vertexai"
)

// newVertex builds a Vertex client for the integration tests.
func newVertex(cfg *config.Config, log *slog.Logger) (*vertexai.Client, error) {
	return vertexai.New(context.Background(), cfg, log, nil)
}
