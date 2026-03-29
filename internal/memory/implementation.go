package memory

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type InMemoryMemoryStore struct {
	mu      sync.RWMutex
	records map[string]MemoryRecord
}

func NewInMemoryMemoryStore() *InMemoryMemoryStore {
	return &InMemoryMemoryStore{
		records: make(map[string]MemoryRecord),
	}
}

func (s *InMemoryMemoryStore) Put(_ context.Context, record MemoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.ID] = record
	return nil
}

func (s *InMemoryMemoryStore) Get(_ context.Context, id string) (MemoryRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	return record, ok, nil
}

func (s *InMemoryMemoryStore) List(_ context.Context) ([]MemoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := make([]MemoryRecord, 0, len(s.records))
	for _, record := range s.records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, nil
}

func (s *InMemoryMemoryStore) CommitPreparedWrite(_ context.Context, prepared PreparedMemoryWrite) (DurableWriteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[prepared.Record.ID] = prepared.Record
	return DurableWriteResult{
		MemoryID:     prepared.Record.ID,
		CommittedAt:  prepared.Record.UpdatedAt,
		EmbeddingSet: prepared.Embedding != nil,
		TermsSet:     len(prepared.Terms) > 0,
	}, nil
}

type MemoryAccessAuditLog struct {
	mu      sync.Mutex
	entries []MemoryAccessAudit
}

func (l *MemoryAccessAuditLog) Append(entry MemoryAccessAudit) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *MemoryAccessAuditLog) Entries() []MemoryAccessAudit {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]MemoryAccessAudit, len(l.entries))
	copy(result, l.entries)
	return result
}

type DefaultMemoryWriter struct {
	Store                      MemoryStore
	Committer                  MemoryWriteCommitter
	Relations                  MemoryRelationStore
	AuditStore                 MemoryAuditStore
	WriteEventStore            MemoryWriteEventStore
	EmbeddingStore             MemoryEmbeddingStore
	EmbeddingProvider          EmbeddingProvider
	EmbeddingModel             string
	AuditLog                   *MemoryAccessAuditLog
	ConflictDetector           ConflictDetector
	SupersedenceDetector       SupersedenceDetector
	MinConfidence              float64
	LowConfidenceEpisodicFloor float64
	Now                        func() time.Time
}

func (w DefaultMemoryWriter) Write(ctx context.Context, record MemoryRecord) error {
	if w.Store == nil {
		return fmt.Errorf("memory writer requires a store")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if len(record.Source.EvidenceIDs) == 0 && record.Source.TaskID == "" && record.Source.WorkflowID == "" {
		return fmt.Errorf("memory provenance is required")
	}
	minConfidence := w.MinConfidence
	if minConfidence == 0 {
		minConfidence = 0.75
	}
	episodicFloor := w.LowConfidenceEpisodicFloor
	if episodicFloor == 0 {
		episodicFloor = 0.55
	}
	if record.Confidence.Score < minConfidence {
		if !(record.Kind == MemoryKindEpisodic && record.Confidence.Score >= episodicFloor) {
			return fmt.Errorf("memory confidence %.2f below writer threshold %.2f", record.Confidence.Score, minConfidence)
		}
	}
	record.UpdatedAt = nowUTC(w.Now)
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}

	existing, err := w.Store.List(ctx)
	if err != nil {
		return err
	}
	record.Conflicts = dedupeConflictRefs(append(record.Conflicts, w.conflictDetector().Detect(existing, record)...))
	record.Supersedes = dedupeSupersedesRefs(append(record.Supersedes, w.supersedenceDetector().Detect(existing, record)...))
	prepared, err := w.prepareWrite(ctx, record)
	if err != nil {
		return err
	}
	if _, err := w.committer().CommitPreparedWrite(ctx, prepared); err != nil {
		return err
	}
	if w.AuditLog != nil {
		if prepared.Audit != nil {
			w.AuditLog.Append(*prepared.Audit)
		}
	}
	return nil
}

