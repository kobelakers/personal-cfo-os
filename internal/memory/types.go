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
	MemoryID   string    `json:"memory_id"`
	Accessor   string    `json:"accessor"`
	Purpose    string    `json:"purpose"`
	AccessedAt time.Time `json:"accessed_at"`
}

type RetrievalQuery struct {
	Text         string   `json:"text"`
	LexicalTerms []string `json:"lexical_terms,omitempty"`
	SemanticHint string   `json:"semantic_hint,omitempty"`
	TopK         int      `json:"top_k"`
}

type RetrievalResult struct {
	MemoryID string  `json:"memory_id"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
	Rejected bool    `json:"rejected"`
}
