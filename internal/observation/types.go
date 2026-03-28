package observation

import "time"

type EvidenceID string

type EvidenceType string

const (
	EvidenceTypeLedgerTransaction EvidenceType = "ledger_transaction"
	EvidenceTypeDocumentStatement EvidenceType = "document_statement"
	EvidenceTypeEventSignal       EvidenceType = "event_signal"
	EvidenceTypeMarketPolicy      EvidenceType = "market_policy"
	EvidenceTypeCalendarDeadline  EvidenceType = "calendar_deadline"
	EvidenceTypeExtractedTable    EvidenceType = "extracted_table"
	EvidenceTypeAgenticParse      EvidenceType = "agentic_parse"
)

type EvidenceSource struct {
	Kind       string `json:"kind"`
	Adapter    string `json:"adapter"`
	Reference  string `json:"reference"`
	Provenance string `json:"provenance"`
}

type EvidenceTimeRange struct {
	ObservedAt time.Time  `json:"observed_at"`
	Start      *time.Time `json:"start,omitempty"`
	End        *time.Time `json:"end,omitempty"`
}

type EvidenceConfidence struct {
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

type EvidenceArtifactRef struct {
	ObjectKey string `json:"object_key"`
	MediaType string `json:"media_type"`
	Checksum  string `json:"checksum"`
}

type EvidenceClaim struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
	ValueJSON string `json:"value_json,omitempty"`
}

type EvidenceNormalizationStatus string

const (
	EvidenceNormalizationNormalized EvidenceNormalizationStatus = "normalized"
	EvidenceNormalizationPartial    EvidenceNormalizationStatus = "partial"
	EvidenceNormalizationRejected   EvidenceNormalizationStatus = "rejected"
)

type EvidenceNormalizationResult struct {
	Status         EvidenceNormalizationStatus `json:"status"`
	CanonicalUnit  string                      `json:"canonical_unit,omitempty"`
	Notes          []string                    `json:"notes,omitempty"`
	RejectedReason string                      `json:"rejected_reason,omitempty"`
}

type EvidenceRecord struct {
	ID            EvidenceID                  `json:"id"`
	Type          EvidenceType                `json:"type"`
	Source        EvidenceSource              `json:"source"`
	TimeRange     EvidenceTimeRange           `json:"time_range"`
	Confidence    EvidenceConfidence          `json:"confidence"`
	Artifact      *EvidenceArtifactRef        `json:"artifact,omitempty"`
	Claims        []EvidenceClaim             `json:"claims"`
	Normalization EvidenceNormalizationResult `json:"normalization"`
	Summary       string                      `json:"summary"`
	CreatedAt     time.Time                   `json:"created_at"`
}

type ObservationRequest struct {
	TaskID     string            `json:"task_id"`
	SourceKind string            `json:"source_kind"`
	Params     map[string]string `json:"params,omitempty"`
}

type RawObservation struct {
	Source  EvidenceSource `json:"source"`
	Payload []byte         `json:"payload"`
}
