package observation

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvidenceRecordRoundTrip(t *testing.T) {
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	record := EvidenceRecord{
		ID:   EvidenceID("evidence-001"),
		Type: EvidenceTypeLedgerTransaction,
		Source: EvidenceSource{
			Kind:       "ledger",
			Adapter:    "csv-import",
			Reference:  "stmt-2026-03",
			Provenance: "import-run-1",
		},
		TimeRange: EvidenceTimeRange{
			ObservedAt: now,
			Start:      &now,
			End:        &now,
		},
		Confidence: EvidenceConfidence{
			Score:  0.92,
			Reason: "matched imported statement line",
		},
		Artifact: &EvidenceArtifactRef{
			ObjectKey: "artifacts/stmt.pdf",
			MediaType: "application/pdf",
			Checksum:  "sha256:abc",
		},
		Claims: []EvidenceClaim{
			{Subject: "cashflow", Predicate: "net_income_cents", Object: "monthly", ValueJSON: "1250000"},
		},
		Normalization: EvidenceNormalizationResult{
			Status:        EvidenceNormalizationNormalized,
			CanonicalUnit: "cents",
			Notes:         []string{"currency normalized to CNY cents"},
		},
		Summary:   "March payroll imported",
		CreatedAt: now,
	}

	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}

	var decoded EvidenceRecord
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}

	if decoded.ID != record.ID || decoded.Source.Reference != record.Source.Reference {
		t.Fatalf("round-trip lost identity or source: %+v", decoded)
	}
	if decoded.Artifact == nil || decoded.Artifact.ObjectKey != record.Artifact.ObjectKey {
		t.Fatalf("round-trip lost artifact ref: %+v", decoded.Artifact)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded evidence should be valid: %v", err)
	}
}

func TestEvidenceNormalizationStatusesValidate(t *testing.T) {
	cases := []EvidenceNormalizationResult{
		{Status: EvidenceNormalizationNormalized},
		{Status: EvidenceNormalizationPartial},
		{Status: EvidenceNormalizationRejected, RejectedReason: "insufficient fields"},
	}

	for _, item := range cases {
		if err := item.Validate(); err != nil {
			t.Fatalf("expected normalization status %q to validate: %v", item.Status, err)
		}
	}
}
