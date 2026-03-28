package verification

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

func TestVerificationArtifactsRoundTrip(t *testing.T) {
	now := time.Date(2026, 3, 28, 13, 0, 0, 0, time.UTC)
	result := VerificationResult{
		Status:    VerificationStatusNeedsReplan,
		Validator: "evidence-coverage-checker",
		Message:   "Tax evidence is incomplete.",
		EvidenceCoverage: EvidenceCoverageReport{
			TaskID:        "task-1",
			CoverageRatio: 0.5,
			Items: []EvidenceCoverageItem{
				{RequirementID: "tax-document", Covered: false, GapReason: "No W-2 evidence supplied"},
				{RequirementID: "ledger", Covered: true, EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-1")}},
			},
		},
		CheckedAt: now,
	}
	verdict := OracleVerdict{
		Scenario:  "missing-tax-evidence",
		Passed:    false,
		Score:     0.4,
		Reasons:   []string{"evidence gap remained unresolved"},
		CheckedAt: now,
	}

	resultData, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal verification result: %v", err)
	}
	var decodedResult VerificationResult
	if err := json.Unmarshal(resultData, &decodedResult); err != nil {
		t.Fatalf("unmarshal verification result: %v", err)
	}
	if err := decodedResult.Validate(); err != nil {
		t.Fatalf("decoded verification result should validate: %v", err)
	}

	verdictData, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("marshal oracle verdict: %v", err)
	}
	var decodedVerdict OracleVerdict
	if err := json.Unmarshal(verdictData, &decodedVerdict); err != nil {
		t.Fatalf("unmarshal oracle verdict: %v", err)
	}
	if err := decodedVerdict.Validate(); err != nil {
		t.Fatalf("decoded oracle verdict should validate: %v", err)
	}
}
