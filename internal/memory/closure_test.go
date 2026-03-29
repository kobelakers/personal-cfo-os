package memory

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type testEmbeddingCallLog struct {
	records []EmbeddingCallRecord
}

func (l *testEmbeddingCallLog) RecordEmbeddingCall(record EmbeddingCallRecord) {
	l.records = append(l.records, record)
}

type failingEmbeddingProvider struct {
	err error
}

func (p failingEmbeddingProvider) GenerateEmbedding(_ context.Context, _ EmbeddingRequest) (EmbeddingResponse, error) {
	return EmbeddingResponse{}, p.err
}

func (p failingEmbeddingProvider) Embed(_ context.Context, _ string) ([]float64, error) {
	return nil, p.err
}

func TestDefaultMemoryWriterAtomicCommitHappyPath(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	stores, writer := newSQLiteWriterHarness(t, now, StaticEmbeddingProvider{Dimensions: 12})

	record := durableWriterRecord("memory-happy", now, "workflow-atomic", "trace-atomic")
	if err := writer.Write(ctx, record); err != nil {
		t.Fatalf("writer.Write: %v", err)
	}

	got, ok, err := stores.Store.Get(ctx, record.ID)
	if err != nil || !ok {
		t.Fatalf("expected durable memory after atomic commit, err=%v ok=%v", err, ok)
	}
	if got.Source.TraceID != "trace-atomic" {
		t.Fatalf("expected persisted trace id, got %+v", got.Source)
	}
	embedding, ok, err := stores.Embeddings.LoadEmbedding(ctx, record.ID, "memory-embed-v1")
	if err != nil || !ok {
		t.Fatalf("expected embedding after commit, err=%v ok=%v", err, ok)
	}
	if embedding.Model != "memory-embed-v1" {
		t.Fatalf("unexpected embedding: %+v", embedding)
	}
	audit, err := stores.Audit.ListAccess(ctx, record.ID)
	if err != nil {
		t.Fatalf("list access audit: %v", err)
	}
	if len(audit) != 1 || audit[0].TraceID != "trace-atomic" {
		t.Fatalf("expected audit trace lineage, got %+v", audit)
	}
	events, err := stores.WriteEvents.ListWriteEvents(ctx, record.ID)
	if err != nil {
		t.Fatalf("list write events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected write + index events, got %+v", events)
	}
	for _, event := range events {
		if event.TraceID != "trace-atomic" {
			t.Fatalf("expected trace id to propagate into write events, got %+v", event)
		}
	}
	lexical, err := stores.Query.SearchLexical(ctx, []string{"subscription"}, 5)
	if err != nil {
		t.Fatalf("search lexical: %v", err)
	}
	if len(lexical) == 0 || lexical[0].MemoryID != record.ID {
		t.Fatalf("expected lexical postings after commit, got %+v", lexical)
	}
}

func TestDefaultMemoryWriterRollbackOnEmbeddingFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	stores, writer := newSQLiteWriterHarness(t, now, failingEmbeddingProvider{err: fmt.Errorf("embedding exploded")})

	record := durableWriterRecord("memory-embedding-fail", now, "workflow-embedding", "trace-embedding")
	if err := writer.Write(ctx, record); err == nil {
		t.Fatalf("expected embedding failure")
	}
	assertNoDurableMemoryArtifacts(t, ctx, stores, record.ID, "memory-embed-v1")
}

func TestDefaultMemoryWriterRollbackOnTermsFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	stores, writer := newSQLiteWriterHarness(t, now, StaticEmbeddingProvider{Dimensions: 12})
	mustExecSQLite(t, stores.DB.db, `
		CREATE TRIGGER fail_memory_terms_insert
		BEFORE INSERT ON memory_terms
		BEGIN
			SELECT RAISE(ABORT, 'forced memory_terms failure');
		END;
	`)

	record := durableWriterRecord("memory-terms-fail", now, "workflow-terms", "trace-terms")
	if err := writer.Write(ctx, record); err == nil {
		t.Fatalf("expected memory terms failure")
	}
	assertNoDurableMemoryArtifacts(t, ctx, stores, record.ID, "memory-embed-v1")
}