func (w DefaultMemoryWriter) prepareWrite(ctx context.Context, record MemoryRecord) (PreparedMemoryWrite, error) {
	prepared := PreparedMemoryWrite{
		Record: record,
		Relations: MemoryRelations{
			Relations:  record.Relations,
			Supersedes: record.Supersedes,
			Conflicts:  record.Conflicts,
		},
		Audit: &MemoryAccessAudit{
			ID:         fmt.Sprintf("%s-write-%d", record.ID, record.UpdatedAt.UnixNano()),
			MemoryID:   record.ID,
			WorkflowID: record.Source.WorkflowID,
			TaskID:     record.Source.TaskID,
			TraceID:    record.Source.TraceID,
			Accessor:   fallbackString(record.Source.Actor, "memory_writer"),
			Purpose:    "persist memory record",
			Action:     "write",
			AccessedAt: record.UpdatedAt,
		},
		WriteEvents: []MemoryWriteEvent{{
			ID:         fmt.Sprintf("%s-write-event-%d", record.ID, record.UpdatedAt.UnixNano()),
			MemoryID:   record.ID,
			WorkflowID: record.Source.WorkflowID,
			TaskID:     record.Source.TaskID,
			TraceID:    record.Source.TraceID,
			Action:     "write",
			Summary:    "persisted durable memory record",
			OccurredAt: record.UpdatedAt,
			Details:    fmt.Sprintf("kind=%s confidence=%.2f", record.Kind, record.Confidence.Score),
		}},
	}
	if w.EmbeddingProvider != nil && strings.TrimSpace(w.EmbeddingModel) != "" {
		response, err := w.EmbeddingProvider.GenerateEmbedding(ctx, EmbeddingRequest{
			Model:      w.EmbeddingModel,
			Input:      semanticText(record),
			WorkflowID: record.Source.WorkflowID,
			TaskID:     record.Source.TaskID,
			TraceID:    record.Source.TraceID,
			Actor:      fallbackString(record.Source.Actor, "memory_writer"),
		})
		if err != nil {
			return PreparedMemoryWrite{}, err
		}
		prepared.Embedding = &MemoryEmbeddingRecord{
			MemoryID:    record.ID,
			Provider:    response.Provider,
			Model:       response.Model,
			Vector:      response.Vector,
			Dimensions:  response.Dimensions,
			EmbeddedAt:  nowUTC(w.Now),
			ContentHash: contentHash(semanticText(record)),
		}
		prepared.Terms = tokenFrequency(semanticText(record))
		prepared.WriteEvents = append(prepared.WriteEvents, MemoryWriteEvent{
			ID:         fmt.Sprintf("%s-index-%d", record.ID, record.UpdatedAt.UnixNano()),
			MemoryID:   record.ID,
			WorkflowID: record.Source.WorkflowID,
			TaskID:     record.Source.TaskID,
			TraceID:    record.Source.TraceID,
			Action:     "index_embedding_and_terms",
			Summary:    "indexed memory for hybrid retrieval",
			Provider:   response.Provider,
			Model:      response.Model,
			OccurredAt: nowUTC(w.Now),
			Details:    fmt.Sprintf("dimensions=%d", response.Dimensions),
		})
	}
	return prepared, nil
}

func (w DefaultMemoryWriter) committer() MemoryWriteCommitter {
	if w.Committer != nil {
		return w.Committer
	}
	if committer, ok := w.Store.(MemoryWriteCommitter); ok {
		return committer
	}
	return fallbackMemoryWriteCommitter{
		store:          w.Store,
		relations:      w.Relations,
		auditStore:     w.AuditStore,
		writeEvents:    w.WriteEventStore,
		embeddingStore: w.EmbeddingStore,
	}
}

func (w DefaultMemoryWriter) conflictDetector() ConflictDetector {
	if w.ConflictDetector != nil {
		return w.ConflictDetector
	}
	return DefaultConflictDetector{}
}

func (w DefaultMemoryWriter) supersedenceDetector() SupersedenceDetector {
	if w.SupersedenceDetector != nil {
		return w.SupersedenceDetector
	}
	return DefaultSupersedenceDetector{}
}

type fallbackMemoryWriteCommitter struct {
	store          MemoryStore
	relations      MemoryRelationStore
	auditStore     MemoryAuditStore
	writeEvents    MemoryWriteEventStore
	embeddingStore MemoryEmbeddingStore
}

