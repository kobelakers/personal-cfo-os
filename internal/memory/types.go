package memory

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

type MemoryKind string

const (
	MemoryKindEpisodic   MemoryKind = "episodic"
	MemoryKindSemantic   MemoryKind = "semantic"
	MemoryKindProcedural MemoryKind = "procedural"
	MemoryKindPolicy     MemoryKind = "policy"
)

type MemoryFact struct {
	Key        string                 `json:"key"`
	Value      string                 `json:"value"`
	EvidenceID observation.EvidenceID `json:"evidence_id,omitempty"`
}

type MemoryRelation struct {
	Type           string `json:"type"`
	TargetMemoryID string `json:"target_memory_id"`
	Description    string `json:"description,omitempty"`
}

type MemorySource struct {
	EvidenceIDs []observation.EvidenceID `json:"evidence_ids,omitempty"`
	TaskID      string                   `json:"task_id,omitempty"`
	WorkflowID  string                   `json:"workflow_id,omitempty"`
	TraceID     string                   `json:"trace_id,omitempty"`
	Actor       string                   `json:"actor,omitempty"`
}

type MemoryConfidence struct {
	Score     float64 `json:"score"`
	Rationale string  `json:"rationale"`
}

type SupersedesRef struct {
	MemoryID string `json:"memory_id"`
	Reason   string `json:"reason"`
}

type ConflictRef struct {
	MemoryID string `json:"memory_id"`
	Reason   string `json:"reason"`
}

type MemoryRecord struct {
	ID         string           `json:"id"`
	Kind       MemoryKind       `json:"kind"`
	Summary    string           `json:"summary"`
	Facts      []MemoryFact     `json:"facts"`
	Relations  []MemoryRelation `json:"relations,omitempty"`
	Source     MemorySource     `json:"source"`
	Confidence MemoryConfidence `json:"confidence"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	Supersedes []SupersedesRef  `json:"supersedes,omitempty"`
	Conflicts  []ConflictRef    `json:"conflicts_with,omitempty"`
}

type MemoryAccessAudit struct {
	ID         string    `json:"id,omitempty"`
	MemoryID   string    `json:"memory_id"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	TaskID     string    `json:"task_id,omitempty"`
	TraceID    string    `json:"trace_id,omitempty"`
	QueryID    string    `json:"query_id,omitempty"`
	Accessor   string    `json:"accessor"`
	Purpose    string    `json:"purpose"`
	Action     string    `json:"action"`
	Reason     string    `json:"reason,omitempty"`
	Score      float64   `json:"score,omitempty"`
	AccessedAt time.Time `json:"accessed_at"`
}

type MemoryWriteEvent struct {
	ID         string    `json:"id"`
	MemoryID   string    `json:"memory_id"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	TaskID     string    `json:"task_id,omitempty"`
	TraceID    string    `json:"trace_id,omitempty"`
	Action     string    `json:"action"`
	Summary    string    `json:"summary,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Model      string    `json:"model,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	Details    string    `json:"details,omitempty"`
}

type RetrievalQuery struct {
	QueryID          string   `json:"query_id,omitempty"`
	WorkflowID       string   `json:"workflow_id,omitempty"`
	TaskID           string   `json:"task_id,omitempty"`
	TraceID          string   `json:"trace_id,omitempty"`
	Consumer         string   `json:"consumer,omitempty"`
	ContextView      string   `json:"context_view,omitempty"`
	Text             string   `json:"text"`
	LexicalTerms     []string `json:"lexical_terms,omitempty"`
	SemanticHint     string   `json:"semantic_hint,omitempty"`
	TopK             int      `json:"top_k"`
	RetrievalPolicy  string   `json:"retrieval_policy,omitempty"`
	FreshnessPolicy  string   `json:"freshness_policy,omitempty"`
	EmbeddingModel   string   `json:"embedding_model,omitempty"`
	EmbeddingProfile string   `json:"embedding_profile,omitempty"`
}

type RetrievalResult struct {
	MemoryID         string       `json:"memory_id"`
	Score            float64      `json:"score"`
	LexicalScore     float64      `json:"lexical_score,omitempty"`
	SemanticScore    float64      `json:"semantic_score,omitempty"`
	FusedScore       float64      `json:"fused_score,omitempty"`
	Reason           string       `json:"reason"`
	Rejected         bool         `json:"rejected"`
	RejectionRule    string       `json:"rejection_rule,omitempty"`
	RejectionReason  string       `json:"rejection_reason,omitempty"`
	Selected         bool         `json:"selected"`
	MatchedTerms     []string     `json:"matched_terms,omitempty"`
	FreshnessAgeDays int          `json:"freshness_age_days,omitempty"`
	Memory           MemoryRecord `json:"memory"`
}

