package memory

import "context"

type EmbeddingProvider interface {
	GenerateEmbedding(ctx context.Context, request EmbeddingRequest) (EmbeddingResponse, error)
	Embed(ctx context.Context, text string) ([]float64, error)
}

type VectorIndex interface {
	Upsert(ctx context.Context, id string, vector []float64) error
	Search(ctx context.Context, vector []float64, topK int) ([]RetrievalResult, error)
}

type RetrievalScorer interface {
	Score(query RetrievalQuery, record MemoryRecord) (float64, string)
}

type SemanticSearchBackend interface {
	Search(ctx context.Context, query RetrievalQuery, records []MemoryRecord) ([]RetrievalResult, error)
}

type MemoryStore interface {
	Put(ctx context.Context, record MemoryRecord) error
	Get(ctx context.Context, id string) (MemoryRecord, bool, error)
	List(ctx context.Context) ([]MemoryRecord, error)
}

type MemoryWriteCommitter interface {
	CommitPreparedWrite(ctx context.Context, prepared PreparedMemoryWrite) (DurableWriteResult, error)
}

type MemoryQueryStore interface {
	LoadByIDs(ctx context.Context, ids []string) ([]MemoryRecord, error)
	ListByKind(ctx context.Context, filter MemoryListFilter) ([]MemoryRecord, error)
	ListRecent(ctx context.Context, limit int) ([]MemoryRecord, error)
	SearchLexical(ctx context.Context, terms []string, limit int) ([]LexicalCandidate, error)
}

type MemoryRelationStore interface {
	SaveRelations(ctx context.Context, memoryID string, relations MemoryRelations) error
	LoadRelations(ctx context.Context, memoryID string) (MemoryRelations, error)
}

type MemoryAuditStore interface {
	AppendAccess(ctx context.Context, audit MemoryAccessAudit) error
	ListAccess(ctx context.Context, memoryID string) ([]MemoryAccessAudit, error)
}

type MemoryWriteEventStore interface {
	AppendWriteEvent(ctx context.Context, event MemoryWriteEvent) error
	ListWriteEvents(ctx context.Context, memoryID string) ([]MemoryWriteEvent, error)
}

type MemoryEmbeddingStore interface {
	SaveEmbedding(ctx context.Context, record MemoryEmbeddingRecord) error
	LoadEmbedding(ctx context.Context, memoryID string, model string) (MemoryEmbeddingRecord, bool, error)
	ListEmbeddings(ctx context.Context, model string) ([]MemoryEmbeddingRecord, error)
	DeleteEmbeddings(ctx context.Context, model string) error
	ReplaceTerms(ctx context.Context, memoryID string, terms map[string]int) error
	LoadIndexedModels(ctx context.Context) ([]string, error)
}

type HybridRetriever interface {
	Retrieve(ctx context.Context, query RetrievalQuery) ([]RetrievalResult, error)
}

type RankFusionStrategy interface {
	Fuse(lexical []RetrievalResult, semantic []RetrievalResult) ([]RetrievalResult, error)
}

type Reranker interface {
	Rerank(ctx context.Context, query RetrievalQuery, results []RetrievalResult) ([]RetrievalResult, error)
}

type RejectionPolicy interface {
	Reject(ctx context.Context, query RetrievalQuery, results []RetrievalResult) ([]RetrievalResult, error)
}

type MemoryWriter interface {
	Write(ctx context.Context, record MemoryRecord) error
}

type ConflictDetector interface {
	Detect(existing []MemoryRecord, candidate MemoryRecord) []ConflictRef
}

type SupersedenceDetector interface {
	Detect(existing []MemoryRecord, candidate MemoryRecord) []SupersedesRef
}

type MemoryWriteGate interface {
	AllowWrite(ctx context.Context, record MemoryRecord) error
}

type MemoryTraceRecorder interface {
	RecordMemoryQuery(record MemoryQueryRecord)
	RecordMemoryRetrieval(record MemoryRetrievalRecord)
	RecordMemorySelection(record MemorySelectionRecord)
}

type MemoryIndexer interface {
	BackfillEmbeddings(ctx context.Context) (IndexBuildSummary, error)
	BackfillLexicalTerms(ctx context.Context) (IndexBuildSummary, error)
	RebuildIndexes(ctx context.Context) (IndexBuildSummary, error)
}
