package memory

import (
	"context"
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
	AuditLog                   *MemoryAccessAuditLog
	MinConfidence              float64
	LowConfidenceEpisodicFloor float64
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
	record.UpdatedAt = time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}

	existing, err := w.Store.List(ctx)
	if err != nil {
		return err
	}
	record.Conflicts = append(record.Conflicts, detectConflicts(existing, record)...)
	record.Supersedes = append(record.Supersedes, detectSupersedes(existing, record)...)
	if err := w.Store.Put(ctx, record); err != nil {
		return err
	}
	if w.AuditLog != nil {
		w.AuditLog.Append(MemoryAccessAudit{
			MemoryID:   record.ID,
			Accessor:   fallbackString(record.Source.Actor, "memory_writer"),
			Purpose:    "persist memory record",
			Action:     "write",
			AccessedAt: record.UpdatedAt,
		})
	}
	return nil
}

type LexicalRetriever struct {
	Store    MemoryStore
	AuditLog *MemoryAccessAuditLog
}

func (r LexicalRetriever) Retrieve(ctx context.Context, query RetrievalQuery) ([]RetrievalResult, error) {
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
			MemoryID: record.ID,
			Score:    score,
			Reason:   "lexical token overlap",
			Memory:   record,
		})
	}
	sortResults(results)
	logMemoryResults(r.AuditLog, results, "retrieve_lexical", fallbackString(query.Text, "lexical lookup"))
	return topK(results, query.TopK), nil
}

type SemanticRetriever struct {
	Store    MemoryStore
	Backend  SemanticSearchBackend
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
	logMemoryResults(r.AuditLog, results, "retrieve_semantic", fallbackString(query.SemanticHint, "semantic lookup"))
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
			if ok {
				existing.Score += score
				existing.Reason = existing.Reason + "+" + source
				combined[result.MemoryID] = existing
				continue
			}
			result.Score = score
			result.Reason = "rrf:" + source
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
	MinScore float64
}

func (p ThresholdRejectionPolicy) Reject(_ context.Context, _ RetrievalQuery, results []RetrievalResult) ([]RetrievalResult, error) {
	threshold := p.MinScore
	if threshold == 0 {
		threshold = 0.015
	}
	filtered := make([]RetrievalResult, 0, len(results))
	for _, result := range results {
		if result.Score < threshold {
			result.Rejected = true
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered, nil
}

type HybridMemoryRetriever struct {
	Lexical         HybridRetriever
	Semantic        HybridRetriever
	Fusion          RankFusionStrategy
	Reranker        Reranker
	RejectionPolicy RejectionPolicy
}

func (r HybridMemoryRetriever) Retrieve(ctx context.Context, query RetrievalQuery) ([]RetrievalResult, error) {
	lexical, err := r.Lexical.Retrieve(ctx, query)
	if err != nil {
		return nil, err
	}
	semantic, err := r.Semantic.Retrieve(ctx, query)
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
	return r.RejectionPolicy.Reject(ctx, query, topK(reranked, query.TopK))
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
	for _, token := range tokenize(text) {
		index := tokenHash(token) % dims
		vector[index] += 1
	}
	return normalizeVector(vector), nil
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
	for _, token := range tokenize(text) {
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
		result.Reason = reason
		result.Memory = record
		results = append(results, result)
	}
	sortResults(results)
	return topK(results, query.TopK), nil
}

func detectConflicts(existing []MemoryRecord, candidate MemoryRecord) []ConflictRef {
	conflicts := make([]ConflictRef, 0)
	for _, record := range existing {
		if record.ID == candidate.ID || record.Kind != candidate.Kind {
			continue
		}
		for _, fact := range candidate.Facts {
			for _, existingFact := range record.Facts {
				if fact.Key == existingFact.Key && fact.Value != existingFact.Value {
					conflicts = append(conflicts, ConflictRef{
						MemoryID: record.ID,
						Reason:   "same fact key with different value",
					})
					goto nextRecord
				}
			}
		}
	nextRecord:
	}
	return conflicts
}

func detectSupersedes(existing []MemoryRecord, candidate MemoryRecord) []SupersedesRef {
	supersedes := make([]SupersedesRef, 0)
	for _, record := range existing {
		if record.ID == candidate.ID || record.Kind != candidate.Kind {
			continue
		}
		if strings.EqualFold(record.Summary, candidate.Summary) && candidate.UpdatedAt.After(record.UpdatedAt) {
			supersedes = append(supersedes, SupersedesRef{
				MemoryID: record.ID,
				Reason:   "same summary updated with newer evidence",
			})
		}
	}
	return supersedes
}

func queryTokens(query RetrievalQuery) []string {
	parts := make([]string, 0, len(query.LexicalTerms)+8)
	parts = append(parts, query.LexicalTerms...)
	parts = append(parts, tokenize(query.Text)...)
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

func tokenize(text string) []string {
	replacer := strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "\n", " ", "\t", " ", "/", " ", "_", " ")
	text = replacer.Replace(strings.ToLower(text))
	fields := strings.Fields(text)
	return dedupeTokens(fields)
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

func logMemoryResults(log *MemoryAccessAuditLog, results []RetrievalResult, action string, purpose string) {
	if log == nil {
		return
	}
	now := time.Now().UTC()
	for _, result := range results {
		log.Append(MemoryAccessAudit{
			MemoryID:   result.MemoryID,
			Accessor:   "hybrid_retriever",
			Purpose:    purpose,
			Action:     action,
			AccessedAt: now,
		})
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
