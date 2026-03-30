package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/api"
	"github.com/kobelakers/personal-cfo-os/internal/app"
)

func main() {
	addr := flag.String("addr", ":8080", "API listen address")
	dbPath := flag.String("db", "./var/runtime.db", "durable runtime sqlite path")
	runtimeProfile := flag.String("runtime-profile", "local-lite", "runtime profile: local-lite or runtime-promotion")
	runtimeBackend := flag.String("runtime-backend", "sqlite", "runtime backend: sqlite or postgres")
	runtimeDSN := flag.String("runtime-dsn", "", "runtime backend dsn (required for postgres)")
	blobBackend := flag.String("blob-backend", "localfs", "blob backend: localfs or minio")
	blobRoot := flag.String("blob-root", "", "localfs blob root")
	blobEndpoint := flag.String("blob-endpoint", "", "minio endpoint")
	blobBucket := flag.String("blob-bucket", "", "minio bucket")
	blobAccessKey := flag.String("blob-access-key", "", "minio access key")
	blobSecretKey := flag.String("blob-secret-key", "", "minio secret key")
	fixtureDir := flag.String("fixture-dir", "./tests/fixtures", "fixture directory for local observation seeds")
	benchmarkDir := flag.String("benchmark-dir", "./docs/eval/samples", "benchmark sample directory")
	uiDist := flag.String("ui-dist", "./web/dist", "served operator ui dist directory")
	flag.Parse()

	plane, err := app.OpenRuntimePlane(app.RuntimePlaneOptions{
		DBPath:         *dbPath,
		RuntimeProfile: *runtimeProfile,
		RuntimeBackend: *runtimeBackend,
		RuntimeDSN:     *runtimeDSN,
		BlobBackend:    *blobBackend,
		BlobRoot:       *blobRoot,
		BlobEndpoint:   *blobEndpoint,
		BlobBucket:     *blobBucket,
		BlobAccessKey:  *blobAccessKey,
		BlobSecretKey:  *blobSecretKey,
		FixtureDir:     *fixtureDir,
	})
	if err != nil {
		log.Fatalf("open runtime plane: %v", err)
	}
	defer func() {
		if closeErr := plane.Close(); closeErr != nil {
			log.Printf("close runtime plane: %v", closeErr)
		}
	}()

	server := &http.Server{
		Addr: *addr,
		Handler: api.NewServer(plane.Query, plane.ReplayQuery, plane.Operator, plane.Service, api.ServerOptions{
			RuntimeProfile:          *runtimeProfile,
			RuntimeBackend:          *runtimeBackend,
			BlobBackend:             *blobBackend,
			BenchmarkCatalogDir:     *benchmarkDir,
			BenchmarkArtifacts:      plane.Stores.Artifacts,
			BenchmarkWorkflowRuns:   plane.Stores.WorkflowRuns,
			SupportedSchemaVersions: []string{"v1"},
			UIDistDir:               *uiDist,
		}).Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := api.Shutdown(shutdownCtx, server); err != nil {
			log.Printf("shutdown api server: %v", err)
		}
	}()

	log.Printf(
		"personal-cfo-os api listening on %s (profile=%s backend=%s db=%s blob=%s)",
		*addr,
		*runtimeProfile,
		*runtimeBackend,
		*dbPath,
		*blobBackend,
	)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve api: %v", err)
	}
}
