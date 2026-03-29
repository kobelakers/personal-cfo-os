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
	fixtureDir := flag.String("fixture-dir", "./tests/fixtures", "fixture directory for local observation seeds")
	once := flag.Bool("once", false, "run a single worker pass and exit")
	dryRun := flag.Bool("dry-run", false, "evaluate worker actions without mutating runtime state")
	interval := flag.Duration("interval", 30*time.Second, "worker polling interval")
	flag.Parse()

	plane, err := app.OpenRuntimePlane(app.RuntimePlaneOptions{
		DBPath:     *dbPath,
		FixtureDir: *fixtureDir,
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
		result, err := plane.Service.RunWorkerPass(ctx, runtime.DefaultAutoExecutionPolicy(), *dryRun)
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
	log.Printf("personal-cfo-os worker started (db=%s, interval=%s, dry_run=%t)", *dbPath, interval.String(), *dryRun)
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
