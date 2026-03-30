package runtime

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	artifactblob "github.com/kobelakers/personal-cfo-os/internal/artifacts"
)

type StoreFactoryOptions struct {
	RuntimeProfile string
	RuntimeBackend string
	SQLitePath     string
	PostgresDSN    string

	BlobBackend   string
	BlobRoot      string
	BlobEndpoint  string
	BlobBucket    string
	BlobAccessKey string
	BlobSecretKey string

	Now func() time.Time
}

func OpenStoreBundle(options StoreFactoryOptions) (*StoreBundle, artifactblob.ArtifactBlobStore, error) {
	profile := firstNonEmpty(strings.TrimSpace(options.RuntimeProfile), "local-lite")
	backend := firstNonEmpty(strings.TrimSpace(options.RuntimeBackend), defaultRuntimeBackend(profile))

	var (
		bundle *StoreBundle
		err    error
	)
	switch backend {
	case "sqlite":
		var stores *SQLiteRuntimeStores
		stores, err = NewSQLiteRuntimeStores(options.SQLitePath)
		if err == nil {
			bundle = BundleFromSQLite(stores)
		}
	case "postgres":
		var stores *PostgresRuntimeStores
		stores, err = NewPostgresRuntimeStores(options.PostgresDSN)
		if err == nil {
			bundle = BundleFromPostgres(stores)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported runtime backend %q", backend)
	}
	if err != nil {
		return nil, nil, err
	}
	if bundle == nil {
		return nil, nil, fmt.Errorf("runtime store bundle is nil")
	}
	bundle.Profile = profile

	blobStore, err := openBlobStore(options, profile)
	if err != nil {
		_ = bundle.Close()
		return nil, nil, err
	}
	if blobStore != nil {
		bundle.Checkpoints = newRefBackedCheckpointStore(bundle.Checkpoints, blobStore)
		bundle.Artifacts = newRefBackedArtifactMetadataStore(bundle.Artifacts, blobStore)
	}
	return bundle, blobStore, nil
}

func BundleFromPostgres(stores *PostgresRuntimeStores) *StoreBundle {
	if stores == nil {
		return nil
	}
	return &StoreBundle{
		CloseFn:          stores.DB.Close,
		Backend:          "postgres",
		Profile:          "runtime-promotion",
		WorkflowRuns:     stores.WorkflowRuns,
		TaskGraphs:       stores.TaskGraphs,
		Executions:       stores.Executions,
		Approvals:        stores.Approvals,
		OperatorActions:  stores.OperatorActions,
		Checkpoints:      stores.Checkpoints,
		Replay:           stores.Replay,
		ReplayProjection: stores.ReplayProjection,
		ReplayQuery:      stores.ReplayQuery,
		Artifacts:        stores.Artifacts,
		WorkQueue:        stores.WorkQueue,
		WorkAttempts:     stores.WorkAttempts,
		Workers:          stores.Workers,
		Scheduler:        stores.Scheduler,
	}
}

func defaultRuntimeBackend(profile string) string {
	if profile == "runtime-promotion" {
		return "postgres"
	}
	return "sqlite"
}

func openBlobStore(options StoreFactoryOptions, profile string) (artifactblob.ArtifactBlobStore, error) {
	backend := strings.TrimSpace(options.BlobBackend)
	if backend == "" {
		if profile == "runtime-promotion" {
			backend = string(artifactblob.BlobBackendMinIO)
		} else {
			backend = string(artifactblob.BlobBackendLocalFS)
		}
	}
	switch backend {
	case string(artifactblob.BlobBackendLocalFS):
		root := strings.TrimSpace(options.BlobRoot)
		if root == "" {
			if strings.TrimSpace(options.SQLitePath) != "" {
				root = defaultLocalFSBlobRoot(options.SQLitePath)
			} else {
				root = "./var/blob"
			}
		}
		return artifactblob.NewLocalFSBlobStore(root, options.Now)
	case string(artifactblob.BlobBackendMinIO):
		endpoint, useSSL, err := artifactblob.ParseMinIOEndpoint(options.BlobEndpoint)
		if err != nil {
			return nil, err
		}
		return artifactblob.NewMinIOBlobStore(artifactblob.MinIOBlobStoreOptions{
			Endpoint:  endpoint,
			Bucket:    options.BlobBucket,
			AccessKey: options.BlobAccessKey,
			SecretKey: options.BlobSecretKey,
			UseSSL:    useSSL,
			Now:       options.Now,
		})
	default:
		return nil, fmt.Errorf("unsupported blob backend %q", backend)
	}
}

func defaultLocalFSBlobRoot(sqlitePath string) string {
	if strings.TrimSpace(sqlitePath) == "" {
		return "./var/blob"
	}
	return filepath.Join(filepath.Dir(sqlitePath), "blob")
}
