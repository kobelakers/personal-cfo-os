package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestPlannerAndCashflowQueryBuildersProduceDistinctQueries(t *testing.T) {
	input := QueryBuildInput{
		WorkflowID: "workflow-1",
		Task: taskspec.TaskSpec{
			ID:    "task-1",
			Goal:  "请帮我做一份月度财务复盘",
			Scope: taskspec.TaskScope{Areas: []string{"cashflow", "debt"}},
		},
		State: state.FinancialWorldState{
			BehaviorState: state.BehaviorState{
				DuplicateSubscriptionCount: 2,
				LateNightSpendingFrequency: 0.22,
			},
			LiabilityState: state.LiabilityState{DebtBurdenRatio: 0.24},
			CashflowState:  state.CashflowState{MonthlyNetIncomeCents: -1200},
		},
		Evidence: []observation.EvidenceRecord{
			retrievalEvidence("evidence-subscription", observation.EvidenceTypeRecurringSubscription, "subscriptions"),
			retrievalEvidence("evidence-late-night", observation.EvidenceTypeLateNightSpendingSignal, "late night"),
		},
		TraceID: "trace-1",
	}
	planning := PlannerMemoryQueryBuilder{}.Build(input)
	cashflow := CashflowMemoryQueryBuilder{}.Build(input)

	if planning.Consumer != MemoryConsumerPlanner || cashflow.Consumer != MemoryConsumerCashflow {
		t.Fatalf("expected distinct consumers, got planning=%+v cashflow=%+v", planning, cashflow)
	}
	if planning.ContextView == cashflow.ContextView {
		t.Fatalf("expected distinct context views, got planning=%+v cashflow=%+v", planning, cashflow)
	}
	if planning.RetrievalPolicy == cashflow.RetrievalPolicy {
		t.Fatalf("expected distinct retrieval policies, got planning=%+v cashflow=%+v", planning, cashflow)
	}
	if planning.SemanticHint == cashflow.SemanticHint {
		t.Fatalf("expected distinct semantic hints, got planning=%+v cashflow=%+v", planning, cashflow)
	}
}

func TestHybridRetrievalFusionAndPolicyDrivenRejection(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	stores, err := NewSQLiteMemoryStores(SQLiteStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("open sqlite memory stores: %v", err)
	}
	defer func() { _ = stores.DB.Close() }()

	records := []MemoryRecord{
		{
			ID:      "memory-selected",
			Kind:    MemoryKindSemantic,
			Summary: "重复订阅需要继续作为现金流优化重点。",
			Facts:   []MemoryFact{{Key: "duplicate_subscription_count", Value: "2", EvidenceID: observation.EvidenceID("evidence-subscription")}},
			Source:  MemorySource{TaskID: "task-1", WorkflowID: "workflow-1", Actor: "memory-steward"},
			Confidence: MemoryConfidence{
				Score:     0.92,
				Rationale: "repeated monthly review signal",
			},
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now.Add(-24 * time.Hour),
		},
		{
			ID:      "memory-stale",
			Kind:    MemoryKindEpisodic,
			Summary: "很久以前记录过一次深夜消费波动。",
			Facts:   []MemoryFact{{Key: "late_night_spending_frequency", Value: "0.4", EvidenceID: observation.EvidenceID("evidence-late-night")}},
			Source:  MemorySource{TaskID: "task-old", WorkflowID: "workflow-old", Actor: "memory-steward"},
			Confidence: MemoryConfidence{
				Score:     0.91,
				Rationale: "old episodic memory",
			},
			CreatedAt: now.Add(-120 * 24 * time.Hour),
			UpdatedAt: now.Add(-120 * 24 * time.Hour),
		},
		{
			ID:      "memory-low-confidence",
			Kind:    MemoryKindEpisodic,
			Summary: "一次弱信号提到可选订阅也许存在重复。",
			Facts:   []MemoryFact{{Key: "duplicate_subscription_count", Value: "1", EvidenceID: observation.EvidenceID("evidence-subscription")}},
			Source:  MemorySource{TaskID: "task-low", WorkflowID: "workflow-low", Actor: "memory-steward"},
			Confidence: MemoryConfidence{
				Score:     0.45,
				Rationale: "weak signal",
			},
			CreatedAt: now.Add(-48 * time.Hour),
			UpdatedAt: now.Add(-48 * time.Hour),
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
	}
	if _, err := indexer.BackfillLexicalTerms(ctx); err != nil {
		t.Fatalf("backfill lexical terms: %v", err)
	}
	if _, err := indexer.BackfillEmbeddings(ctx); err != nil {
		t.Fatalf("backfill embeddings: %v", err)
	}

	retriever := HybridMemoryRetriever{
		Lexical: LexicalRetriever{
			Query: stores.Query,
			Audit: stores.Audit,
		},
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
					Name:                  "monthly_review_freshness",
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
		QueryID:         "query-1",
		WorkflowID:      "workflow-1",
		TaskID:          "task-1",
		TraceID:         "trace-1",
		Consumer:        MemoryConsumerCashflow,
		ContextView:     "execution",
		Text:            "请优先关注重复订阅和夜间消费记忆",
		LexicalTerms:    []string{"subscriptions", "late-night", "spending"},
		SemanticHint:    "cashflow recommendation framing",
		TopK:            5,
		RetrievalPolicy: "monthly_review_cashflow",
		FreshnessPolicy: "monthly_review_freshness",
	})
	if err != nil {
		t.Fatalf("hybrid retrieval: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("expected retrieval results including rejected items, got %+v", results)
	}
	var selectedResult *RetrievalResult
	rejected := map[string]RetrievalResult{}
	for i := range results {
		if !results[i].Rejected && results[i].MemoryID == "memory-selected" {
			selectedResult = &results[i]
		}
		if results[i].Rejected {
			rejected[results[i].MemoryID] = results[i]
		}
	}
	if selectedResult == nil {
		t.Fatalf("expected recent semantic memory to survive retrieval, got %+v", results)
	}
	if rejected["memory-stale"].RejectionRule == "" || rejected["memory-stale"].FreshnessAgeDays < 90 {
		t.Fatalf("expected stale memory rejection metadata, got %+v", rejected["memory-stale"])
	}
	if rejected["memory-low-confidence"].RejectionRule == "" || rejected["memory-low-confidence"].RejectionReason == "" {
		t.Fatalf("expected low-confidence rejection metadata, got %+v", rejected["memory-low-confidence"])
	}
}

func retrievalEvidence(id string, evidenceType observation.EvidenceType, summary string) observation.EvidenceRecord {
	return observation.EvidenceRecord{
		ID:            observation.EvidenceID(id),
		Type:          evidenceType,
		Summary:       summary,
		Source:        observation.EvidenceSource{Kind: "ledger", Adapter: "test", Reference: id, Provenance: "fixture"},
		TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC)},
		Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
		Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
	}
}