func (c fallbackMemoryWriteCommitter) CommitPreparedWrite(ctx context.Context, prepared PreparedMemoryWrite) (DurableWriteResult, error) {
	if c.store == nil {
		return DurableWriteResult{}, fmt.Errorf("memory write committer requires a store")
	}
	if c.relations != nil || c.auditStore != nil || c.writeEvents != nil || c.embeddingStore != nil {
		return DurableWriteResult{}, fmt.Errorf("memory write requires a transactional committer when auxiliary durable stores are configured")
	}
	if err := c.store.Put(ctx, prepared.Record); err != nil {
		return DurableWriteResult{}, err
	}
	return DurableWriteResult{
		MemoryID:     prepared.Record.ID,
		CommittedAt:  prepared.Record.UpdatedAt,
		EmbeddingSet: prepared.Embedding != nil,
		TermsSet:     len(prepared.Terms) > 0,
	}, nil
}

type LexicalRetriever struct {
	Store    MemoryStore
	Query    MemoryQueryStore
	Audit    MemoryAuditStore
	AuditLog *MemoryAccessAuditLog
}

func (r LexicalRetriever) Retrieve(ctx context.Context, query RetrievalQuery) ([]RetrievalResult, error) {
	if r.Query != nil {
		candidates, err := r.Query.SearchLexical(ctx, queryTokens(query), query.TopK*2)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(candidates))
		for _, item := range candidates {
			ids = append(ids, item.MemoryID)
		}
		records, err := r.Query.LoadByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		recordByID := make(map[string]MemoryRecord, len(records))
		for _, record := range records {
			recordByID[record.ID] = record
		}
		results := make([]RetrievalResult, 0, len(candidates))
		for _, candidate := range candidates {
			record, ok := recordByID[candidate.MemoryID]
			if !ok {
				continue
			}
			results = append(results, RetrievalResult{
				MemoryID:     candidate.MemoryID,
				Score:        candidate.Score,
				LexicalScore: candidate.Score,
				FusedScore:   candidate.Score,
				Reason:       "lexical bm25-like scoring",
				MatchedTerms: append([]string{}, candidate.MatchedTerms...),
				Memory:       record,
			})
		}
		sortResults(results)
		logMemoryResults(ctx, r.AuditLog, r.Audit, query, results, "retrieve_lexical", fallbackString(query.Text, "lexical lookup"))
		return topK(results, query.TopK), nil
	}
	records, err := r.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	tokens := queryTokens(query)
	results := make([]RetrievalResult, 0, len(records))
	for _, record := range records {
		score := lexicalScore(tokens, record)
		if score == 0 {
			continue
		}
		results = append(results, RetrievalResult{
			MemoryID:     record.ID,
			Score:        score,
			LexicalScore: score,
			FusedScore:   score,
			Reason:       "lexical token overlap",
			MatchedTerms: matchedTerms(tokens, record),
			Memory:       record,
		})
	}
	sortResults(results)
	logMemoryResults(ctx, r.AuditLog, r.Audit, query, results, "retrieve_lexical", fallbackString(query.Text, "lexical lookup"))
	return topK(results, query.TopK), nil
}

type SemanticRetriever struct {
	Store    MemoryStore
	Backend  SemanticSearchBackend
	Audit    MemoryAuditStore
	AuditLog *MemoryAccessAuditLog
}

func (r SemanticRetriever) Retrieve(ctx context.Context, query RetrievalQuery) ([]RetrievalResult, error) {
	if r.Backend == nil {
		return nil, fmt.Errorf("semantic retriever requires backend")
	}
	records, err := r.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	results, err := r.Backend.Search(ctx, query, records)
	if err != nil {
		return nil, err
	}
	sortResults(results)
	for i := range results {
		if results[i].SemanticScore == 0 {
			results[i].SemanticScore = results[i].Score
		}
		if results[i].FusedScore == 0 {
			results[i].FusedScore = results[i].Score
		}
	}
	logMemoryResults(ctx, r.AuditLog, r.Audit, query, results, "retrieve_semantic", fallbackString(query.SemanticHint, "semantic lookup"))
	return topK(results, query.TopK), nil
}

type ReciprocalRankFusion struct {
	K float64
}

