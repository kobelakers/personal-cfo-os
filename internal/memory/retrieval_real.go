package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type EmbeddedSemanticSearchBackend struct {
	Provider   EmbeddingProvider
	Embeddings MemoryEmbeddingStore
	Model      string
	Scorer     RetrievalScorer
}

func (b EmbeddedSemanticSearchBackend) Search(ctx context.Context, query RetrievalQuery, records []MemoryRecord) ([]RetrievalResult, error) {
	if b.Provider == nil {
		return nil, fmt.Errorf("semantic search backend requires embedding provider")
	}
	if b.Embeddings == nil {
		return nil, fmt.Errorf("semantic search backend requires embedding store")
	}
	if b.Model == "" {
		return nil, fmt.Errorf("semantic search backend requires embedding model")
	}
	scorer := b.Scorer
	if scorer == nil {
		scorer = DefaultRetrievalScorer{}
	}
	queryResponse, err := b.Provider.GenerateEmbedding(ctx, EmbeddingRequest{
		Model:      b.Model,
		Input:      stringsTrimSpaceJoin(query.Text, query.SemanticHint),
		WorkflowID: query.WorkflowID,
		TaskID:     query.TaskID,
		TraceID:    query.TraceID,
		Actor:      fallbackString(query.Consumer, "semantic_retriever"),
		QueryID:    query.QueryID,
	})
	if err != nil {
		return nil, err
	}
	embeddings, err := b.Embeddings.ListEmbeddings(ctx, b.Model)
	if err != nil {
		return nil, err
	}
	recordByID := make(map[string]MemoryRecord, len(records))
	for _, record := range records {
		recordByID[record.ID] = record
	}
	results := make([]RetrievalResult, 0, len(embeddings))
	for _, embedding := range embeddings {
		record, ok := recordByID[embedding.MemoryID]
		if !ok {
			continue
		}
		score := cosineSimilarity(queryResponse.Vector, embedding.Vector)
		boost, reason := scorer.Score(query, record)
		fused := roundTo((score+boost)/2, 4)
		results = append(results, RetrievalResult{
			MemoryID:      record.ID,
			Score:         fused,
			SemanticScore: score,
			FusedScore:    fused,
			Reason:        reason,
			Memory:        record,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].MemoryID < results[j].MemoryID
		}
		return results[i].Score > results[j].Score
	})
	return topK(results, query.TopK), nil
}

type DefaultMemoryIndexer struct {
	Store       MemoryStore
	Embeddings  MemoryEmbeddingStore
	Writer      MemoryWriteEventStore
	Provider    EmbeddingProvider
	Model       string
	Now         func() time.Time
	WorkflowID  string
	TaskID      string
	TraceID     string
}

func (i DefaultMemoryIndexer) BackfillEmbeddings(ctx context.Context) (IndexBuildSummary, error) {
	start := nowUTC(i.Now)
	if i.Store == nil || i.Embeddings == nil || i.Provider == nil {
		return IndexBuildSummary{}, fmt.Errorf("memory indexer requires store, embeddings, and provider")
	}
	if i.Model == "" {
		return IndexBuildSummary{}, fmt.Errorf("memory indexer requires embedding model")
	}
	records, err := i.Store.List(ctx)
	if err != nil {
		return IndexBuildSummary{}, err
	}
	summary := IndexBuildSummary{RecordsScanned: len(records), StartedAt: start}
	for _, record := range records {
		response, err := i.Provider.GenerateEmbedding(ctx, EmbeddingRequest{
			Model:      i.Model,
			Input:      semanticText(record),
			WorkflowID: i.WorkflowID,
			TaskID:     i.TaskID,
			TraceID:    i.TraceID,
			Actor:      "memory_indexer",
		})
		if err != nil {
			return summary, err
		}
		if err := i.Embeddings.SaveEmbedding(ctx, MemoryEmbeddingRecord{
			MemoryID:    record.ID,
			Provider:    response.Provider,
			Model:       response.Model,
			Vector:      response.Vector,
			Dimensions:  response.Dimensions,
			EmbeddedAt:  nowUTC(i.Now),
			ContentHash: contentHash(semanticText(record)),
		}); err != nil {
			return summary, err
		}
		summary.EmbeddingsBuilt++
		summary.Provider = response.Provider
		summary.Model = response.Model
	}
	summary.RecordsIndexed = summary.EmbeddingsBuilt
	summary.CompletedAt = nowUTC(i.Now)
	if i.Writer != nil {
		_ = i.Writer.AppendWriteEvent(ctx, MemoryWriteEvent{
			ID:         fmt.Sprintf("memory-index-embeddings-%d", summary.CompletedAt.UnixNano()),
			MemoryID:   "all",
			WorkflowID: i.WorkflowID,
			TaskID:     i.TaskID,
			TraceID:    i.TraceID,
			Action:     "backfill_embeddings",
			Summary:    fmt.Sprintf("indexed %d memory embeddings", summary.EmbeddingsBuilt),
			Provider:   summary.Provider,
			Model:      summary.Model,
			OccurredAt: summary.CompletedAt,
			Details:    fmt.Sprintf("records_scanned=%d", summary.RecordsScanned),
		})
	}
	return summary, nil
}

