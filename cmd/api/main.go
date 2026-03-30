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
	fixtureDir := flag.String("fixture-dir", "./tests/fixtures", "fixture directory for local observation seeds")
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

	server := &http.Server{
		Addr:    *addr,
		Handler: api.NewServer(plane.Query, plane.ReplayQuery, plane.Operator, plane.Service).Handler(),
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

	log.Printf("personal-cfo-os api listening on %s (db=%s)", *addr, *dbPath)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve api: %v", err)
	}
}
