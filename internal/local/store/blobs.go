package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// IANA media types stamped on blobs so they are self-describing, independent
// of the tile column that references them.
const (
	mediaMarkdown = "text/markdown"
	mediaJPEG     = "image/jpeg"
)

// GetBlob returns the bytes of a blob.
func (s *Store) GetBlob(ctx context.Context, blobID int64) ([]byte, error) {
	data, _, err := s.GetBlobWithMedia(ctx, blobID)
	return data, err
}

// GetBlobWithMedia returns a blob's bytes with its IANA media type, so a
// reader reports what the blob is instead of hard-coding a type.
func (s *Store) GetBlobWithMedia(ctx context.Context, blobID int64) ([]byte, string, error) {
	var (
		data      []byte
		mediaType string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT data, media_type FROM blobs WHERE id = ?`, blobID,
	).Scan(&data, &mediaType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("load blob: %w", err)
	}
	return data, mediaType, nil
}