func (f ReciprocalRankFusion) Fuse(lexical []RetrievalResult, semantic []RetrievalResult) ([]RetrievalResult, error) {
	k := f.K
	if k == 0 {
		k = 60
	}
	combined := map[string]RetrievalResult{}
	accumulate := func(results []RetrievalResult, source string) {
		for rank, result := range results {
			existing, ok := combined[result.MemoryID]
			score := 1.0 / (k + float64(rank+1))
			sourceScore := result.Score
			if ok {
				existing.Score += score
				existing.FusedScore = existing.Score
				existing.Reason = existing.Reason + "+" + source
				if source == "lexical" {
					existing.LexicalScore = sourceScore
				}
				if source == "semantic" {
					existing.SemanticScore = sourceScore
				}
				combined[result.MemoryID] = existing
				continue
			}
			result.Score = score
			result.FusedScore = score
			result.Reason = "rrf:" + source
			if source == "lexical" {
				result.LexicalScore = sourceScore
			}
			if source == "semantic" {
				result.SemanticScore = sourceScore
			}
			combined[result.MemoryID] = result
		}
	}
	accumulate(lexical, "lexical")
	accumulate(semantic, "semantic")

	results := make([]RetrievalResult, 0, len(combined))
	for _, result := range combined {
		results = append(results, result)
	}
	sortResults(results)
	return results, nil
}

type BaselineReranker struct{}

func (BaselineReranker) Rerank(_ context.Context, query RetrievalQuery, results []RetrievalResult) ([]RetrievalResult, error) {
	tokens := queryTokens(query)
	for i := range results {
		if containsAllTokens(strings.ToLower(results[i].Memory.Summary), tokens) {
			results[i].Score += 0.2
			results[i].Reason += "+summary_boost"
		}
	}
	sortResults(results)
	return results, nil
}

type ThresholdRejectionPolicy struct {
	MinScore      float64
	DefaultPolicy RetrievalPolicy
	Policies      map[string]RetrievalPolicy
	Now           func() time.Time
}

func (p ThresholdRejectionPolicy) Reject(_ context.Context, query RetrievalQuery, results []RetrievalResult) ([]RetrievalResult, error) {
	threshold := p.MinScore
	if threshold == 0 {
		threshold = 0.015
	}
	policy := p.resolvePolicy(query)
	if policy.FreshnessPolicy.MinAcceptedFusedScore > 0 {
		threshold = policy.FreshnessPolicy.MinAcceptedFusedScore
	}
	now := nowUTC(p.Now)
	for i := range results {
		if results[i].FusedScore == 0 {
			results[i].FusedScore = results[i].Score
		}
		if results[i].FusedScore < threshold {
			results[i].Rejected = true
			results[i].RejectionRule = "low_fused_score"
			results[i].RejectionReason = fmt.Sprintf("fused score %.4f below threshold %.4f", results[i].FusedScore, threshold)
			continue
		}
		if policy.FreshnessPolicy.RejectLowConfidence && results[i].Memory.Confidence.Score < policy.FreshnessPolicy.LowConfidenceFloor {
			results[i].Rejected = true
			results[i].RejectionRule = "low_confidence"
			results[i].RejectionReason = fmt.Sprintf("confidence %.2f below floor %.2f", results[i].Memory.Confidence.Score, policy.FreshnessPolicy.LowConfidenceFloor)
			continue
		}
		if policy.FreshnessPolicy.EpisodicMaxAge > 0 && results[i].Memory.Kind == MemoryKindEpisodic && !results[i].Memory.UpdatedAt.IsZero() {
			age := now.Sub(results[i].Memory.UpdatedAt)
			results[i].FreshnessAgeDays = int(age.Hours() / 24)
			if age > policy.FreshnessPolicy.EpisodicMaxAge {
				results[i].Rejected = true
				results[i].RejectionRule = "stale_episodic"
				results[i].RejectionReason = fmt.Sprintf("episodic memory age %s exceeds policy %s", age.Truncate(24*time.Hour), policy.FreshnessPolicy.EpisodicMaxAge)
			}
		}
	}
	if len(results) == 0 {
		return []RetrievalResult{{
			Rejected:        true,
			RejectionRule:   "empty_hit",
			RejectionReason: "retrieval returned no candidates",
		}}, nil
	}
	return results, nil
}

