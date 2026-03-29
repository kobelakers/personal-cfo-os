package verification

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

type VerificationStatus string
type VerificationScope string

const (
	VerificationStatusPass        VerificationStatus = "pass"
	VerificationStatusFail        VerificationStatus = "fail"
	VerificationStatusNeedsReplan VerificationStatus = "needs_replan"
	VerificationStatusBlocked     VerificationStatus = "blocked"

	VerificationScopeBlock VerificationScope = "block"
	VerificationScopeFinal VerificationScope = "final"
)

type EvidenceCoverageItem struct {
	RequirementID string                   `json:"requirement_id"`
	Covered       bool                     `json:"covered"`
	EvidenceIDs   []observation.EvidenceID `json:"evidence_ids,omitempty"`
	GapReason     string                   `json:"gap_reason,omitempty"`
}

type EvidenceCoverageReport struct {
	TaskID        string                 `json:"task_id"`
	CoverageRatio float64                `json:"coverage_ratio"`
	Items         []EvidenceCoverageItem `json:"items"`
}

type VerificationResult struct {
	Status                  VerificationStatus     `json:"status"`
	Scope                   VerificationScope      `json:"scope,omitempty"`
	BlockID                 string                 `json:"block_id,omitempty"`
	BlockKind               string                 `json:"block_kind,omitempty"`
	Validator               string                 `json:"validator"`
	Message                 string                 `json:"message"`
	Details                 map[string]any         `json:"details,omitempty"`
	FailedRules             []string               `json:"failed_rules,omitempty"`
	MissingEvidence         []string               `json:"missing_evidence,omitempty"`
	RecommendedReplanAction string                 `json:"recommended_replan_action,omitempty"`
	Severity                string                 `json:"severity,omitempty"`
	EvidenceCoverage        EvidenceCoverageReport `json:"evidence_coverage"`
	CheckedAt               time.Time              `json:"checked_at"`
}

type OracleVerdict struct {
	Scenario  string    `json:"scenario"`
	Passed    bool      `json:"passed"`
	Score     float64   `json:"score"`
	Reasons   []string  `json:"reasons,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}
