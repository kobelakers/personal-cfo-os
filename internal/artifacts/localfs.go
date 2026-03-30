package artifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type LocalFSBlobStore struct {
	root string
	now  func() time.Time
}

func NewLocalFSBlobStore(root string, now func() time.Time) (*LocalFSBlobStore, error) {
	if root == "" {
		return nil, fmt.Errorf("localfs blob root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalFSBlobStore{
		root: root,
		now:  now,
	}, nil
}

func (s *LocalFSBlobStore) Backend() BlobBackend {
	return BlobBackendLocalFS
}

func (s *LocalFSBlobStore) WriteBlob(_ context.Context, key string, _ string, content []byte) (BlobWriteResult, error) {
	if key == "" {
		return BlobWriteResult{}, fmt.Errorf("blob key is required")
	}
	location := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(location), 0o755); err != nil {
		return BlobWriteResult{}, err
	}
	if err := os.WriteFile(location, content, 0o644); err != nil {
		return BlobWriteResult{}, err
	}
	return BlobWriteResult{
		Ref: BlobRef{
			Backend:  BlobBackendLocalFS,
			Key:      key,
			Location: location,
		},
		BytesWritten: int64(len(content)),
		WrittenAt:    s.nowUTC(),
	}, nil
}

func (s *LocalFSBlobStore) ReadBlob(_ context.Context, ref BlobRef) (BlobReadResult, error) {
	if ref.Location == "" {
		return BlobReadResult{}, fmt.Errorf("blob location is required")
	}
	content, err := os.ReadFile(ref.Location)
	if err != nil {
		return BlobReadResult{}, err
	}
	return BlobReadResult{
		Ref:     ref,
		Content: content,
		ReadAt:  s.nowUTC(),
	}, nil
}

func (s *LocalFSBlobStore) nowUTC() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}