func (p ThresholdRejectionPolicy) resolvePolicy(query RetrievalQuery) RetrievalPolicy {
	if query.RetrievalPolicy != "" && p.Policies != nil {
		if policy, ok := p.Policies[query.RetrievalPolicy]; ok {
			return policy
		}
	}
	policy := p.DefaultPolicy
	if policy.Name == "" {
		policy = RetrievalPolicy{
			Name: "default",
			FreshnessPolicy: FreshnessPolicy{
				Name:                  fallbackString(query.FreshnessPolicy, "default"),
				EpisodicMaxAge:        90 * 24 * time.Hour,
				RejectLowConfidence:   true,
				LowConfidenceFloor:    0.7,
				MinAcceptedFusedScore: p.MinScore,
			},
		}
	}
	if policy.FreshnessPolicy.Name == "" {
		policy.FreshnessPolicy.Name = fallbackString(query.FreshnessPolicy, "default")
	}
	return policy
}

type HybridMemoryRetriever struct {
	Lexical         HybridRetriever
	Semantic        HybridRetriever
	Fusion          RankFusionStrategy
	Reranker        Reranker
	RejectionPolicy RejectionPolicy
}

func (r HybridMemoryRetriever) Retrieve(ctx context.Context, query RetrievalQuery) ([]RetrievalResult, error) {
	expandedQuery := query
	expandedQuery.TopK = retrievalCandidateWindow(query.TopK)

	lexical, err := r.Lexical.Retrieve(ctx, expandedQuery)
	if err != nil {
		return nil, err
	}
	semantic, err := r.Semantic.Retrieve(ctx, expandedQuery)
	if err != nil {
		return nil, err
	}
	fused, err := r.Fusion.Fuse(lexical, semantic)
	if err != nil {
		return nil, err
	}
	reranked, err := r.Reranker.Rerank(ctx, query, fused)
	if err != nil {
		return nil, err
	}
	results, err := r.RejectionPolicy.Reject(ctx, query, reranked)
	if err != nil {
		return nil, err
	}
	accepted := make([]int, 0, len(results))
	for i := range results {
		if !results[i].Rejected && results[i].MemoryID != "" {
			accepted = append(accepted, i)
		}
	}
	if len(accepted) == 0 && len(results) > 0 {
		for i := range results {
			results[i].Selected = false
		}
		if allCandidatesRejected(results) {
			results = append(results, RetrievalResult{
				Rejected:        true,
				RejectionRule:   "fully_rejected",
				RejectionReason: "all fused candidates were rejected before final topK selection",
			})
		}
		return results, nil
	}
	for index, resultIndex := range accepted {
		if query.TopK > 0 && index >= query.TopK {
			break
		}
		results[resultIndex].Selected = true
	}
	return results, nil
}

func retrievalCandidateWindow(topK int) int {
	switch {
	case topK <= 0:
		return 8
	case topK < 3:
		return topK + 5
	default:
		return maxInt(topK*3, topK+4)
	}
}

type KeywordEmbeddingProvider struct {
	Dimensions int
}

func (p KeywordEmbeddingProvider) Embed(_ context.Context, text string) ([]float64, error) {
	dims := p.Dimensions
	if dims == 0 {
		dims = 16
	}
	vector := make([]float64, dims)
	for _, token := range tokenizeForIndexing(text) {
		index := tokenHash(token) % dims
		vector[index] += 1
	}
	return normalizeVector(vector), nil
}

func (p KeywordEmbeddingProvider) GenerateEmbedding(ctx context.Context, request EmbeddingRequest) (EmbeddingResponse, error) {
	vector, err := p.Embed(contextWithEmbeddingRequest(ctx, request), request.Input)
	if err != nil {
		return EmbeddingResponse{}, err
	}
	return EmbeddingResponse{
		Provider:   "static-keyword",
		Model:      fallbackString(request.Model, "keyword-hash"),
		Vector:     vector,
		Dimensions: len(vector),
		Usage: EmbeddingUsageRecord{
			Provider:    "static-keyword",
			Model:       fallbackString(request.Model, "keyword-hash"),
			WorkflowID:  request.WorkflowID,
			TaskID:      request.TaskID,
			TraceID:     request.TraceID,
			Actor:       request.Actor,
			QueryID:     request.QueryID,
			InputTokens: maxInt(len(tokenizeForIndexing(request.Input)), 1),
			RecordedAt:  time.Now().UTC(),
		},
		Latency: 5 * time.Millisecond,
	}, nil
}

