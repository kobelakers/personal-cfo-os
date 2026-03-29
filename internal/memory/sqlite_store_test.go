package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

func TestSQLiteMemoryStorePersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	stores, err := NewSQLiteMemoryStores(SQLiteStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("open sqlite memory stores: %v", err)
	}
	record := durableMemoryRecord(now)
	if err := stores.Store.Put(ctx, record); err != nil {
		t.Fatalf("put memory record: %v", err)
	}
	relations := MemoryRelations{
		Relations:  []MemoryRelation{{Type: "supports_plan", TargetMemoryID: "plan-1", Description: "supports planner emphasis"}},
		Supersedes: []SupersedesRef{{MemoryID: "memory-old", Reason: "newer durable memory"}},
		Conflicts:  []ConflictRef{{MemoryID: "memory-conflict", Reason: "older contradictory memory"}},
	}
	if err := stores.Relations.SaveRelations(ctx, record.ID, relations); err != nil {
		t.Fatalf("save relations: %v", err)
	}
	if err := stores.Embeddings.SaveEmbedding(ctx, MemoryEmbeddingRecord{
		MemoryID:    record.ID,
		Provider:    "static",
		Model:       "embedding-v1",
		Vector:      []float64{1, 0, 0},
		Dimensions:  3,
		EmbeddedAt:  now,
		ContentHash: "hash-1",
	}); err != nil {
		t.Fatalf("save embedding: %v", err)
	}
	if err := stores.Embeddings.ReplaceTerms(ctx, record.ID, map[string]int{"subscriptions": 2, "cleanup": 1}); err != nil {
		t.Fatalf("replace terms: %v", err)
	}
	if err := stores.Audit.AppendAccess(ctx, MemoryAccessAudit{
		ID:         "audit-1",
		MemoryID:   record.ID,
		WorkflowID: "workflow-1",
		TaskID:     "task-1",
		TraceID:    "trace-1",
		QueryID:    "query-1",
		Accessor:   "planner_agent",
		Purpose:    "memory_retrieval",
		Action:     "selected",
		Reason:     "high fused score",
		Score:      0.88,
		AccessedAt: now,
	}); err != nil {
		t.Fatalf("append audit: %v", err)
	}
	if err := stores.WriteEvents.AppendWriteEvent(ctx, MemoryWriteEvent{
		ID:         "event-1",
		MemoryID:   record.ID,
		WorkflowID: "workflow-1",
		TaskID:     "task-1",
		TraceID:    "trace-1",
		Action:     "inserted",
		Summary:    "seeded durable memory",
		Provider:   "static",
		Model:      "embedding-v1",
		OccurredAt: now,
		Details:    "initial seed",
	}); err != nil {
		t.Fatalf("append write event: %v", err)
	}
	if err := stores.DB.Close(); err != nil {
		t.Fatalf("close sqlite memory db: %v", err)
	}

	reopened, err := NewSQLiteMemoryStores(SQLiteStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("reopen sqlite memory stores: %v", err)
	}
	defer func() { _ = reopened.DB.Close() }()

	got, ok, err := reopened.Store.Get(ctx, record.ID)
	if err != nil || !ok {
		t.Fatalf("get reopened memory: err=%v ok=%v", err, ok)
	}
	if got.Summary != record.Summary {
		t.Fatalf("expected persisted summary, got %+v", got)
	}
	if len(got.Supersedes) != 1 || len(got.Conflicts) != 1 || len(got.Relations) != 1 {
		t.Fatalf("expected persisted relations, got %+v", got)
	}
	byKind, err := reopened.Query.ListByKind(ctx, MemoryListFilter{Kinds: []MemoryKind{MemoryKindSemantic}, Limit: 5, Recent: true})
	if err != nil {
		t.Fatalf("list by kind: %v", err)
	}
	if len(byKind) != 1 || byKind[0].ID != record.ID {
		t.Fatalf("expected list by kind to return durable record, got %+v", byKind)
	}
	recent, err := reopened.Query.ListRecent(ctx, 5)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent) != 1 || recent[0].ID != record.ID {
		t.Fatalf("expected recent list to return durable record, got %+v", recent)
	}
	embedding, ok, err := reopened.Embeddings.LoadEmbedding(ctx, record.ID, "embedding-v1")
	if err != nil || !ok {
		t.Fatalf("load embedding after restart: err=%v ok=%v", err, ok)
	}
	if embedding.Provider != "static" || embedding.Dimensions != 3 {
		t.Fatalf("expected persisted embedding metadata, got %+v", embedding)
	}
	models, err := reopened.Embeddings.LoadIndexedModels(ctx)
	if err != nil {
		t.Fatalf("load indexed models: %v", err)
	}
	if len(models) != 1 || models[0] != "embedding-v1" {
		t.Fatalf("expected indexed model to persist, got %+v", models)
	}
	lexical, err := reopened.Query.SearchLexical(ctx, []string{"subscriptions"}, 5)
	if err != nil {
		t.Fatalf("search lexical: %v", err)
	}
	if len(lexical) != 1 || lexical[0].MemoryID != record.ID {
		t.Fatalf("expected lexical hit after restart, got %+v", lexical)
	}
	audit, err := reopened.Audit.ListAccess(ctx, record.ID)
	if err != nil {
		t.Fatalf("list access audit: %v", err)
	}
	if len(audit) != 1 || audit[0].QueryID != "query-1" {
		t.Fatalf("expected persisted access audit, got %+v", audit)
	}
	events, err := reopened.WriteEvents.ListWriteEvents(ctx, record.ID)
	if err != nil {
		t.Fatalf("list write events: %v", err)
	}
	if len(events) != 1 || events[0].Action != "inserted" {
		t.Fatalf("expected persisted write event, got %+v", events)
	}
}

