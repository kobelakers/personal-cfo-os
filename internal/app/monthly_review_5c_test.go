package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func TestMonthlyReview5CCrossSessionMemoryInfluence(t *testing.T) {
	memoryDB := filepath.Join(t.TempDir(), "memory.db")
	firstEnv := openMonthlyReview5CTestEnv(t, memoryDB, time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	first, err := firstEnv.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("first monthly review 5c run: %v", err)
	}
	for _, selection := range first.Trace.MemorySelections {
		if len(selection.SelectedMemoryIDs) != 0 {
			t.Fatalf("expected seed run to have no selected prior memory, got %+v", first.Trace.MemorySelections)
		}
	}
	if err := firstEnv.Close(); err != nil {
		t.Fatalf("close first env: %v", err)
	}

	secondEnv := openMonthlyReview5CTestEnv(t, memoryDB, time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC))
	second, err := secondEnv.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("second monthly review 5c run: %v", err)
	}
	if err := secondEnv.Close(); err != nil {
		t.Fatalf("close second env: %v", err)
	}

	controlDB := filepath.Join(t.TempDir(), "memory-control.db")
	controlEnv := openMonthlyReview5CTestEnv(t, controlDB, time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC))
	control, err := controlEnv.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("control monthly review 5c run: %v", err)
	}
	if err := controlEnv.Close(); err != nil {
		t.Fatalf("close control env: %v", err)
	}

	if len(second.Trace.MemoryQueries) < 2 || len(second.Trace.MemoryRetrievals) < 2 {
		t.Fatalf("expected planner and cashflow memory traces on second run, got %+v", second.Trace)
	}
	if len(second.Trace.MemorySelections) == 0 {
		t.Fatalf("expected second run to select durable memories, got %+v", second.Trace.MemorySelections)
	}
	if len(second.Result.Report.SourceMemoryIDs) == 0 {
		t.Fatalf("expected second report to record source memories, got %+v", second.Result.Report)
	}
	if len(control.Result.Report.SourceMemoryIDs) != 0 {
		t.Fatalf("expected fresh control db to have no prior memory influence, got %+v", control.Result.Report.SourceMemoryIDs)
	}
	if second.Result.Report.Summary == control.Result.Report.Summary {
		t.Fatalf("expected durable memory to change final monthly review summary, got second=%q control=%q", second.Result.Report.Summary, control.Result.Report.Summary)
	}
	if len(second.Result.Report.OptimizationSuggestions) == 0 || len(control.Result.Report.OptimizationSuggestions) == 0 {
		t.Fatalf("expected optimization suggestions in both runs")
	}
	if second.Result.Report.OptimizationSuggestions[0].Detail == control.Result.Report.OptimizationSuggestions[0].Detail {
		t.Fatalf("expected durable memory to change recommendation framing, got second=%q control=%q", second.Result.Report.OptimizationSuggestions[0].Detail, control.Result.Report.OptimizationSuggestions[0].Detail)
	}
	selected := map[string]struct{}{}
	for _, selection := range second.Trace.MemorySelections {
		for _, id := range selection.SelectedMemoryIDs {
			selected[id] = struct{}{}
		}
	}
	for _, id := range second.Result.Report.SourceMemoryIDs {
		if _, ok := selected[id]; !ok {
			t.Fatalf("expected selected memory ids to flow into final report provenance, missing %s in %+v", id, second.Trace.MemorySelections)
		}
	}
}