type InMemoryVectorIndex struct {
	mu      sync.RWMutex
	vectors map[string][]float64
}

func NewInMemoryVectorIndex() *InMemoryVectorIndex {
	return &InMemoryVectorIndex{
		vectors: make(map[string][]float64),
	}
}

func (i *InMemoryVectorIndex) Upsert(_ context.Context, id string, vector []float64) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	copied := make([]float64, len(vector))
	copy(copied, vector)
	i.vectors[id] = copied
	return nil
}

func (i *InMemoryVectorIndex) Search(_ context.Context, vector []float64, limit int) ([]RetrievalResult, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	results := make([]RetrievalResult, 0, len(i.vectors))
	for id, candidate := range i.vectors {
		results = append(results, RetrievalResult{
			MemoryID: id,
			Score:    cosineSimilarity(vector, candidate),
			Reason:   "vector similarity",
		})
	}
	sortResults(results)
	return topK(results, limit), nil
}

type DefaultRetrievalScorer struct{}

func (DefaultRetrievalScorer) Score(query RetrievalQuery, record MemoryRecord) (float64, string) {
	text := strings.ToLower(query.Text + " " + query.SemanticHint)
	recordText := strings.ToLower(record.Summary + " " + flattenFacts(record.Facts))
	shared := 0
	for _, token := range tokenizeForQuery(text) {
		if strings.Contains(recordText, token) {
			shared++
		}
	}
	if shared == 0 {
		return 0.01, "semantic baseline floor"
	}
	return math.Min(0.99, 0.2+float64(shared)*0.12), "semantic scorer token affinity"
}

type FakeSemanticSearchBackend struct {
	Provider EmbeddingProvider
	Index    VectorIndex
	Scorer   RetrievalScorer
}

func (b FakeSemanticSearchBackend) Search(ctx context.Context, query RetrievalQuery, records []MemoryRecord) ([]RetrievalResult, error) {
	if b.Provider == nil {
		return nil, fmt.Errorf("semantic search backend requires embedding provider")
	}
	if b.Index == nil {
		return nil, fmt.Errorf("semantic search backend requires vector index")
	}
	scorer := b.Scorer
	if scorer == nil {
		scorer = DefaultRetrievalScorer{}
	}
	for _, record := range records {
		vector, err := b.Provider.Embed(ctx, semanticText(record))
		if err != nil {
			return nil, err
		}
		if err := b.Index.Upsert(ctx, record.ID, vector); err != nil {
			return nil, err
		}
	}
	queryVector, err := b.Provider.Embed(ctx, query.Text+" "+query.SemanticHint)
	if err != nil {
		return nil, err
	}
	baseResults, err := b.Index.Search(ctx, queryVector, maxInt(query.TopK*2, query.TopK))
	if err != nil {
		return nil, err
	}

	recordByID := make(map[string]MemoryRecord, len(records))
	for _, record := range records {
		recordByID[record.ID] = record
	}
	results := make([]RetrievalResult, 0, len(baseResults))
	for _, result := range baseResults {
		record, ok := recordByID[result.MemoryID]
		if !ok {
			continue
		}
		boost, reason := scorer.Score(query, record)
		result.Score = roundTo((result.Score+boost)/2, 4)
		result.SemanticScore = result.Score
		result.FusedScore = result.Score
		result.Reason = reason
		result.Memory = record
		results = append(results, result)
	}
	sortResults(results)
	return topK(results, query.TopK), nil
}

func queryTokens(query RetrievalQuery) []string {
	parts := make([]string, 0, len(query.LexicalTerms)+8)
	parts = append(parts, query.LexicalTerms...)
	parts = append(parts, tokenizeForQuery(query.Text)...)
	return dedupeTokens(parts)
}

