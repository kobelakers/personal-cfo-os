package observation

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestLedgerObservationAdapterObserveProducesTypedEvidence(t *testing.T) {
	transactionsData := mustReadFixture(t, "ledger_transactions_2026-03.csv")
	debtsData := mustReadFixture(t, "debts_2026-03.csv")
	holdingsData := mustReadFixture(t, "holdings_2026-03.csv")

	transactions, err := LoadLedgerTransactionsCSV(transactionsData)
	if err != nil {
		t.Fatalf("load transactions: %v", err)
	}
	debts, err := LoadDebtRecordsCSV(debtsData)
	if err != nil {
		t.Fatalf("load debts: %v", err)
	}
	holdings, err := LoadHoldingRecordsCSV(holdingsData)
	if err != nil {
		t.Fatalf("load holdings: %v", err)
	}

	adapter := LedgerObservationAdapter{
		Transactions: transactions,
		Debts:        debts,
		Holdings:     holdings,
		Now: func() time.Time {
			return time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)
		},
	}
	records, err := adapter.Observe(t.Context(), ObservationRequest{
		TaskID:     "task-monthly-review",
		SourceKind: "ledger",
		Params: map[string]string{
			"user_id": "user-1",
			"start":   "2026-03-01",
			"end":     "2026-03-31T23:59:59Z",
		},
	})
	if err != nil {
		t.Fatalf("observe ledger: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("expected 5 ledger evidence records, got %d", len(records))
	}
	expectedTypes := map[EvidenceType]bool{
		EvidenceTypeTransactionBatch:        false,
		EvidenceTypeRecurringSubscription:   false,
		EvidenceTypeLateNightSpendingSignal: false,
		EvidenceTypeDebtObligationSnapshot:  false,
		EvidenceTypePortfolioAllocationSnap: false,
	}
	for _, record := range records {
		expectedTypes[record.Type] = true
		if err := record.Validate(); err != nil {
			t.Fatalf("ledger evidence should validate: %v", err)
		}
	}
	for evidenceType, seen := range expectedTypes {
		if !seen {
			t.Fatalf("expected evidence type %q to be emitted", evidenceType)
		}
	}
}

func TestDocumentObservationAdaptersProduceTypedEvidence(t *testing.T) {
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	structuredAdapter := StructuredDocumentObservationAdapter{
		Artifacts: []DocumentArtifact{
			{
				ID:         "doc-payslip",
				UserID:     "user-1",
				Kind:       DocumentKindPayslip,
				Filename:   "payslip_2026-03.csv",
				MediaType:  "text/csv",
				Content:    mustReadFixture(t, "payslip_2026-03.csv"),
				ObservedAt: now,
			},
			{
				ID:         "doc-credit",
				UserID:     "user-1",
				Kind:       DocumentKindCreditCardStatement,
				Filename:   "credit_card_2026-03.csv",
				MediaType:  "text/csv",
				Content:    mustReadFixture(t, "credit_card_2026-03.csv"),
				ObservedAt: now,
			},
			{
				ID:         "doc-broker",
				UserID:     "user-1",
				Kind:       DocumentKindBrokerStatement,
				Filename:   "broker_statement_2026-03.csv",
				MediaType:  "text/csv",
				Content:    mustReadFixture(t, "broker_statement_2026-03.csv"),
				ObservedAt: now,
			},
		},
	}
	agenticAdapter := AgenticDocumentObservationAdapterStub{
		Artifacts: []DocumentArtifact{
			{
				ID:         "doc-tax",
				UserID:     "user-1",
				Kind:       DocumentKindTaxDocument,
				Filename:   "tax_2026.txt",
				MediaType:  "text/plain",
				Content:    mustReadFixture(t, "tax_2026.txt"),
				ObservedAt: now,
			},
		},
	}

	structuredRecords, err := structuredAdapter.Observe(t.Context(), ObservationRequest{
		TaskID:     "task-monthly-review",
		SourceKind: "document",
		Params: map[string]string{
			"user_id": "user-1",
		},
	})
	if err != nil {
		t.Fatalf("observe structured docs: %v", err)
	}
	if len(structuredRecords) != 3 {
		t.Fatalf("expected 3 structured doc evidence records, got %d", len(structuredRecords))
	}
	agenticRecords, err := agenticAdapter.Observe(t.Context(), ObservationRequest{
		TaskID:     "task-monthly-review",
		SourceKind: "document",
		Params: map[string]string{
			"user_id": "user-1",
		},
	})
	if err != nil {
		t.Fatalf("observe agentic docs: %v", err)
	}
	if len(agenticRecords) != 1 {
		t.Fatalf("expected 1 agentic doc evidence record, got %d", len(agenticRecords))
	}
	if agenticRecords[0].Type != EvidenceTypeTaxDocument {
		t.Fatalf("expected tax document evidence, got %q", agenticRecords[0].Type)
	}
}

func TestEventAndDeadlineAdaptersProduceTypedEvidence(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	eventAdapter := EventObservationAdapter{
		Events: []LifeEventRecord{
			{
				ID:         "evt-salary-1",
				UserID:     "user-1",
				Kind:       LifeEventSalaryChange,
				Source:     "hris",
				Provenance: "fixture:salary_change",
				ObservedAt: now,
				Confidence: 0.95,
				SalaryChange: &SalaryChangeEventPayload{
					PreviousMonthlyIncomeCents: 900000,
					NewMonthlyIncomeCents:      1100000,
					EffectiveAt:                now.AddDate(0, 0, 7),
				},
			},
		},
		Now: func() time.Time { return now },
	}
	deadlineAdapter := CalendarDeadlineObservationAdapter{
		Deadlines: []CalendarDeadlineRecord{
			{
				ID:               "ddl-tax-1",
				UserID:           "user-1",
				Kind:             "withholding_review",
				RelatedEventID:   "evt-salary-1",
				RelatedEventKind: LifeEventSalaryChange,
				Source:           "calendar",
				Provenance:       "fixture:deadline",
				ObservedAt:       now,
				DeadlineAt:       now.AddDate(0, 0, 21),
				Description:      "review withholding after salary change",
				Confidence:       0.9,
			},
		},
		Now: func() time.Time { return now },
	}
	eventRecords, err := eventAdapter.Observe(t.Context(), ObservationRequest{
		TaskID:     "task-life-event",
		SourceKind: "event",
		Params:     map[string]string{"user_id": "user-1", "event_kind": string(LifeEventSalaryChange)},
	})
	if err != nil {
		t.Fatalf("observe event records: %v", err)
	}
	if len(eventRecords) != 1 || eventRecords[0].Type != EvidenceTypeEventSignal {
		t.Fatalf("expected one event_signal evidence record, got %+v", eventRecords)
	}
	if err := eventRecords[0].Validate(); err != nil {
		t.Fatalf("event evidence should validate: %v", err)
	}

	deadlineRecords, err := deadlineAdapter.Observe(t.Context(), ObservationRequest{
		TaskID:     "task-life-event",
		SourceKind: "calendar_deadline",
		Params:     map[string]string{"user_id": "user-1", "event_id": "evt-salary-1"},
	})
	if err != nil {
		t.Fatalf("observe deadline records: %v", err)
	}
	if len(deadlineRecords) != 1 || deadlineRecords[0].Type != EvidenceTypeCalendarDeadline {
		t.Fatalf("expected one calendar deadline evidence record, got %+v", deadlineRecords)
	}
	if err := deadlineRecords[0].Validate(); err != nil {
		t.Fatalf("deadline evidence should validate: %v", err)
	}
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