func TestMemoryIndexerBackfillsAndRebuildsIndexes(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	stores, err := NewSQLiteMemoryStores(SQLiteStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("open sqlite memory stores: %v", err)
	}
	defer func() { _ = stores.DB.Close() }()

	records := []MemoryRecord{
		durableMemoryRecord(now),
		{
			ID:      "memory-cashflow-1",
			Kind:    MemoryKindEpisodic,
			Summary: "上轮月度复盘提到夜间消费和经常性订阅仍需继续跟踪。",
			Facts: []MemoryFact{
				{Key: "late_night_spending_frequency", Value: "0.31", EvidenceID: observation.EvidenceID("evidence-late-night")},
			},
			Source: MemorySource{
				TaskID:      "task-monthly-review-2",
				WorkflowID:  "workflow-2",
				Actor:       "memory-steward",
				EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-late-night")},
			},
			Confidence: MemoryConfidence{Score: 0.82, Rationale: "prior monthly review memory"},
			CreatedAt:  now.Add(-24 * time.Hour),
			UpdatedAt:  now.Add(-24 * time.Hour),
		},
	}
	for _, record := range records {
		if err := stores.Store.Put(ctx, record); err != nil {
			t.Fatalf("put memory record %s: %v", record.ID, err)
		}
	}

	indexer := DefaultMemoryIndexer{
		Store:      stores.Store,
		Embeddings: stores.Embeddings,
		Writer:     stores.WriteEvents,
		Provider:   StaticEmbeddingProvider{Dimensions: 8},
		Model:      "memory-embed-v1",
		Now:        func() time.Time { return now },
		WorkflowID: "workflow-index",
		TaskID:     "task-index",
		TraceID:    "trace-index",
	}
	embedSummary, err := indexer.BackfillEmbeddings(ctx)
	if err != nil {
		t.Fatalf("backfill embeddings: %v", err)
	}
	if embedSummary.EmbeddingsBuilt != len(records) {
		t.Fatalf("expected embeddings for all records, got %+v", embedSummary)
	}
	termSummary, err := indexer.BackfillLexicalTerms(ctx)
	if err != nil {
		t.Fatalf("backfill lexical terms: %v", err)
	}
	if termSummary.TermsBuilt == 0 {
		t.Fatalf("expected lexical postings to be built, got %+v", termSummary)
	}
	lexical, err := stores.Query.SearchLexical(ctx, []string{"subscriptions", "night"}, 5)
	if err != nil {
		t.Fatalf("lexical search after backfill: %v", err)
	}
	if len(lexical) == 0 {
		t.Fatalf("expected lexical search results after backfill")
	}

	reindexer := indexer
	reindexer.Model = "memory-embed-v2"
	rebuildSummary, err := reindexer.RebuildIndexes(ctx)
	if err != nil {
		t.Fatalf("rebuild indexes with new model: %v", err)
	}
	if rebuildSummary.Model != "memory-embed-v2" || rebuildSummary.EmbeddingsBuilt != len(records) {
		t.Fatalf("expected rebuild summary for new model, got %+v", rebuildSummary)
	}
	models, err := stores.Embeddings.LoadIndexedModels(ctx)
	if err != nil {
		t.Fatalf("load indexed models: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected both embedding models to be queryable after rebuild, got %+v", models)
	}
}

func durableMemoryRecord(now time.Time) MemoryRecord {
	return MemoryRecord{
		ID:      "memory-subscription-1",
		Kind:    MemoryKindSemantic,
		Summary: "重复订阅是持续性现金流优化点，应优先放入月度复盘。",
		Facts: []MemoryFact{
			{Key: "duplicate_subscription_count", Value: "2", EvidenceID: observation.EvidenceID("evidence-subscription")},
		},
		Source: MemorySource{
			TaskID:      "task-monthly-review-1",
			WorkflowID:  "workflow-1",
			Actor:       "memory-steward",
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-subscription")},
		},
		Confidence: MemoryConfidence{Score: 0.92, Rationale: "repeated prior review signal"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
