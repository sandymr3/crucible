PROJECT  ?= crucible-hack-0725
REGION   ?= us-central1
SERVICE  ?= crucible-backend
VERSION  := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

.PHONY: help
help:
	@echo "run          start the server locally (reads .env)"
	@echo "build        compile all binaries into bin/"
	@echo "test         unit tests"
	@echo "test-int     integration tests that call live Vertex (costs credits)"
	@echo "check        vet + test, what CI runs"
	@echo "deploy       build and deploy to Cloud Run"
	@echo "smoke        hit the deployed health endpoints"
	@echo "regionprobe  re-verify which regions serve the Live model"
	@echo "sdk-doc      dump the pinned genai Live surface for docs/sdk-surface.md"
	@echo "logs         tail Cloud Run logs"

.PHONY: run
run:
	go run ./cmd/server

.PHONY: build
build:
	go build -ldflags="-X main.buildVersion=$(VERSION)" -o bin/ ./cmd/...

.PHONY: test
test:
	go test ./... -count=1

# Integration tests hit real Vertex endpoints and therefore cost credits and
# take real time. Deliberately excluded from `check` so CI never burns budget.
.PHONY: test-int
test-int:
	go test ./... -count=1 -tags=integration -timeout=10m

.PHONY: check
check:
	go vet ./...
	go test ./... -count=1

.PHONY: deploy
deploy:
	bash deploy/deploy.sh

.PHONY: smoke
smoke:
	@URL=$$(gcloud run services describe $(SERVICE) --project=$(PROJECT) --region=$(REGION) --format='value(status.url)'); \
	echo "service: $$URL"; \
	echo "--- /health ---"; curl -fsS $$URL/health; echo; \
	echo "--- /readyz ---"; curl -fsS $$URL/readyz; echo

.PHONY: regionprobe
regionprobe:
	go run ./cmd/regionprobe

# Regenerate the verified SDK surface. Run this after any genai version bump —
# the Live API is marked Preview and its field names have moved before.
.PHONY: sdk-doc
sdk-doc:
	@for T in Backend ClientConfig Live Session LiveConnectConfig \
	          LiveSendRealtimeInputParameters LiveSendClientContentParameters \
	          LiveServerMessage LiveServerContent RealtimeInputConfig \
	          AutomaticActivityDetection SpeechConfig Blob; do \
	  echo "########## $$T ##########"; \
	  go doc google.golang.org/genai $$T; \
	  echo; \
	done

.PHONY: logs
logs:
	gcloud run services logs tail $(SERVICE) --project=$(PROJECT) --region=$(REGION)