func TestDefaultMemoryWriterRollbackOnAuditFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	stores, writer := newSQLiteWriterHarness(t, now, StaticEmbeddingProvider{Dimensions: 12})
	mustExecSQLite(t, stores.DB.db, `
		CREATE TRIGGER fail_memory_audit_insert
		BEFORE INSERT ON memory_access_audit
		BEGIN
			SELECT RAISE(ABORT, 'forced memory_access_audit failure');
		END;
	`)

	record := durableWriterRecord("memory-audit-fail", now, "workflow-audit", "trace-audit")
	if err := writer.Write(ctx, record); err == nil {
		t.Fatalf("expected memory access audit failure")
	}
	assertNoDurableMemoryArtifacts(t, ctx, stores, record.ID, "memory-embed-v1")
}

func TestDefaultMemoryWriterRollbackOnWriteEventFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	stores, writer := newSQLiteWriterHarness(t, now, StaticEmbeddingProvider{Dimensions: 12})
	mustExecSQLite(t, stores.DB.db, `
		CREATE TRIGGER fail_memory_write_event_insert
		BEFORE INSERT ON memory_write_events
		BEGIN
			SELECT RAISE(ABORT, 'forced memory_write_events failure');
		END;
	`)

	record := durableWriterRecord("memory-event-fail", now, "workflow-event", "trace-event")
	if err := writer.Write(ctx, record); err == nil {
		t.Fatalf("expected memory write-event failure")
	}
	assertNoDurableMemoryArtifacts(t, ctx, stores, record.ID, "memory-embed-v1")
}

func TestTokenFrequencyPreservesRepeatedTerms(t *testing.T) {
	freq := tokenFrequency("subscription subscription subscription cleanup")
	if freq["subscription"] != 3 {
		t.Fatalf("expected repeated term frequency to be preserved, got %+v", freq)
	}
}

func TestSQLiteLexicalRetrievalUsesTermFrequency(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	stores, writer := newSQLiteWriterHarness(t, now, StaticEmbeddingProvider{Dimensions: 12})

	repeated := durableWriterRecord("memory-repeated", now, "workflow-lexical", "trace-lexical")
	repeated.Summary = "subscription subscription subscription cleanup"
	single := durableWriterRecord("memory-single", now, "workflow-lexical", "trace-lexical")
	single.Summary = "subscription cleanup"

	if err := writer.Write(ctx, repeated); err != nil {
		t.Fatalf("write repeated record: %v", err)
	}
	if err := writer.Write(ctx, single); err != nil {
		t.Fatalf("write single record: %v", err)
	}

	var termFreq int
	if err := stores.DB.db.QueryRowContext(ctx, `SELECT term_freq FROM memory_terms WHERE memory_id = ? AND term = ?`, repeated.ID, "subscription").Scan(&termFreq); err != nil {
		t.Fatalf("query term frequency: %v", err)
	}
	if termFreq <= 1 {
		t.Fatalf("expected stored term frequency to preserve repetition, got %d", termFreq)
	}

	results, err := stores.Query.SearchLexical(ctx, []string{"subscription"}, 5)
	if err != nil {
		t.Fatalf("search lexical: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected lexical hits for both memories, got %+v", results)
	}
	if results[0].MemoryID != repeated.ID {
		t.Fatalf("expected repeated term memory to rank first, got %+v", results)
	}
	if results[0].Score <= results[1].Score {
		t.Fatalf("expected repeated term memory to score higher, got %+v", results)
	}
}

