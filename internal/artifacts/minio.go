package artifacts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOBlobStoreOptions struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Now       func() time.Time
}

type MinIOBlobStore struct {
	client *minio.Client
	bucket string
	now    func() time.Time
}

func NewMinIOBlobStore(options MinIOBlobStoreOptions) (*MinIOBlobStore, error) {
	if strings.TrimSpace(options.Endpoint) == "" {
		return nil, fmt.Errorf("minio endpoint is required")
	}
	if strings.TrimSpace(options.Bucket) == "" {
		return nil, fmt.Errorf("minio bucket is required")
	}
	client, err := minio.New(options.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(options.AccessKey, options.SecretKey, ""),
		Secure: options.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, options.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, options.Bucket, minio.MakeBucketOptions{}); err != nil {
			exists, checkErr := client.BucketExists(ctx, options.Bucket)
			if checkErr != nil || !exists {
				if checkErr != nil {
					return nil, checkErr
				}
				return nil, err
			}
		}
	}
	return &MinIOBlobStore{
		client: client,
		bucket: options.Bucket,
		now:    options.Now,
	}, nil
}

func (s *MinIOBlobStore) Backend() BlobBackend {
	return BlobBackendMinIO
}

func (s *MinIOBlobStore) WriteBlob(ctx context.Context, key string, contentType string, content []byte) (BlobWriteResult, error) {
	if strings.TrimSpace(key) == "" {
		return BlobWriteResult{}, fmt.Errorf("blob key is required")
	}
	reader := bytes.NewReader(content)
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, int64(len(content)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return BlobWriteResult{}, err
	}
	return BlobWriteResult{
		Ref: BlobRef{
			Backend:  BlobBackendMinIO,
			Bucket:   s.bucket,
			Key:      key,
			Location: fmt.Sprintf("s3://%s/%s", s.bucket, key),
		},
		BytesWritten: int64(len(content)),
		WrittenAt:    s.nowUTC(),
	}, nil
}

func (s *MinIOBlobStore) ReadBlob(ctx context.Context, ref BlobRef) (BlobReadResult, error) {
	key := ref.Key
	bucket := ref.Bucket
	if bucket == "" || key == "" {
		parsed, err := BlobRefFromLocation(ref.Location)
		if err != nil {
			return BlobReadResult{}, err
		}
		key = parsed.Key
		bucket = parsed.Bucket
		ref = parsed
	}
	reader, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return BlobReadResult{}, err
	}
	defer func() { _ = reader.Close() }()
	content, err := io.ReadAll(reader)
	if err != nil {
		return BlobReadResult{}, err
	}
	return BlobReadResult{
		Ref:     ref,
		Content: content,
		ReadAt:  s.nowUTC(),
	}, nil
}

func ParseMinIOEndpoint(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return "", false, fmt.Errorf("minio endpoint is required")
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", false, err
		}
		return parsed.Host, parsed.Scheme == "https", nil
	}
	return raw, false, nil
}

func (s *MinIOBlobStore) nowUTC() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}
