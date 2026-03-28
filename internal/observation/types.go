package observation

import "time"

type EvidenceID string

type EvidenceType string

const (
	EvidenceTypeLedgerTransaction       EvidenceType = "ledger_transaction"
	EvidenceTypeDocumentStatement       EvidenceType = "document_statement"
	EvidenceTypeEventSignal             EvidenceType = "event_signal"
	EvidenceTypeMarketPolicy            EvidenceType = "market_policy"
	EvidenceTypeCalendarDeadline        EvidenceType = "calendar_deadline"
	EvidenceTypeExtractedTable          EvidenceType = "extracted_table"
	EvidenceTypeAgenticParse            EvidenceType = "agentic_parse"
	EvidenceTypeTransactionBatch        EvidenceType = "transaction_batch"
	EvidenceTypeRecurringSubscription   EvidenceType = "recurring_subscription_signal"
	EvidenceTypeLateNightSpendingSignal EvidenceType = "late_night_spending_signal"
	EvidenceTypeDebtObligationSnapshot  EvidenceType = "debt_obligation_snapshot"
	EvidenceTypePortfolioAllocationSnap EvidenceType = "portfolio_allocation_snapshot"
	EvidenceTypePayslipStatement        EvidenceType = "payslip_statement"
	EvidenceTypeCreditCardStatement     EvidenceType = "credit_card_statement"
	EvidenceTypeTaxDocument             EvidenceType = "tax_document"
	EvidenceTypeBrokerStatement         EvidenceType = "broker_statement"
)

type DocumentKind string

const (
	DocumentKindPayslip             DocumentKind = "payslip"
	DocumentKindCreditCardStatement DocumentKind = "credit_card_statement"
	DocumentKindTaxDocument         DocumentKind = "tax_document"
	DocumentKindBrokerStatement     DocumentKind = "broker_statement"
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

type LedgerTransactionRecord struct {
	UserID        string    `json:"user_id"`
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	OccurredAt    time.Time `json:"occurred_at"`
	Description   string    `json:"description"`
	Merchant      string    `json:"merchant"`
	Category      string    `json:"category"`
	Direction     string    `json:"direction"`
	AmountCents   int64     `json:"amount_cents"`
}

type DebtRecord struct {
	UserID          string    `json:"user_id"`
	AccountID       string    `json:"account_id"`
	Name            string    `json:"name"`
	BalanceCents    int64     `json:"balance_cents"`
	AnnualRate      float64   `json:"annual_rate"`
	MinimumDueCents int64     `json:"minimum_due_cents"`
	SnapshotAt      time.Time `json:"snapshot_at"`
}

type HoldingRecord struct {
	UserID           string    `json:"user_id"`
	AccountID        string    `json:"account_id"`
	SnapshotAt       time.Time `json:"snapshot_at"`
	AssetClass       string    `json:"asset_class"`
	Symbol           string    `json:"symbol"`
	MarketValueCents int64     `json:"market_value_cents"`
	TargetAllocation float64   `json:"target_allocation"`
}

type DocumentArtifact struct {
	ID         string       `json:"id"`
	UserID     string       `json:"user_id"`
	Kind       DocumentKind `json:"kind"`
	Filename   string       `json:"filename"`
	MediaType  string       `json:"media_type"`
	Content    []byte       `json:"content"`
	ObservedAt time.Time    `json:"observed_at"`
}
