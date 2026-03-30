package artifacts

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLocalFSBlobStoreContract(t *testing.T) {
	store, err := NewLocalFSBlobStore(t.TempDir(), func() time.Time {
		return time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("create localfs blob store: %v", err)
	}
	written, err := store.WriteBlob(context.Background(), "reports/final.json", "application/json", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("write localfs blob: %v", err)
	}
	if written.Ref.Backend != BlobBackendLocalFS || written.Ref.Location == "" {
		t.Fatalf("expected localfs blob ref, got %+v", written)
	}
	read, err := store.ReadBlob(context.Background(), written.Ref)
	if err != nil {
		t.Fatalf("read localfs blob: %v", err)
	}
	if string(read.Content) != `{"ok":true}` {
		t.Fatalf("unexpected blob content %q", string(read.Content))
	}
}

func TestMinIOBlobStoreContract(t *testing.T) {
	endpoint := os.Getenv("MINIO_TEST_ENDPOINT")
	bucket := os.Getenv("MINIO_TEST_BUCKET")
	accessKey := os.Getenv("MINIO_TEST_ACCESS_KEY")
	secretKey := os.Getenv("MINIO_TEST_SECRET_KEY")
	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		t.Skip("set MINIO_TEST_ENDPOINT/MINIO_TEST_BUCKET/MINIO_TEST_ACCESS_KEY/MINIO_TEST_SECRET_KEY to run MinIO blob contract test")
	}
	parsedEndpoint, useSSL, err := ParseMinIOEndpoint(endpoint)
	if err != nil {
		t.Fatalf("parse minio endpoint: %v", err)
	}
	store, err := NewMinIOBlobStore(MinIOBlobStoreOptions{
		Endpoint:  parsedEndpoint,
		Bucket:    bucket,
		AccessKey: accessKey,
		SecretKey: secretKey,
		UseSSL:    useSSL,
		Now: func() time.Time {
			return time.Date(2026, 3, 30, 12, 5, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("create minio blob store: %v", err)
	}
	key := "runtime-tests/blob-store-contract.json"
	written, err := store.WriteBlob(context.Background(), key, "application/json", []byte(`{"backend":"minio"}`))
	if err != nil {
		t.Fatalf("write minio blob: %v", err)
	}
	if written.Ref.Backend != BlobBackendMinIO || written.Ref.Location == "" {
		t.Fatalf("expected minio blob ref, got %+v", written)
	}
	read, err := store.ReadBlob(context.Background(), written.Ref)
	if err != nil {
		t.Fatalf("read minio blob: %v", err)
	}
	if string(read.Content) != `{"backend":"minio"}` {
		t.Fatalf("unexpected minio blob content %q", string(read.Content))
	}
}