func TestHybridRetrieverSelectsAcceptedCandidatesAfterRejection(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	stores, err := NewSQLiteMemoryStores(SQLiteStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("open sqlite memory stores: %v", err)
	}
	defer func() { _ = stores.DB.Close() }()

	records := []MemoryRecord{
		{
			ID:      "memory-stale-top",
			Kind:    MemoryKindEpisodic,
			Summary: "subscription subscription subscription cleanup reminder",
			Facts:   []MemoryFact{{Key: "duplicate_subscription_count", Value: "2"}},
			Source:  MemorySource{TaskID: "task-1", WorkflowID: "workflow-old", TraceID: "trace-old", Actor: "memory_steward"},
			Confidence: MemoryConfidence{
				Score:     0.92,
				Rationale: "older monthly review memory",
			},
			CreatedAt: now.Add(-120 * 24 * time.Hour),
			UpdatedAt: now.Add(-120 * 24 * time.Hour),
		},
		{
			ID:      "memory-fresh-accepted",
			Kind:    MemoryKindSemantic,
			Summary: "subscription cleanup should remain a current cashflow priority",
			Facts:   []MemoryFact{{Key: "duplicate_subscription_count", Value: "2"}},
			Source:  MemorySource{TaskID: "task-2", WorkflowID: "workflow-new", TraceID: "trace-new", Actor: "memory_steward"},
			Confidence: MemoryConfidence{
				Score:     0.9,
				Rationale: "fresh durable memory",
			},
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now.Add(-24 * time.Hour),
		},
	}
	for _, record := range records {
		if err := stores.Store.Put(ctx, record); err != nil {
			t.Fatalf("put memory %s: %v", record.ID, err)
		}
	}
	indexer := DefaultMemoryIndexer{
		Store:      stores.Store,
		Embeddings: stores.Embeddings,
		Provider:   StaticEmbeddingProvider{Dimensions: 12},
		Model:      "memory-embed-v1",
		Now:        func() time.Time { return now },
		WorkflowID: "workflow-index",
		TaskID:     "task-index",
		TraceID:    "trace-index",
	}
	if _, err := indexer.RebuildIndexes(ctx); err != nil {
		t.Fatalf("rebuild indexes: %v", err)
	}

	retriever := HybridMemoryRetriever{
		Lexical: LexicalRetriever{Query: stores.Query, Audit: stores.Audit},
		Semantic: SemanticRetriever{
			Store: stores.Store,
			Backend: EmbeddedSemanticSearchBackend{
				Provider:   StaticEmbeddingProvider{Dimensions: 12},
				Embeddings: stores.Embeddings,
				Model:      "memory-embed-v1",
				Scorer:     DefaultRetrievalScorer{},
			},
			Audit: stores.Audit,
		},
		Fusion:   ReciprocalRankFusion{},
		Reranker: BaselineReranker{},
		RejectionPolicy: ThresholdRejectionPolicy{
			MinScore: 0.01,
			DefaultPolicy: RetrievalPolicy{
				Name: "monthly_review_default",
				FreshnessPolicy: FreshnessPolicy{
					Name:                  "monthly_review_default",
					EpisodicMaxAge:        90 * 24 * time.Hour,
					RejectLowConfidence:   true,
					LowConfidenceFloor:    0.7,
					MinAcceptedFusedScore: 0.01,
				},
			},
			Now: func() time.Time { return now },
		},
	}

	results, err := retriever.Retrieve(ctx, RetrievalQuery{
		QueryID:         "query-after-reject",
		WorkflowID:      "workflow-query",
		TaskID:          "task-query",
		TraceID:         "trace-query",
		Consumer:        MemoryConsumerCashflow,
		ContextView:     "execution",
		Text:            "subscription cleanup priority",
		LexicalTerms:    []string{"subscription", "cleanup"},
		SemanticHint:    "cashflow recommendation framing",
		TopK:            1,
		RetrievalPolicy: "monthly_review_cashflow",
		FreshnessPolicy: "monthly_review_default",
	})
	if err != nil {
		t.Fatalf("hybrid retrieve: %v", err)
	}
	var staleRejected, freshSelected bool
	for _, result := range results {
		if result.MemoryID == "memory-stale-top" && result.Rejected && result.RejectionRule == "stale_episodic" {
			staleRejected = true
		}
		if result.MemoryID == "memory-fresh-accepted" && result.Selected {
			freshSelected = true
		}
	}
	if !staleRejected || !freshSelected {
		t.Fatalf("expected stale rejection and lower-rank accepted selection, got %+v", results)
	}
}

