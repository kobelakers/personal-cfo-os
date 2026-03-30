package artifacts

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type BlobBackend string

const (
	BlobBackendLocalFS BlobBackend = "localfs"
	BlobBackendMinIO   BlobBackend = "minio"
)

type BlobRef struct {
	Backend  BlobBackend `json:"backend"`
	Bucket   string      `json:"bucket,omitempty"`
	Key      string      `json:"key,omitempty"`
	Location string      `json:"location"`
}

type BlobWriteResult struct {
	Ref          BlobRef   `json:"ref"`
	BytesWritten int64     `json:"bytes_written"`
	WrittenAt    time.Time `json:"written_at"`
}

type BlobReadResult struct {
	Ref     BlobRef   `json:"ref"`
	Content []byte    `json:"content"`
	ReadAt  time.Time `json:"read_at"`
}

type CheckpointPayloadRef struct {
	CheckpointID string  `json:"checkpoint_id"`
	BlobRef      BlobRef `json:"blob_ref"`
}

type ReplayBundleRef struct {
	ArtifactID string  `json:"artifact_id"`
	BlobRef    BlobRef `json:"blob_ref"`
}

type ArtifactBlobStore interface {
	Backend() BlobBackend
	WriteBlob(ctx context.Context, key string, contentType string, content []byte) (BlobWriteResult, error)
	ReadBlob(ctx context.Context, ref BlobRef) (BlobReadResult, error)
}

func BlobRefFromLocation(location string) (BlobRef, error) {
	trimmed := strings.TrimSpace(location)
	if trimmed == "" {
		return BlobRef{}, fmt.Errorf("blob location is required")
	}
	if strings.HasPrefix(trimmed, "s3://") {
		withoutScheme := strings.TrimPrefix(trimmed, "s3://")
		parts := strings.SplitN(withoutScheme, "/", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return BlobRef{}, fmt.Errorf("invalid s3 blob location %q", location)
		}
		return BlobRef{
			Backend:  BlobBackendMinIO,
			Bucket:   parts[0],
			Key:      parts[1],
			Location: trimmed,
		}, nil
	}
	return BlobRef{
		Backend:  BlobBackendLocalFS,
		Key:      filepath.Base(trimmed),
		Location: trimmed,
	}, nil
}
