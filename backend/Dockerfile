# syntax=docker/dockerfile:1

# ---- build ----
# Must be >= the go directive in go.mod (1.26.4).
FROM golang:1.26-bookworm AS build

WORKDIR /src

# Copy manifests first so dependency download is cached independently of source
# changes. Matters when you are redeploying every twenty minutes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG BUILD_VERSION=dev
# CGO off gives a static binary that runs on distroless without a libc.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
      -ldflags="-s -w -X main.buildVersion=${BUILD_VERSION}" \
      -o /out/crucible-server ./cmd/server

# ---- runtime ----
# Distroless: no shell, no package manager, minimal attack surface. The image
# holds one static binary and CA certificates for TLS to Vertex.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/crucible-server /crucible-server

# Never bake credentials into the image. On Cloud Run the service account is
# attached to the revision and Application Default Credentials resolve it.
ENV APP_ENV=cloudrun

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/crucible-server"]