func TestDefaultMemoryWriterPreservesTraceIDLineage(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	callLog := &testEmbeddingCallLog{}
	stores, writer := newSQLiteWriterHarness(t, now, StaticEmbeddingProvider{
		Dimensions:   12,
		CallRecorder: callLog,
	})
	record := durableWriterRecord("memory-lineage", now, "workflow-lineage", "trace-lineage")
	if err := writer.Write(ctx, record); err != nil {
		t.Fatalf("writer.Write: %v", err)
	}
	if len(callLog.records) != 1 || callLog.records[0].TraceID != "trace-lineage" {
		t.Fatalf("expected embedding trace lineage to preserve trace id, got %+v", callLog.records)
	}
	audit, err := stores.Audit.ListAccess(ctx, record.ID)
	if err != nil {
		t.Fatalf("list access audit: %v", err)
	}
	if len(audit) != 1 || audit[0].WorkflowID != "workflow-lineage" || audit[0].TraceID != "trace-lineage" {
		t.Fatalf("expected write audit lineage, got %+v", audit)
	}
	events, err := stores.WriteEvents.ListWriteEvents(ctx, record.ID)
	if err != nil {
		t.Fatalf("list write events: %v", err)
	}
	for _, event := range events {
		if event.WorkflowID == event.TraceID {
			t.Fatalf("expected trace id to remain distinct from workflow id, got %+v", event)
		}
		if event.TraceID != "trace-lineage" {
			t.Fatalf("expected propagated trace id, got %+v", event)
		}
	}
}

func TestDefaultMemoryWriterAllowsEpisodicWithinLowConfidenceFloor(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	stores, writer := newSQLiteWriterHarness(t, now, nil)
	record := durableWriterRecord("memory-episodic-floor", now, "workflow-episodic", "trace-episodic")
	record.Kind = MemoryKindEpisodic
	record.Confidence = MemoryConfidence{Score: 0.6, Rationale: "episodic signal within floor"}
	if err := writer.Write(ctx, record); err != nil {
		t.Fatalf("expected episodic memory within floor to persist, got %v", err)
	}
	if _, ok, err := stores.Store.Get(ctx, record.ID); err != nil || !ok {
		t.Fatalf("expected episodic memory to persist, err=%v ok=%v", err, ok)
	}
}

func TestWorkflowMemoryServiceUsesDistinctTraceLineage(t *testing.T) {
	store := NewInMemoryMemoryStore()
	traceLog := &memoryTraceRecorderStub{}
	service := WorkflowMemoryService{
		Writer:        DefaultMemoryWriter{Store: store, MinConfidence: 0.7, LowConfidenceEpisodicFloor: 0.55},
		Retriever:     HybridMemoryRetriever{Lexical: LexicalRetriever{Store: store}, Semantic: SemanticRetriever{Store: store, Backend: FakeSemanticSearchBackend{Provider: KeywordEmbeddingProvider{Dimensions: 8}, Index: NewInMemoryVectorIndex()}}, Fusion: ReciprocalRankFusion{}, Reranker: BaselineReranker{}, RejectionPolicy: ThresholdRejectionPolicy{MinScore: 0.01}},
		TraceRecorder: traceLog,
		Now:           func() time.Time { return time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC) },
	}

	spec := taskspec.TaskSpec{ID: "task-monthly-review", Goal: "月度复盘", Scope: taskspec.TaskScope{Areas: []string{"cashflow"}}}
	current := state.FinancialWorldState{
		UserID: "user-1",
		BehaviorState: state.BehaviorState{
			DuplicateSubscriptionCount: 2,
		},
	}
	evidence := []observation.EvidenceRecord{
		retrievalEvidence("evidence-subscription", observation.EvidenceTypeRecurringSubscription, "subscriptions"),
	}

	result, err := service.SyncMonthlyReview(t.Context(), spec, "workflow-monthly-review-1", current, evidence)
	if err != nil {
		t.Fatalf("sync monthly review memories: %v", err)
	}
	if len(result.GeneratedRecords) == 0 {
		t.Fatalf("expected generated memories")
	}
	expectedTraceID := result.GeneratedRecords[0].Source.TraceID
	if expectedTraceID == "" || expectedTraceID == "workflow-monthly-review-1" {
		t.Fatalf("expected distinct generated trace id, got %q", expectedTraceID)
	}
	if len(traceLog.queries) == 0 || len(traceLog.retrievals) == 0 || len(traceLog.selections) == 0 {
		t.Fatalf("expected memory trace records, got %+v", traceLog)
	}
	for _, record := range traceLog.queries {
		if record.TraceID != expectedTraceID {
			t.Fatalf("expected query trace id %q, got %+v", expectedTraceID, record)
		}
	}
	for _, record := range traceLog.retrievals {
		if record.TraceID != expectedTraceID {
			t.Fatalf("expected retrieval trace id %q, got %+v", expectedTraceID, record)
		}
	}
	for _, record := range traceLog.selections {
		if record.TraceID != expectedTraceID {
			t.Fatalf("expected selection trace id %q, got %+v", expectedTraceID, record)
		}
	}
}

