package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCorpusOutputsStructuredJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{
		"--mode", "corpus",
		"--scenario", "monthly_review_happy_path",
		"--fixture-dir", filepath.Join("..", "..", "tests", "fixtures"),
		"--workdir", t.TempDir(),
		"--format", "json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("run corpus json output: %v stderr=%s", err, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode eval json output: %v", err)
	}
	if payload["run_id"] == nil || payload["results"] == nil {
		t.Fatalf("expected eval run json payload, got %v", payload)
	}
}

func TestRunPhase5DMockRemainsSupported(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{
		"--mode", "phase",
		"--phase", "5d",
		"--workflow", "monthly_review",
		"--provider-mode", "mock",
		"--fixture-dir", filepath.Join("..", "..", "tests", "fixtures"),
		"--holdings-fixture", "holdings_2026-03-safe.csv",
		"--memory-db", filepath.Join(tempDir, "memory.db"),
		"--runtime-db", filepath.Join(tempDir, "runtime.db"),
	}, &stdout, &stderr); err != nil {
		t.Fatalf("run phase 5d compatibility path: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "workflow_id=") || !strings.Contains(output, "runtime_state=") {
		t.Fatalf("expected backward-compatible phase output, got %s", output)
	}
}

func TestRunPhase6BCorpusSummaryRemainsSupported(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{
		"--mode", "corpus",
		"--corpus", "phase6b-default",
		"--fixture-dir", filepath.Join("..", "..", "tests", "fixtures"),
		"--workdir", t.TempDir(),
		"--format", "summary",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("run phase 6b corpus summary: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "corpus=phase6b-default") || !strings.Contains(output, "behavior_intervention_happy_path") {
		t.Fatalf("expected phase 6b corpus summary output, got %s", output)
	}
}