func lexicalScore(tokens []string, record MemoryRecord) float64 {
	recordText := strings.ToLower(record.Summary + " " + flattenFacts(record.Facts))
	matches := 0
	for _, token := range tokens {
		if strings.Contains(recordText, token) {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	return roundTo(float64(matches)/float64(maxInt(len(tokens), 1)), 4)
}

func containsAllTokens(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

func normalizeTokens(text string) []string {
	replacer := strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "\n", " ", "\t", " ", "/", " ", "_", " ")
	text = replacer.Replace(strings.ToLower(text))
	return strings.Fields(text)
}

func tokenizeForIndexing(text string) []string {
	return normalizeTokens(text)
}

func tokenizeForQuery(text string) []string {
	return dedupeTokens(normalizeTokens(text))
}

func dedupeTokens(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func semanticText(record MemoryRecord) string {
	return record.Summary + " " + flattenFacts(record.Facts)
}

func flattenFacts(facts []MemoryFact) string {
	parts := make([]string, 0, len(facts))
	for _, fact := range facts {
		parts = append(parts, fact.Key+" "+fact.Value)
	}
	return strings.Join(parts, " ")
}

func sortResults(results []RetrievalResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].MemoryID < results[j].MemoryID
		}
		return results[i].Score > results[j].Score
	})
}

func topK(results []RetrievalResult, limit int) []RetrievalResult {
	if limit <= 0 || limit >= len(results) {
		return results
	}
	return results[:limit]
}

func logMemoryResults(ctx context.Context, log *MemoryAccessAuditLog, store MemoryAuditStore, query RetrievalQuery, results []RetrievalResult, action string, purpose string) {
	now := time.Now().UTC()
	for _, result := range results {
		if result.MemoryID == "" {
			continue
		}
		entry := MemoryAccessAudit{
			ID:         fmt.Sprintf("%s-%s-%d", result.MemoryID, action, now.UnixNano()),
			MemoryID:   result.MemoryID,
			WorkflowID: query.WorkflowID,
			TaskID:     query.TaskID,
			TraceID:    query.TraceID,
			QueryID:    query.QueryID,
			Accessor:   "hybrid_retriever",
			Purpose:    purpose,
			Action:     action,
			Reason:     result.Reason,
			Score:      result.FusedScore,
			AccessedAt: now,
		}
		if store != nil {
			_ = store.AppendAccess(ctx, entry)
		}
		if log != nil {
			log.Append(entry)
		}
	}
}

func tokenHash(token string) int {
	hash := 17
	for _, char := range token {
		hash = hash*31 + int(char)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

func normalizeVector(vector []float64) []float64 {
	sumSquares := 0.0
	for _, value := range vector {
		sumSquares += value * value
	}
	if sumSquares == 0 {
		return vector
	}
	norm := math.Sqrt(sumSquares)
	result := make([]float64, len(vector))
	for i, value := range vector {
		result[i] = value / norm
	}
	return result
}

func cosineSimilarity(left []float64, right []float64) float64 {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	if limit == 0 {
		return 0
	}
	sum := 0.0
	for i := 0; i < limit; i++ {
		sum += left[i] * right[i]
	}
	return roundTo(sum, 4)
}

func roundTo(value float64, precision int) float64 {
	scale := math.Pow10(precision)
	return math.Round(value*scale) / scale
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func matchedTerms(tokens []string, record MemoryRecord) []string {
	recordText := strings.ToLower(record.Summary + " " + flattenFacts(record.Facts))
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if strings.Contains(recordText, token) {
			result = append(result, token)
		}
	}
	return dedupeTokens(result)
}

func tokenFrequency(text string) map[string]int {
	freq := make(map[string]int)
	for _, token := range tokenizeForIndexing(text) {
		freq[token]++
	}
	return freq
}

func allCandidatesRejected(results []RetrievalResult) bool {
	hasCandidate := false
	for _, result := range results {
		if result.MemoryID == "" {
			continue
		}
		hasCandidate = true
		if !result.Rejected {
			return false
		}
	}
	return hasCandidate
}

func nowUTC(nowFn func() time.Time) time.Time {
	if nowFn != nil {
		return nowFn().UTC()
	}
	return time.Now().UTC()
}

func contentHash(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}
