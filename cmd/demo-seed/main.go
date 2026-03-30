package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/app"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type demoManifest struct {
	GeneratedAt                 time.Time `json:"generated_at"`
	RuntimeDB                   string    `json:"runtime_db"`
	MemoryDB                    string    `json:"memory_db"`
	BlobRoot                    string    `json:"blob_root"`
	MonthlyReviewWorkflowID     string    `json:"monthly_review_workflow_id,omitempty"`
	BehaviorWorkflowID          string    `json:"behavior_workflow_id,omitempty"`
	BehaviorApprovalID          string    `json:"behavior_approval_id,omitempty"`
	LifeEventWorkflowID         string    `json:"life_event_workflow_id,omitempty"`
	LifeEventTaskGraphID        string    `json:"life_event_task_graph_id,omitempty"`
	LifeEventChildWorkflowCount int       `json:"life_event_child_workflow_count,omitempty"`
}

func main() {
	var (
		runtimeDB  = flag.String("runtime-db", "./var/interview-demo/runtime.db", "runtime sqlite db path")
		memoryDB   = flag.String("memory-db", "./var/interview-demo/memory.db", "memory sqlite db path")
		blobRoot   = flag.String("blob-root", "./var/interview-demo/blob", "blob root for local-lite profile")
		fixtureDir = flag.String("fixture-dir", "./tests/fixtures", "fixture directory")
		outputPath = flag.String("out", "./var/interview-demo/demo-manifest.json", "seed manifest output path")
		reset      = flag.Bool("reset", true, "remove existing seed files before seeding")
	)
	flag.Parse()

	if *reset {
		for _, target := range []string{*runtimeDB, *memoryDB} {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				log.Fatalf("remove %s: %v", target, err)
			}
		}
		if err := os.RemoveAll(*blobRoot); err != nil && !os.IsNotExist(err) {
			log.Fatalf("remove blob root: %v", err)
		}
	}
	for _, dir := range []string{filepath.Dir(*runtimeDB), filepath.Dir(*memoryDB), *blobRoot, filepath.Dir(*outputPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	ctx := context.Background()
	manifest := demoManifest{
		GeneratedAt: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC),
		RuntimeDB:   *runtimeDB,
		MemoryDB:    *memoryDB,
		BlobRoot:    *blobRoot,
	}

	monthlyNow := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	monthlyEnv, err := app.OpenPhase5DEnvironment(app.Phase5DOptions{
		FixtureDir:    *fixtureDir,
		MemoryDBPath:  *memoryDB,
		RuntimeDBPath: *runtimeDB,
		Now:           func() time.Time { return monthlyNow },
	})
	if err != nil {
		log.Fatalf("open monthly review env: %v", err)
	}
	monthlyResult, err := monthlyEnv.RunMonthlyReview(ctx, "user-1", "请帮我做一份月度财务复盘，并给我一个适合 operator 演示的完整回放。", state.FinancialWorldState{})
	if err != nil {
		log.Fatalf("run monthly review seed: %v", err)
	}
	manifest.MonthlyReviewWorkflowID = monthlyResult.Result.WorkflowID
	if err := monthlyEnv.Close(); err != nil {
		log.Fatalf("close monthly review env: %v", err)
	}

	behaviorNow := monthlyNow.Add(5 * time.Minute)
	behaviorEnv, err := app.OpenPhase6BEnvironment(app.Phase6BOptions{
		FixtureDir:    *fixtureDir,
		MemoryDBPath:  *memoryDB,
		RuntimeDBPath: *runtimeDB,
		Now:           func() time.Time { return behaviorNow },
	})
	if err != nil {
		log.Fatalf("open behavior env: %v", err)
	}
	behaviorResult, err := behaviorEnv.RunBehaviorIntervention(ctx, "user-1", "请帮我做一份消费习惯复盘，并给我一个需要审批的消费护栏建议。", state.FinancialWorldState{})
	if err != nil {
		log.Fatalf("run behavior seed: %v", err)
	}
	manifest.BehaviorWorkflowID = behaviorResult.Result.WorkflowID
	if behaviorResult.Result.PendingApproval != nil {
		manifest.BehaviorApprovalID = behaviorResult.Result.PendingApproval.ApprovalID
	}
	if err := behaviorEnv.Close(); err != nil {
		log.Fatalf("close behavior env: %v", err)
	}

	lifeEventNow := behaviorNow.Add(5 * time.Minute)
	plane, err := app.OpenRuntimePlane(app.RuntimePlaneOptions{
		DBPath:         *runtimeDB,
		RuntimeProfile: "local-lite",
		RuntimeBackend: "sqlite",
		BlobBackend:    "localfs",
		BlobRoot:       *blobRoot,
		FixtureDir:     *fixtureDir,
		Now:            func() time.Time { return lifeEventNow },
	})
	if err != nil {
		log.Fatalf("open runtime plane: %v", err)
	}
	defer func() {
		if err := plane.Close(); err != nil {
			log.Printf("close runtime plane: %v", err)
		}
	}()

	lifeEvent, err := plane.LifeEvent.Run(ctx, sampleSalaryChangeEvent(lifeEventNow), state.FinancialWorldState{})
	if err != nil {
		log.Fatalf("run life event seed: %v", err)
	}
	manifest.LifeEventWorkflowID = lifeEvent.WorkflowID
	manifest.LifeEventTaskGraphID = lifeEvent.TaskGraph.GraphID
	manifest.LifeEventChildWorkflowCount = len(lifeEvent.FollowUpExecution.ExecutedTasks)

	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		log.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(*outputPath, payload, 0o644); err != nil {
		log.Fatalf("write manifest: %v", err)
	}
	fmt.Printf("seeded interview-demo data into %s\n", *outputPath)
}

func sampleSalaryChangeEvent(now time.Time) observation.LifeEventRecord {
	return observation.LifeEventRecord{
		ID:         "event-salary-follow-up",
		UserID:     "user-1",
		Kind:       observation.LifeEventSalaryChange,
		Source:     "fixture-hris",
		Provenance: "local runtime fixture salary change",
		ObservedAt: now,
		Confidence: 0.95,
		SalaryChange: &observation.SalaryChangeEventPayload{
			PreviousMonthlyIncomeCents: 1000000,
			NewMonthlyIncomeCents:      1250000,
			EffectiveAt:                now.AddDate(0, 0, -1),
		},
	}
}