func (i DefaultMemoryIndexer) BackfillLexicalTerms(ctx context.Context) (IndexBuildSummary, error) {
	start := nowUTC(i.Now)
	if i.Store == nil || i.Embeddings == nil {
		return IndexBuildSummary{}, fmt.Errorf("memory indexer requires store and embedding store")
	}
	records, err := i.Store.List(ctx)
	if err != nil {
		return IndexBuildSummary{}, err
	}
	summary := IndexBuildSummary{RecordsScanned: len(records), StartedAt: start}
	for _, record := range records {
		if err := i.Embeddings.ReplaceTerms(ctx, record.ID, tokenFrequency(semanticText(record))); err != nil {
			return summary, err
		}
		summary.RecordsIndexed++
		summary.TermsBuilt += len(tokenFrequency(semanticText(record)))
	}
	summary.CompletedAt = nowUTC(i.Now)
	if i.Writer != nil {
		_ = i.Writer.AppendWriteEvent(ctx, MemoryWriteEvent{
			ID:         fmt.Sprintf("memory-index-terms-%d", summary.CompletedAt.UnixNano()),
			MemoryID:   "all",
			WorkflowID: i.WorkflowID,
			TaskID:     i.TaskID,
			TraceID:    i.TraceID,
			Action:     "backfill_terms",
			Summary:    fmt.Sprintf("indexed lexical terms for %d records", summary.RecordsIndexed),
			OccurredAt: summary.CompletedAt,
			Details:    fmt.Sprintf("terms_built=%d", summary.TermsBuilt),
		})
	}
	return summary, nil
}

func (i DefaultMemoryIndexer) RebuildIndexes(ctx context.Context) (IndexBuildSummary, error) {
	if i.Embeddings == nil {
		return IndexBuildSummary{}, fmt.Errorf("memory indexer requires embedding store")
	}
	if i.Model != "" {
		if err := i.Embeddings.DeleteEmbeddings(ctx, i.Model); err != nil {
			return IndexBuildSummary{}, err
		}
	}
	terms, err := i.BackfillLexicalTerms(ctx)
	if err != nil {
		return IndexBuildSummary{}, err
	}
	embeddings, err := i.BackfillEmbeddings(ctx)
	if err != nil {
		return IndexBuildSummary{}, err
	}
	return IndexBuildSummary{
		RecordsScanned:  maxInt(terms.RecordsScanned, embeddings.RecordsScanned),
		RecordsIndexed:  maxInt(terms.RecordsIndexed, embeddings.RecordsIndexed),
		EmbeddingsBuilt: embeddings.EmbeddingsBuilt,
		TermsBuilt:      terms.TermsBuilt,
		Provider:        embeddings.Provider,
		Model:           embeddings.Model,
		StartedAt:       terms.StartedAt,
		CompletedAt:     embeddings.CompletedAt,
	}, nil
}

func stringsTrimSpaceJoin(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return strings.Join(filtered, " ")
}