func TestConflictAndSupersedenceDetectors(t *testing.T) {
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	existing := []MemoryRecord{
		{
			ID:         "memory-old-value",
			Kind:       MemoryKindSemantic,
			Summary:    "Subscription cleanup remains important.",
			Facts:      []MemoryFact{{Key: "duplicate_subscription_count", Value: "2"}},
			Source:     MemorySource{WorkflowID: "workflow-1"},
			Confidence: MemoryConfidence{Score: 0.9, Rationale: "existing"},
			CreatedAt:  now.Add(-48 * time.Hour),
			UpdatedAt:  now.Add(-48 * time.Hour),
		},
		{
			ID:         "memory-same-summary",
			Kind:       MemoryKindSemantic,
			Summary:    "Keep tracking duplicate subscriptions.",
			Facts:      []MemoryFact{{Key: "duplicate_subscription_count", Value: "2"}},
			Source:     MemorySource{WorkflowID: "workflow-1"},
			Confidence: MemoryConfidence{Score: 0.9, Rationale: "existing"},
			CreatedAt:  now.Add(-24 * time.Hour),
			UpdatedAt:  now.Add(-24 * time.Hour),
		},
	}

	conflictCandidate := MemoryRecord{
		ID:         "memory-new-value",
		Kind:       MemoryKindSemantic,
		Summary:    "Subscription cleanup remains important.",
		Facts:      []MemoryFact{{Key: "duplicate_subscription_count", Value: "3"}},
		Source:     MemorySource{WorkflowID: "workflow-2"},
		Confidence: MemoryConfidence{Score: 0.9, Rationale: "candidate"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	conflicts := DefaultConflictDetector{}.Detect(existing, conflictCandidate)
	if len(conflicts) == 0 || conflicts[0].MemoryID != "memory-old-value" {
		t.Fatalf("expected same fact key with different value to conflict, got %+v", conflicts)
	}

	supersedeCandidate := MemoryRecord{
		ID:         "memory-newer-summary",
		Kind:       MemoryKindSemantic,
		Summary:    "Keep tracking duplicate subscriptions.",
		Facts:      []MemoryFact{{Key: "duplicate_subscription_count", Value: "2"}},
		Source:     MemorySource{WorkflowID: "workflow-2"},
		Confidence: MemoryConfidence{Score: 0.92, Rationale: "candidate"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	supersedes := DefaultSupersedenceDetector{}.Detect(existing, supersedeCandidate)
	if len(supersedes) == 0 || supersedes[0].MemoryID != "memory-same-summary" {
		t.Fatalf("expected newer same-summary memory to supersede, got %+v", supersedes)
	}

	selfConflict := DefaultConflictDetector{}.Detect([]MemoryRecord{supersedeCandidate}, supersedeCandidate)
	if len(selfConflict) != 0 {
		t.Fatalf("expected self conflict to be ignored, got %+v", selfConflict)
	}
	selfSupersede := DefaultSupersedenceDetector{}.Detect([]MemoryRecord{supersedeCandidate}, supersedeCandidate)
	if len(selfSupersede) != 0 {
		t.Fatalf("expected self supersede to be ignored, got %+v", selfSupersede)
	}
}

func TestExplicitConflictAndSupersedesPersistOnWrite(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	stores, writer := newSQLiteWriterHarness(t, now, StaticEmbeddingProvider{Dimensions: 8})

	record := durableWriterRecord("memory-explicit-relations", now, "workflow-explicit", "trace-explicit")
	record.Supersedes = []SupersedesRef{{MemoryID: "memory-old", Reason: "explicit supersedence"}}
	record.Conflicts = []ConflictRef{{MemoryID: "memory-conflict", Reason: "explicit conflict"}}
	if err := writer.Write(ctx, record); err != nil {
		t.Fatalf("writer.Write: %v", err)
	}
	got, ok, err := stores.Store.Get(ctx, record.ID)
	if err != nil || !ok {
		t.Fatalf("expected persisted record, err=%v ok=%v", err, ok)
	}
	if len(got.Supersedes) != 1 || got.Supersedes[0].MemoryID != "memory-old" {
		t.Fatalf("expected explicit supersedes relation to persist, got %+v", got.Supersedes)
	}
	if len(got.Conflicts) != 1 || got.Conflicts[0].MemoryID != "memory-conflict" {
		t.Fatalf("expected explicit conflict relation to persist, got %+v", got.Conflicts)
	}
}

type memoryTraceRecorderStub struct {
	queries    []MemoryQueryRecord
	retrievals []MemoryRetrievalRecord
	selections []MemorySelectionRecord
}

func (s *memoryTraceRecorderStub) RecordMemoryQuery(record MemoryQueryRecord) {
	s.queries = append(s.queries, record)
}

func (s *memoryTraceRecorderStub) RecordMemoryRetrieval(record MemoryRetrievalRecord) {
	s.retrievals = append(s.retrievals, record)
}

func (s *memoryTraceRecorderStub) RecordMemorySelection(record MemorySelectionRecord) {
	s.selections = append(s.selections, record)
}

func newSQLiteWriterHarness(t *testing.T, now time.Time, provider EmbeddingProvider) (*SQLiteMemoryStores, DefaultMemoryWriter) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	stores, err := NewSQLiteMemoryStores(SQLiteStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("open sqlite memory stores: %v", err)
	}
	t.Cleanup(func() { _ = stores.DB.Close() })
	writer := DefaultMemoryWriter{
		Store:                      stores.Store,
		EmbeddingProvider:          provider,
		EmbeddingModel:             "memory-embed-v1",
		MinConfidence:              0.7,
		LowConfidenceEpisodicFloor: 0.55,
		Now:                        func() time.Time { return now },
	}
	return stores, writer
}

func durableWriterRecord(id string, now time.Time, workflowID string, traceID string) MemoryRecord {
	return MemoryRecord{
		ID:      id,
		Kind:    MemoryKindSemantic,
		Summary: "subscription subscription cleanup remains a priority",
		Facts: []MemoryFact{
			{Key: "duplicate_subscription_count", Value: "2", EvidenceID: observation.EvidenceID("evidence-subscription")},
		},
		Source: MemorySource{
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-subscription")},
			TaskID:      "task-memory",
			WorkflowID:  workflowID,
			TraceID:     traceID,
			Actor:       "memory_steward",
		},
		Confidence: MemoryConfidence{
			Score:     0.9,
			Rationale: "stable recurring signal",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func assertNoDurableMemoryArtifacts(t *testing.T, ctx context.Context, stores *SQLiteMemoryStores, memoryID string, model string) {
	t.Helper()
	if _, ok, err := stores.Store.Get(ctx, memoryID); err != nil {
		t.Fatalf("get memory: %v", err)
	} else if ok {
		t.Fatalf("expected no persisted memory record for %s", memoryID)
	}
	if embedding, ok, err := stores.Embeddings.LoadEmbedding(ctx, memoryID, model); err != nil {
		t.Fatalf("load embedding: %v", err)
	} else if ok {
		t.Fatalf("expected no persisted embedding, got %+v", embedding)
	}
	lexical, err := stores.Query.SearchLexical(ctx, []string{"subscription"}, 5)
	if err != nil {
		t.Fatalf("search lexical: %v", err)
	}
	for _, item := range lexical {
		if item.MemoryID == memoryID {
			t.Fatalf("expected no lexical postings for %s, got %+v", memoryID, lexical)
		}
	}
	audit, err := stores.Audit.ListAccess(ctx, memoryID)
	if err != nil {
		t.Fatalf("list access audit: %v", err)
	}
	if len(audit) != 0 {
		t.Fatalf("expected no durable access audit for %s, got %+v", memoryID, audit)
	}
	events, err := stores.WriteEvents.ListWriteEvents(ctx, memoryID)
	if err != nil {
		t.Fatalf("list write events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no durable write events for %s, got %+v", memoryID, events)
	}
}

func mustExecSQLite(t *testing.T, db *sql.DB, stmt string) {
	t.Helper()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("exec sqlite statement: %v", err)
	}
}
