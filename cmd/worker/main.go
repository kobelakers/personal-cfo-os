package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/app"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

func main() {
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
	workerID := flag.String("worker-id", "worker-local", "worker identity")
	role := flag.String("role", "all", "worker role: worker, scheduler, or all")
	once := flag.Bool("once", false, "run a single worker pass and exit")
	dryRun := flag.Bool("dry-run", false, "evaluate worker actions without mutating runtime state")
	interval := flag.Duration("interval", 30*time.Second, "worker polling interval")
	leaseTTL := flag.Duration("lease-ttl", 30*time.Second, "lease ttl for claimed work items")
	heartbeatInterval := flag.Duration("heartbeat-interval", 10*time.Second, "heartbeat interval for lease upkeep")
	claimBatch := flag.Int("claim-batch", 4, "maximum number of work items to claim per loop")
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runPass := func() error {
		result, err := plane.Service.RunAsyncWorkerOnce(ctx, runtime.DefaultAutoExecutionPolicy(), runtime.WorkerRunOptions{
			WorkerID:          runtime.WorkerID(*workerID),
			Role:              runtime.WorkerRole(*role),
			LeaseTTL:          *leaseTTL,
			HeartbeatInterval: *heartbeatInterval,
			ClaimBatch:        *claimBatch,
		}, *dryRun)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	if *once {
		if err := runPass(); err != nil {
			log.Fatalf("run worker pass: %v", err)
		}
		return
	}

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	log.Printf(
		"personal-cfo-os worker started (worker_id=%s role=%s profile=%s backend=%s db=%s interval=%s dry_run=%t)",
		*workerID,
		*role,
		*runtimeProfile,
		*runtimeBackend,
		*dbPath,
		interval.String(),
		*dryRun,
	)
	if err := runPass(); err != nil {
		log.Fatalf("run worker pass: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := runPass(); err != nil {
				log.Printf("run worker pass: %v", err)
			}
		}
	}
}