type MemoryQueryRecord struct {
	QueryID          string    `json:"query_id"`
	WorkflowID       string    `json:"workflow_id,omitempty"`
	TaskID           string    `json:"task_id,omitempty"`
	TraceID          string    `json:"trace_id,omitempty"`
	Consumer         string    `json:"consumer,omitempty"`
	ContextView      string    `json:"context_view,omitempty"`
	Text             string    `json:"text"`
	LexicalTerms     []string  `json:"lexical_terms,omitempty"`
	SemanticHint     string    `json:"semantic_hint,omitempty"`
	TopK             int       `json:"top_k"`
	RetrievalPolicy  string    `json:"retrieval_policy,omitempty"`
	FreshnessPolicy  string    `json:"freshness_policy,omitempty"`
	EmbeddingModel   string    `json:"embedding_model,omitempty"`
	EmbeddingProfile string    `json:"embedding_profile,omitempty"`
	OccurredAt       time.Time `json:"occurred_at"`
}

type MemoryRetrievalRecord struct {
	QueryID          string            `json:"query_id"`
	WorkflowID       string            `json:"workflow_id,omitempty"`
	TaskID           string            `json:"task_id,omitempty"`
	TraceID          string            `json:"trace_id,omitempty"`
	Consumer         string            `json:"consumer,omitempty"`
	RetrievalPolicy  string            `json:"retrieval_policy,omitempty"`
	FreshnessPolicy  string            `json:"freshness_policy,omitempty"`
	FusionSummary    string            `json:"fusion_summary,omitempty"`
	SelectedMemoryID []string          `json:"selected_memory_ids,omitempty"`
	RejectedMemoryID []string          `json:"rejected_memory_ids,omitempty"`
	Results          []RetrievalResult `json:"results,omitempty"`
	OccurredAt       time.Time         `json:"occurred_at"`
}

type MemorySelectionRecord struct {
	QueryID           string    `json:"query_id"`
	WorkflowID        string    `json:"workflow_id,omitempty"`
	TaskID            string    `json:"task_id,omitempty"`
	TraceID           string    `json:"trace_id,omitempty"`
	Consumer          string    `json:"consumer,omitempty"`
	SelectedMemoryIDs []string  `json:"selected_memory_ids,omitempty"`
	RejectedMemoryIDs []string  `json:"rejected_memory_ids,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	OccurredAt        time.Time `json:"occurred_at"`
}

type MemoryRelations struct {
	Relations  []MemoryRelation `json:"relations,omitempty"`
	Supersedes []SupersedesRef  `json:"supersedes,omitempty"`
	Conflicts  []ConflictRef    `json:"conflicts_with,omitempty"`
}

type MemoryEmbeddingRecord struct {
	MemoryID    string    `json:"memory_id"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	Vector      []float64 `json:"vector"`
	Dimensions  int       `json:"dimensions"`
	EmbeddedAt  time.Time `json:"embedded_at"`
	ContentHash string    `json:"content_hash,omitempty"`
}

// PreparedMemoryWrite belongs to the memory layer, not the workflow layer.
// It lets the subsystem compute embeddings, relations, terms, and audit records
// first, then commit all durable memory state atomically in one store-level step.
type PreparedMemoryWrite struct {
	Record      MemoryRecord           `json:"record"`
	Relations   MemoryRelations        `json:"relations,omitempty"`
	Embedding   *MemoryEmbeddingRecord `json:"embedding,omitempty"`
	Terms       map[string]int         `json:"terms,omitempty"`
	Audit       *MemoryAccessAudit     `json:"audit,omitempty"`
	WriteEvents []MemoryWriteEvent     `json:"write_events,omitempty"`
}

type DurableWriteResult struct {
	MemoryID     string    `json:"memory_id"`
	CommittedAt  time.Time `json:"committed_at"`
	EmbeddingSet bool      `json:"embedding_set,omitempty"`
	TermsSet     bool      `json:"terms_set,omitempty"`
}

type MemoryWriteContext struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	Actor      string `json:"actor,omitempty"`
}

type MemoryRetrievalContext struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	Consumer   string `json:"consumer,omitempty"`
	QueryID    string `json:"query_id,omitempty"`
}

type LexicalCandidate struct {
	MemoryID     string   `json:"memory_id"`
	Score        float64  `json:"score"`
	MatchedTerms []string `json:"matched_terms,omitempty"`
}

type MemoryListFilter struct {
	Kinds  []MemoryKind `json:"kinds,omitempty"`
	Limit  int          `json:"limit,omitempty"`
	Recent bool         `json:"recent,omitempty"`
}

type FreshnessPolicy struct {
	Name                  string        `json:"name"`
	EpisodicMaxAge        time.Duration `json:"episodic_max_age,omitempty"`
	RejectLowConfidence   bool          `json:"reject_low_confidence,omitempty"`
	LowConfidenceFloor    float64       `json:"low_confidence_floor,omitempty"`
	MinAcceptedFusedScore float64       `json:"min_accepted_fused_score,omitempty"`
}

type RetrievalPolicy struct {
	Name            string          `json:"name"`
	FreshnessPolicy FreshnessPolicy `json:"freshness_policy"`
}

type IndexBuildSummary struct {
	RecordsScanned  int       `json:"records_scanned"`
	RecordsIndexed  int       `json:"records_indexed"`
	EmbeddingsBuilt int       `json:"embeddings_built,omitempty"`
	TermsBuilt      int       `json:"terms_built,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
}
