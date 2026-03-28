package memory

import "context"

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
