package memory

import "context"

type EmbeddingProvider interface {
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

type MemoryWriteGate interface {
	AllowWrite(ctx context.Context, record MemoryRecord) error
}
