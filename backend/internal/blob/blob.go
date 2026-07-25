// Package blob handles Cloud Storage: resume PDFs in, turn audio out.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/storage"
)

const (
	// MaxResumeBytes matches the PRD's ingestion limit. Enforced here as well
	// as at the HTTP layer, because the cost of the digest call scales with it.
	MaxResumeBytes = 10 << 20 // 10 MiB

	// uploadTimeout bounds a single object write.
	uploadTimeout = 60 * time.Second
)

// ErrTooLarge is returned when an upload exceeds the size limit.
var ErrTooLarge = errors.New("blob: file exceeds the size limit")

// Store wraps a GCS bucket.
type Store struct {
	client *storage.Client
	bucket string
}

// New connects to Cloud Storage.
func New(ctx context.Context, bucket string) (*Store, error) {
	c, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("blob: connecting to cloud storage: %w", err)
	}
	return &Store{client: c, bucket: bucket}, nil
}

// Close releases the client.
func (s *Store) Close() error { return s.client.Close() }

// ResumePath is where a session's resume lives.
func ResumePath(uid, sessionID string) string {
	return fmt.Sprintf("resumes/%s/%s.pdf", uid, sessionID)
}

// AudioPath is where one turn's answer audio lives. A lifecycle rule deletes
// this prefix after seven days — audio is the bulk of storage and is worthless
// once the report exists.
func AudioPath(sessionID, turnID string) string {
	return fmt.Sprintf("audio/%s/%s.wav", sessionID, turnID)
}

// URI returns the gs:// URI for an object path, which is what Gemini accepts as
// a FileData part.
func (s *Store) URI(path string) string {
	return "gs://" + s.bucket + "/" + path
}

// Upload writes an object and returns its gs:// URI.
//
// The size limit is enforced while streaming rather than by trusting a
// Content-Length header, so a lying client cannot make us buffer an arbitrary
// amount of data.
func (s *Store) Upload(ctx context.Context, path, contentType string, r io.Reader, maxBytes int64) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()

	w := s.client.Bucket(s.bucket).Object(path).NewWriter(ctx)
	w.ContentType = contentType

	// One byte over the limit is enough to detect the overflow.
	written, err := io.Copy(w, io.LimitReader(r, maxBytes+1))
	if err != nil {
		_ = w.Close()
		return "", fmt.Errorf("blob: writing %s: %w", path, err)
	}
	if written > maxBytes {
		_ = w.Close()
		// Best-effort cleanup of the partial object.
		_ = s.Delete(context.WithoutCancel(ctx), path)
		return "", ErrTooLarge
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("blob: finalising %s: %w", path, err)
	}
	return s.URI(path), nil
}

// Download reads an object into memory.
func (s *Store) Download(ctx context.Context, path string) ([]byte, error) {
	r, err := s.client.Bucket(s.bucket).Object(path).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("blob: opening %s: %w", path, err)
	}
	defer r.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("blob: reading %s: %w", path, err)
	}
	return b, nil
}

// Delete removes an object. A missing object is not an error.
func (s *Store) Delete(ctx context.Context, path string) error {
	err := s.client.Bucket(s.bucket).Object(path).Delete(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("blob: deleting %s: %w", path, err)
	}
	return nil
}