func TestMonthlyReview5CTraceIncludesRejectedMemories(t *testing.T) {
	memoryDB := filepath.Join(t.TempDir(), "memory.db")
	seedEnv := openMonthlyReview5CTestEnv(t, memoryDB, time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	seed, err := seedEnv.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("seed monthly review 5c run: %v", err)
	}
	if len(seed.Result.GeneratedMemories) == 0 {
		t.Fatalf("expected seed run to write durable memories")
	}

	stale := memory.MemoryRecord{
		ID:      "memory-sample-stale-episodic",
		Kind:    memory.MemoryKindEpisodic,
		Summary: "subscription subscription subscription cleanup reminder from a much older review",
		Facts:   []memory.MemoryFact{{Key: "duplicate_subscription_count", Value: "2", EvidenceID: observation.EvidenceID("evidence-subscription")}},
		Source: memory.MemorySource{
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-subscription")},
			TaskID:      "task-sample-stale",
			WorkflowID:  "workflow-sample-stale",
			TraceID:     "trace-sample-stale",
			Actor:       "memory_steward",
		},
		Confidence: memory.MemoryConfidence{Score: 0.92, Rationale: "old sample memory"},
		CreatedAt:  time.Date(2025, 11, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2025, 11, 1, 8, 0, 0, 0, time.UTC),
	}
	lowConfidence := memory.MemoryRecord{
		ID:      "memory-sample-low-confidence",
		Kind:    memory.MemoryKindSemantic,
		Summary: "subscription subscription recurring cashflow cleanup should maybe stay a recommendation priority",
		Facts:   []memory.MemoryFact{{Key: "duplicate_subscription_count", Value: "2", EvidenceID: observation.EvidenceID("evidence-subscription")}},
		Source: memory.MemorySource{
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-subscription")},
			TaskID:      "task-sample-low",
			WorkflowID:  "workflow-sample-low",
			TraceID:     "trace-sample-low",
			Actor:       "memory_steward",
		},
		Confidence: memory.MemoryConfidence{Score: 0.42, Rationale: "intentionally weak sample memory"},
		CreatedAt:  time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC),
	}
	for _, record := range []memory.MemoryRecord{stale, lowConfidence} {
		if err := seedEnv.MemoryStores.Store.Put(t.Context(), record); err != nil {
			t.Fatalf("seed rejection fixture %s: %v", record.ID, err)
		}
	}
	if _, err := seedEnv.RebuildMemoryIndexes(t.Context()); err != nil {
		t.Fatalf("rebuild memory indexes with rejection fixture: %v", err)
	}
	if err := seedEnv.Close(); err != nil {
		t.Fatalf("close seed env: %v", err)
	}

	secondEnv := openMonthlyReview5CTestEnv(t, memoryDB, time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC))
	result, err := secondEnv.Run(t.Context(), "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		t.Fatalf("second monthly review 5c run: %v", err)
	}
	if err := secondEnv.Close(); err != nil {
		t.Fatalf("close second env: %v", err)
	}

	var staleRejected, lowConfidenceRejected, selectedPresent bool
	for _, retrieval := range result.Trace.MemoryRetrievals {
		if len(retrieval.SelectedMemoryID) > 0 {
			selectedPresent = true
		}
		for _, candidate := range retrieval.Results {
			if candidate.MemoryID == stale.ID && candidate.RejectionRule == "stale_episodic" {
				staleRejected = true
			}
			if candidate.MemoryID == lowConfidence.ID && candidate.RejectionRule == "low_confidence" {
				lowConfidenceRejected = true
			}
		}
	}
	if !staleRejected || !lowConfidenceRejected || !selectedPresent {
		t.Fatalf("expected rejection trace evidence plus selected memories, got %+v", result.Trace.MemoryRetrievals)
	}
	if len(result.Result.Report.SourceMemoryIDs) == 0 {
		t.Fatalf("expected selected memories to continue influencing the final report")
	}
}

func openMonthlyReview5CTestEnv(t *testing.T, memoryDB string, now time.Time) *MonthlyReview5CEnvironment {
	t.Helper()
	env, err := OpenMonthlyReview5CEnvironment(MonthlyReview5COptions{
		FixtureDir:      monthlyReview5BFixtureDir(),
		HoldingsFixture: "holdings_2026-03-safe.csv",
		MemoryDBPath:    memoryDB,
		EmbeddingModel:  "mock-embedding-model",
		Now:             func() time.Time { return now },
		ChatModelFactory: func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return NewMockMonthlyReviewChatModelWithTrace(callRecorder, usageRecorder)
		},
		EmbeddingProviderFactory: func(callRecorder memory.EmbeddingCallRecorder, usageRecorder memory.EmbeddingUsageRecorder) memory.EmbeddingProvider {
			return NewMockMonthlyReviewEmbeddingProvider(callRecorder, usageRecorder)
		},
	})
	if err != nil {
		t.Fatalf("open monthly review 5c environment: %v", err)
	}
	return env
}
