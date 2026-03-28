package observation

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type LedgerObservationAdapter struct {
	AdapterName  string
	Transactions []LedgerTransactionRecord
	Debts        []DebtRecord
	Holdings     []HoldingRecord
	Extractor    EvidenceExtractor
	Normalizer   EvidenceNormalizer
	Now          func() time.Time
}

func (a LedgerObservationAdapter) SourceType() string {
	return "ledger"
}

func (a LedgerObservationAdapter) Observe(ctx context.Context, request ObservationRequest) ([]EvidenceRecord, error) {
	start, end := parseRequestTimeRange(request.Params)
	userID := strings.TrimSpace(request.Params["user_id"])

	transactions := filterTransactions(a.Transactions, userID, start, end)
	debts := filterDebts(a.Debts, userID, end)
	holdings := filterHoldings(a.Holdings, userID, end)

	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}

	extractor := a.Extractor
	if extractor == nil {
		extractor = TransactionEvidenceExtractor{}
	}
	normalizer := a.Normalizer
	if normalizer == nil {
		normalizer = CanonicalEvidenceNormalizer{}
	}

	records := make([]EvidenceRecord, 0, 5)
	if len(transactions) > 0 {
		batch, err := a.buildTransactionBatchEvidence(ctx, extractor, normalizer, request.TaskID, userID, transactions, now, start, end)
		if err != nil {
			return nil, err
		}
		records = append(records, batch)

		subscription, err := a.buildRecurringSubscriptionEvidence(ctx, normalizer, userID, transactions, now, start, end)
		if err != nil {
			return nil, err
		}
		records = append(records, subscription)

		lateNight, err := a.buildLateNightSpendingEvidence(ctx, normalizer, userID, transactions, now, start, end)
		if err != nil {
			return nil, err
		}
		records = append(records, lateNight)
	}
	if len(debts) > 0 {
		debtSnapshot, err := a.buildDebtSnapshotEvidence(ctx, normalizer, userID, debts, now, start, end)
		if err != nil {
			return nil, err
		}
		records = append(records, debtSnapshot)
	}
	if len(holdings) > 0 {
		portfolioSnapshot, err := a.buildPortfolioSnapshotEvidence(ctx, normalizer, userID, holdings, transactions, now, start, end)
		if err != nil {
			return nil, err
		}
		records = append(records, portfolioSnapshot)
	}
	return records, nil
}

func (a LedgerObservationAdapter) buildTransactionBatchEvidence(
	ctx context.Context,
	extractor EvidenceExtractor,
	normalizer EvidenceNormalizer,
	taskID string,
	userID string,
	transactions []LedgerTransactionRecord,
	now time.Time,
	start *time.Time,
	end *time.Time,
) (EvidenceRecord, error) {
	payload, err := json.Marshal(transactions)
	if err != nil {
		return EvidenceRecord{}, err
	}

	claims, err := extractor.Extract(ctx, RawObservation{
		Source: EvidenceSource{
			Kind:       "ledger:transaction_batch",
			Adapter:    a.SourceType(),
			Reference:  taskID,
			Provenance: "ledger_transactions",
		},
		Payload: payload,
	})
	if err != nil {
		return EvidenceRecord{}, err
	}

	record := EvidenceRecord{
		ID:   EvidenceID("evidence-transaction-batch-" + userID + "-" + now.Format("20060102150405")),
		Type: EvidenceTypeTransactionBatch,
		Source: EvidenceSource{
			Kind:       "ledger",
			Adapter:    a.SourceType(),
			Reference:  userID,
			Provenance: "ledger_transactions",
		},
		TimeRange: EvidenceTimeRange{
			ObservedAt: now,
			Start:      start,
			End:        end,
		},
		Confidence: EvidenceConfidence{
			Score:  0.98,
			Reason: "derived from structured transaction records",
		},
		Claims:    claims,
		Summary:   fmt.Sprintf("normalized %d transactions into monthly cashflow evidence", len(transactions)),
		CreatedAt: now,
	}
	return normalizer.Normalize(ctx, record)
}

func (a LedgerObservationAdapter) buildRecurringSubscriptionEvidence(
	ctx context.Context,
	normalizer EvidenceNormalizer,
	userID string,
	transactions []LedgerTransactionRecord,
	now time.Time,
	start *time.Time,
	end *time.Time,
) (EvidenceRecord, error) {
	merchants := recurringSubscriptionMerchants(transactions)
	countJSON, err := json.Marshal(len(merchants))
	if err != nil {
		return EvidenceRecord{}, err
	}
	merchantsJSON, err := json.Marshal(merchants)
	if err != nil {
		return EvidenceRecord{}, err
	}
	record := EvidenceRecord{
		ID:   EvidenceID("evidence-subscription-signal-" + userID + "-" + now.Format("20060102150405")),
		Type: EvidenceTypeRecurringSubscription,
		Source: EvidenceSource{
			Kind:       "ledger",
			Adapter:    a.SourceType(),
			Reference:  userID,
			Provenance: "ledger_transactions",
		},
		TimeRange: EvidenceTimeRange{
			ObservedAt: now,
			Start:      start,
			End:        end,
		},
		Confidence: EvidenceConfidence{
			Score:  0.9,
			Reason: "merchant/category heuristics over structured transactions",
		},
		Claims: []EvidenceClaim{
			{Subject: "behavior", Predicate: "duplicate_subscription_count", Object: "month", ValueJSON: string(countJSON)},
			{Subject: "behavior", Predicate: "recurring_subscription_merchants", Object: "month", ValueJSON: string(merchantsJSON)},
		},
		Normalization: EvidenceNormalizationResult{Status: EvidenceNormalizationNormalized},
		Summary:       fmt.Sprintf("detected %d recurring subscription merchants", len(merchants)),
		CreatedAt:     now,
	}
	return normalizer.Normalize(ctx, record)
}

func (a LedgerObservationAdapter) buildLateNightSpendingEvidence(
	ctx context.Context,
	normalizer EvidenceNormalizer,
	userID string,
	transactions []LedgerTransactionRecord,
	now time.Time,
	start *time.Time,
	end *time.Time,
) (EvidenceRecord, error) {
	outgoing := outgoingTransactions(transactions)
	lateNightCount := 0
	for _, tx := range outgoing {
		hour := tx.OccurredAt.UTC().Hour()
		if hour >= 23 || hour < 5 {
			lateNightCount++
		}
	}
	frequency := 0.0
	if len(outgoing) > 0 {
		frequency = float64(lateNightCount) / float64(len(outgoing))
	}
	countJSON, err := json.Marshal(lateNightCount)
	if err != nil {
		return EvidenceRecord{}, err
	}
	frequencyJSON, err := json.Marshal(roundTo(frequency, 4))
	if err != nil {
		return EvidenceRecord{}, err
	}
	record := EvidenceRecord{
		ID:   EvidenceID("evidence-late-night-signal-" + userID + "-" + now.Format("20060102150405")),
		Type: EvidenceTypeLateNightSpendingSignal,
		Source: EvidenceSource{
			Kind:       "ledger",
			Adapter:    a.SourceType(),
			Reference:  userID,
			Provenance: "ledger_transactions",
		},
		TimeRange: EvidenceTimeRange{
			ObservedAt: now,
			Start:      start,
			End:        end,
		},
		Confidence: EvidenceConfidence{
			Score:  0.88,
			Reason: "computed from transaction timestamps",
		},
		Claims: []EvidenceClaim{
			{Subject: "behavior", Predicate: "late_night_spending_count", Object: "month", ValueJSON: string(countJSON)},
			{Subject: "behavior", Predicate: "late_night_spending_frequency", Object: "month", ValueJSON: string(frequencyJSON)},
		},
		Normalization: EvidenceNormalizationResult{Status: EvidenceNormalizationNormalized},
		Summary:       fmt.Sprintf("late-night spending frequency is %.2f", frequency),
		CreatedAt:     now,
	}
	return normalizer.Normalize(ctx, record)
}

func (a LedgerObservationAdapter) buildDebtSnapshotEvidence(
	ctx context.Context,
	normalizer EvidenceNormalizer,
	userID string,
	debts []DebtRecord,
	now time.Time,
	start *time.Time,
	end *time.Time,
) (EvidenceRecord, error) {
	totalDebt := int64(0)
	totalAPRWeighted := 0.0
	totalMinimumDue := int64(0)
	accountSummaries := make([]map[string]any, 0, len(debts))
	for _, debt := range debts {
		totalDebt += debt.BalanceCents
		totalAPRWeighted += float64(debt.BalanceCents) * debt.AnnualRate
		totalMinimumDue += debt.MinimumDueCents
		accountSummaries = append(accountSummaries, map[string]any{
			"name":              debt.Name,
			"balance_cents":     debt.BalanceCents,
			"annual_rate":       debt.AnnualRate,
			"minimum_due_cents": debt.MinimumDueCents,
		})
	}
	averageAPR := 0.0
	if totalDebt > 0 {
		averageAPR = totalAPRWeighted / float64(totalDebt)
	}
	debtBurden := 0.0
	minimumPaymentPressure := 0.0
	monthlyIncome := maxInt64(totalMonthlyInflowFromTransactions(a.Transactions, userID, start, end), 1)
	debtBurden = float64(totalMinimumDue) / float64(monthlyIncome)
	minimumPaymentPressure = debtBurden

	totalDebtJSON, err := json.Marshal(totalDebt)
	if err != nil {
		return EvidenceRecord{}, err
	}
	averageAPRJSON, err := json.Marshal(roundTo(averageAPR, 4))
	if err != nil {
		return EvidenceRecord{}, err
	}
	debtBurdenJSON, err := json.Marshal(roundTo(debtBurden, 4))
	if err != nil {
		return EvidenceRecord{}, err
	}
	minimumPaymentJSON, err := json.Marshal(roundTo(minimumPaymentPressure, 4))
	if err != nil {
		return EvidenceRecord{}, err
	}
	accountsJSON, err := json.Marshal(accountSummaries)
	if err != nil {
		return EvidenceRecord{}, err
	}
	record := EvidenceRecord{
		ID:   EvidenceID("evidence-debt-snapshot-" + userID + "-" + now.Format("20060102150405")),
		Type: EvidenceTypeDebtObligationSnapshot,
		Source: EvidenceSource{
			Kind:       "ledger",
			Adapter:    a.SourceType(),
			Reference:  userID,
			Provenance: "debt_snapshot",
		},
		TimeRange: EvidenceTimeRange{ObservedAt: now, Start: start, End: end},
		Confidence: EvidenceConfidence{
			Score:  0.97,
			Reason: "derived from structured debt account records",
		},
		Claims: []EvidenceClaim{
			{Subject: "liability", Predicate: "total_debt_cents", Object: "snapshot", ValueJSON: string(totalDebtJSON)},
			{Subject: "liability", Predicate: "average_apr", Object: "snapshot", ValueJSON: string(averageAPRJSON)},
			{Subject: "liability", Predicate: "debt_burden_ratio", Object: "snapshot", ValueJSON: string(debtBurdenJSON)},
			{Subject: "liability", Predicate: "minimum_payment_pressure", Object: "snapshot", ValueJSON: string(minimumPaymentJSON)},
			{Subject: "liability", Predicate: "accounts", Object: "snapshot", ValueJSON: string(accountsJSON)},
		},
		Normalization: EvidenceNormalizationResult{Status: EvidenceNormalizationNormalized},
		Summary:       fmt.Sprintf("captured %d debt accounts with %.2f weighted APR", len(debts), averageAPR),
		CreatedAt:     now,
	}
	return normalizer.Normalize(ctx, record)
}

func (a LedgerObservationAdapter) buildPortfolioSnapshotEvidence(
	ctx context.Context,
	normalizer EvidenceNormalizer,
	userID string,
	holdings []HoldingRecord,
	transactions []LedgerTransactionRecord,
	now time.Time,
	start *time.Time,
	end *time.Time,
) (EvidenceRecord, error) {
	totalAssets := int64(0)
	assetTotals := make(map[string]int64)
	targets := make(map[string]float64)
	for _, holding := range holdings {
		totalAssets += holding.MarketValueCents
		assetTotals[holding.AssetClass] += holding.MarketValueCents
		targets[holding.AssetClass] = holding.TargetAllocation
	}
	allocations := make(map[string]float64, len(assetTotals))
	drift := make(map[string]float64, len(assetTotals))
	for assetClass, value := range assetTotals {
		ratio := float64(value) / float64(maxInt64(totalAssets, 1))
		allocations[assetClass] = roundTo(ratio, 4)
		drift[assetClass] = roundTo(ratio-targets[assetClass], 4)
	}

	monthlyOutflow := maxInt64(totalMonthlyOutflowFromTransactions(transactions), 1)
	emergencyFundMonths := 0.0
	if cashValue, ok := assetTotals["cash"]; ok {
		emergencyFundMonths = roundTo(float64(cashValue)/float64(monthlyOutflow), 2)
	}

	totalAssetsJSON, err := json.Marshal(totalAssets)
	if err != nil {
		return EvidenceRecord{}, err
	}
	allocationsJSON, err := json.Marshal(allocations)
	if err != nil {
		return EvidenceRecord{}, err
	}
	targetsJSON, err := json.Marshal(targets)
	if err != nil {
		return EvidenceRecord{}, err
	}
	driftJSON, err := json.Marshal(drift)
	if err != nil {
		return EvidenceRecord{}, err
	}
	emergencyFundJSON, err := json.Marshal(emergencyFundMonths)
	if err != nil {
		return EvidenceRecord{}, err
	}
	record := EvidenceRecord{
		ID:   EvidenceID("evidence-portfolio-snapshot-" + userID + "-" + now.Format("20060102150405")),
		Type: EvidenceTypePortfolioAllocationSnap,
		Source: EvidenceSource{
			Kind:       "ledger",
			Adapter:    a.SourceType(),
			Reference:  userID,
			Provenance: "holdings_snapshot",
		},
		TimeRange: EvidenceTimeRange{ObservedAt: now, Start: start, End: end},
		Confidence: EvidenceConfidence{
			Score:  0.96,
			Reason: "derived from structured holding records",
		},
		Claims: []EvidenceClaim{
			{Subject: "portfolio", Predicate: "total_investable_assets_cents", Object: "snapshot", ValueJSON: string(totalAssetsJSON)},
			{Subject: "portfolio", Predicate: "asset_allocations", Object: "snapshot", ValueJSON: string(allocationsJSON)},
			{Subject: "portfolio", Predicate: "target_allocations", Object: "snapshot", ValueJSON: string(targetsJSON)},
			{Subject: "portfolio", Predicate: "allocation_drift", Object: "snapshot", ValueJSON: string(driftJSON)},
			{Subject: "portfolio", Predicate: "emergency_fund_months", Object: "snapshot", ValueJSON: string(emergencyFundJSON)},
		},
		Normalization: EvidenceNormalizationResult{Status: EvidenceNormalizationNormalized},
		Summary:       fmt.Sprintf("captured %d holdings across %d asset classes", len(holdings), len(assetTotals)),
		CreatedAt:     now,
	}
	return normalizer.Normalize(ctx, record)
}

type StructuredDocumentObservationAdapter struct {
	AdapterName string
	Artifacts   []DocumentArtifact
	Extractor   EvidenceExtractor
	Normalizer  EvidenceNormalizer
}

func (a StructuredDocumentObservationAdapter) SourceType() string {
	return "document_structured"
}

func (a StructuredDocumentObservationAdapter) Observe(ctx context.Context, request ObservationRequest) ([]EvidenceRecord, error) {
	userID := strings.TrimSpace(request.Params["user_id"])
	normalizer := a.Normalizer
	if normalizer == nil {
		normalizer = CanonicalEvidenceNormalizer{}
	}
	extractor := a.Extractor
	if extractor == nil {
		extractor = DocumentEvidenceExtractor{}
	}

	records := make([]EvidenceRecord, 0, len(a.Artifacts))
	for _, artifact := range a.Artifacts {
		if userID != "" && artifact.UserID != userID {
			continue
		}
		if artifact.Kind == DocumentKindTaxDocument {
			continue
		}
		record, err := buildDocumentEvidenceRecord(ctx, a.SourceType(), artifact, extractor, normalizer)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

type AgenticDocumentObservationAdapterStub struct {
	AdapterName string
	Artifacts   []DocumentArtifact
	Extractor   EvidenceExtractor
	Normalizer  EvidenceNormalizer
}

func (a AgenticDocumentObservationAdapterStub) SourceType() string {
	return "document_agentic_stub"
}

func (a AgenticDocumentObservationAdapterStub) Observe(ctx context.Context, request ObservationRequest) ([]EvidenceRecord, error) {
	userID := strings.TrimSpace(request.Params["user_id"])
	normalizer := a.Normalizer
	if normalizer == nil {
		normalizer = CanonicalEvidenceNormalizer{}
	}
	extractor := a.Extractor
	if extractor == nil {
		extractor = DocumentEvidenceExtractor{}
	}

	records := make([]EvidenceRecord, 0, len(a.Artifacts))
	for _, artifact := range a.Artifacts {
		if userID != "" && artifact.UserID != userID {
			continue
		}
		if artifact.Kind != DocumentKindTaxDocument {
			continue
		}
		record, err := buildDocumentEvidenceRecord(ctx, a.SourceType(), artifact, extractor, normalizer)
		if err != nil {
			return nil, err
		}
		record.Confidence.Score = 0.72
		record.Confidence.Reason = "heuristic agentic parsing stub over semi-structured tax text"
		record.Normalization.Notes = append(record.Normalization.Notes, "parsed via deterministic stub that preserves agentic adapter boundary")
		if err := record.Validate(); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

type TransactionEvidenceExtractor struct{}

func (TransactionEvidenceExtractor) Extract(_ context.Context, raw RawObservation) ([]EvidenceClaim, error) {
	var transactions []LedgerTransactionRecord
	if err := json.Unmarshal(raw.Payload, &transactions); err != nil {
		return nil, err
	}
	inflow := int64(0)
	outflow := int64(0)
	fixedExpense := int64(0)
	for _, tx := range transactions {
		if strings.EqualFold(tx.Direction, "inflow") {
			inflow += tx.AmountCents
			continue
		}
		outflow += tx.AmountCents
		if isFixedExpenseCategory(tx.Category) {
			fixedExpense += tx.AmountCents
		}
	}
	variableExpense := outflow - fixedExpense
	net := inflow - outflow

	claims := make([]EvidenceClaim, 0, 5)
	for predicate, value := range map[string]int64{
		"monthly_inflow_cents":           inflow,
		"monthly_outflow_cents":          outflow,
		"monthly_net_income_cents":       net,
		"monthly_fixed_expense_cents":    fixedExpense,
		"monthly_variable_expense_cents": variableExpense,
	} {
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		claims = append(claims, EvidenceClaim{
			Subject:   "cashflow",
			Predicate: predicate,
			Object:    "month",
			ValueJSON: string(payload),
		})
	}
	return claims, nil
}

type DocumentEvidenceExtractor struct{}

func (DocumentEvidenceExtractor) Extract(_ context.Context, raw RawObservation) ([]EvidenceClaim, error) {
	kind := strings.TrimPrefix(raw.Source.Kind, "document:")
	switch DocumentKind(kind) {
	case DocumentKindPayslip:
		return extractPayslipClaims(raw.Payload)
	case DocumentKindCreditCardStatement:
		return extractCreditCardClaims(raw.Payload)
	case DocumentKindBrokerStatement:
		return extractBrokerClaims(raw.Payload)
	case DocumentKindTaxDocument:
		return extractTaxClaims(raw.Payload)
	default:
		return nil, fmt.Errorf("unsupported document kind %q", kind)
	}
}

type CanonicalEvidenceNormalizer struct{}

func (CanonicalEvidenceNormalizer) Normalize(_ context.Context, record EvidenceRecord) (EvidenceRecord, error) {
	normalized := record
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC()
	}
	normalized.Summary = strings.TrimSpace(normalized.Summary)
	partial := false
	for i := range normalized.Claims {
		normalized.Claims[i].Subject = normalizeToken(normalized.Claims[i].Subject)
		normalized.Claims[i].Predicate = normalizeToken(normalized.Claims[i].Predicate)
		normalized.Claims[i].Object = normalizeToken(normalized.Claims[i].Object)
		if strings.TrimSpace(normalized.Claims[i].ValueJSON) == "" {
			partial = true
		}
	}
	if normalized.Normalization.Status == "" {
		normalized.Normalization.Status = EvidenceNormalizationNormalized
	}
	if normalized.Normalization.CanonicalUnit == "" {
		normalized.Normalization.CanonicalUnit = inferCanonicalUnit(normalized.Claims)
	}
	if partial && normalized.Normalization.Status == EvidenceNormalizationNormalized {
		normalized.Normalization.Status = EvidenceNormalizationPartial
		normalized.Normalization.Notes = append(normalized.Normalization.Notes, "one or more claims were missing value_json")
	}
	if len(normalized.Claims) == 0 {
		normalized.Normalization.Status = EvidenceNormalizationRejected
		normalized.Normalization.RejectedReason = "no claims extracted"
	}
	if err := normalized.Validate(); err != nil {
		return EvidenceRecord{}, err
	}
	return normalized, nil
}

func buildDocumentEvidenceRecord(
	ctx context.Context,
	adapterName string,
	artifact DocumentArtifact,
	extractor EvidenceExtractor,
	normalizer EvidenceNormalizer,
) (EvidenceRecord, error) {
	claims, err := extractor.Extract(ctx, RawObservation{
		Source: EvidenceSource{
			Kind:       "document:" + string(artifact.Kind),
			Adapter:    adapterName,
			Reference:  artifact.ID,
			Provenance: artifact.Filename,
		},
		Payload: artifact.Content,
	})
	if err != nil {
		return EvidenceRecord{}, err
	}

	record := EvidenceRecord{
		ID:   EvidenceID("evidence-document-" + artifact.ID),
		Type: evidenceTypeForDocumentKind(artifact.Kind),
		Source: EvidenceSource{
			Kind:       "document",
			Adapter:    adapterName,
			Reference:  artifact.ID,
			Provenance: artifact.Filename,
		},
		TimeRange: EvidenceTimeRange{ObservedAt: artifact.ObservedAt},
		Confidence: EvidenceConfidence{
			Score:  0.94,
			Reason: "derived from document artifact fields",
		},
		Artifact: &EvidenceArtifactRef{
			ObjectKey: "artifacts/" + artifact.Filename,
			MediaType: artifact.MediaType,
			Checksum:  fmt.Sprintf("bytes:%d", len(artifact.Content)),
		},
		Claims:        claims,
		Normalization: EvidenceNormalizationResult{Status: EvidenceNormalizationNormalized},
		Summary:       fmt.Sprintf("parsed %s document %s", artifact.Kind, artifact.Filename),
		CreatedAt:     artifact.ObservedAt,
	}
	return normalizer.Normalize(ctx, record)
}

func LoadLedgerTransactionsCSV(data []byte) ([]LedgerTransactionRecord, error) {
	rows, err := readCSVRows(data)
	if err != nil {
		return nil, err
	}
	records := make([]LedgerTransactionRecord, 0, len(rows))
	for _, row := range rows {
		occurredAt, err := time.Parse(time.RFC3339, row["occurred_at"])
		if err != nil {
			return nil, err
		}
		amountCents, err := strconv.ParseInt(row["amount_cents"], 10, 64)
		if err != nil {
			return nil, err
		}
		records = append(records, LedgerTransactionRecord{
			UserID:        row["user_id"],
			TransactionID: row["transaction_id"],
			AccountID:     row["account_id"],
			OccurredAt:    occurredAt,
			Description:   row["description"],
			Merchant:      row["merchant"],
			Category:      row["category"],
			Direction:     row["direction"],
			AmountCents:   amountCents,
		})
	}
	return records, nil
}

func LoadDebtRecordsCSV(data []byte) ([]DebtRecord, error) {
	rows, err := readCSVRows(data)
	if err != nil {
		return nil, err
	}
	records := make([]DebtRecord, 0, len(rows))
	for _, row := range rows {
		snapshotAt, err := time.Parse(time.RFC3339, row["snapshot_at"])
		if err != nil {
			return nil, err
		}
		balanceCents, err := strconv.ParseInt(row["balance_cents"], 10, 64)
		if err != nil {
			return nil, err
		}
		annualRate, err := strconv.ParseFloat(row["annual_rate"], 64)
		if err != nil {
			return nil, err
		}
		minimumDueCents, err := strconv.ParseInt(row["minimum_due_cents"], 10, 64)
		if err != nil {
			return nil, err
		}
		records = append(records, DebtRecord{
			UserID:          row["user_id"],
			AccountID:       row["account_id"],
			Name:            row["name"],
			BalanceCents:    balanceCents,
			AnnualRate:      annualRate,
			MinimumDueCents: minimumDueCents,
			SnapshotAt:      snapshotAt,
		})
	}
	return records, nil
}

func LoadHoldingRecordsCSV(data []byte) ([]HoldingRecord, error) {
	rows, err := readCSVRows(data)
	if err != nil {
		return nil, err
	}
	records := make([]HoldingRecord, 0, len(rows))
	for _, row := range rows {
		snapshotAt, err := time.Parse(time.RFC3339, row["snapshot_at"])
		if err != nil {
			return nil, err
		}
		marketValueCents, err := strconv.ParseInt(row["market_value_cents"], 10, 64)
		if err != nil {
			return nil, err
		}
		targetAllocation, err := strconv.ParseFloat(row["target_allocation"], 64)
		if err != nil {
			return nil, err
		}
		records = append(records, HoldingRecord{
			UserID:           row["user_id"],
			AccountID:        row["account_id"],
			SnapshotAt:       snapshotAt,
			AssetClass:       row["asset_class"],
			Symbol:           row["symbol"],
			MarketValueCents: marketValueCents,
			TargetAllocation: targetAllocation,
		})
	}
	return records, nil
}

func readCSVRows(data []byte) ([]map[string]string, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	rows := make([]map[string]string, 0)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) == 0 {
			continue
		}
		row := make(map[string]string, len(header))
		for i, column := range header {
			if i >= len(record) {
				row[column] = ""
				continue
			}
			row[column] = strings.TrimSpace(record[i])
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func extractPayslipClaims(data []byte) ([]EvidenceClaim, error) {
	rows, err := readCSVRows(data)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("payslip document requires at least one row")
	}
	row := rows[0]
	grossPay, err := strconv.ParseInt(row["gross_pay_cents"], 10, 64)
	if err != nil {
		return nil, err
	}
	netPay, err := strconv.ParseInt(row["net_pay_cents"], 10, 64)
	if err != nil {
		return nil, err
	}
	taxWithheld, err := strconv.ParseInt(row["tax_withheld_cents"], 10, 64)
	if err != nil {
		return nil, err
	}
	childcareBenefit, err := strconv.ParseInt(row["childcare_benefit_cents"], 10, 64)
	if err != nil {
		return nil, err
	}
	effectiveTaxRate := 0.0
	if grossPay > 0 {
		effectiveTaxRate = roundTo(float64(taxWithheld)/float64(grossPay), 4)
	}
	return claimsFromPairs([]claimPair{
		{Subject: "cashflow", Predicate: "monthly_inflow_cents", Object: "payroll", Value: netPay},
		{Subject: "cashflow", Predicate: "monthly_net_income_cents", Object: "payroll", Value: netPay},
		{Subject: "tax", Predicate: "effective_tax_rate", Object: "payroll", Value: effectiveTaxRate},
		{Subject: "tax", Predicate: "tax_withheld_cents", Object: "payroll", Value: taxWithheld},
		{Subject: "tax", Predicate: "childcare_benefit_cents", Object: "payroll", Value: childcareBenefit},
		{Subject: "tax", Predicate: "childcare_tax_signal", Object: "payroll", Value: childcareBenefit > 0},
	})
}

func extractCreditCardClaims(data []byte) ([]EvidenceClaim, error) {
	rows, err := readCSVRows(data)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("credit card statement requires at least one row")
	}
	row := rows[0]
	totalDue, err := strconv.ParseInt(row["total_due_cents"], 10, 64)
	if err != nil {
		return nil, err
	}
	minimumDue, err := strconv.ParseInt(row["minimum_due_cents"], 10, 64)
	if err != nil {
		return nil, err
	}
	lateFee, err := strconv.ParseInt(row["late_fee_cents"], 10, 64)
	if err != nil {
		return nil, err
	}
	return claimsFromPairs([]claimPair{
		{Subject: "liability", Predicate: "credit_card_total_due_cents", Object: "statement", Value: totalDue},
		{Subject: "liability", Predicate: "credit_card_minimum_due_cents", Object: "statement", Value: minimumDue},
		{Subject: "liability", Predicate: "credit_card_late_fee_cents", Object: "statement", Value: lateFee},
	})
}

func extractBrokerClaims(data []byte) ([]EvidenceClaim, error) {
	rows, err := readCSVRows(data)
	if err != nil {
		return nil, err
	}
	totalAssets := int64(0)
	assetTotals := make(map[string]int64)
	targets := make(map[string]float64)
	for _, row := range rows {
		marketValue, err := strconv.ParseInt(row["market_value_cents"], 10, 64)
		if err != nil {
			return nil, err
		}
		targetAllocation, err := strconv.ParseFloat(row["target_allocation"], 64)
		if err != nil {
			return nil, err
		}
		assetClass := row["asset_class"]
		totalAssets += marketValue
		assetTotals[assetClass] += marketValue
		targets[assetClass] = targetAllocation
	}
	allocations := make(map[string]float64, len(assetTotals))
	drift := make(map[string]float64, len(assetTotals))
	for assetClass, value := range assetTotals {
		ratio := float64(value) / float64(maxInt64(totalAssets, 1))
		allocations[assetClass] = roundTo(ratio, 4)
		drift[assetClass] = roundTo(ratio-targets[assetClass], 4)
	}
	return claimsFromPairs([]claimPair{
		{Subject: "portfolio", Predicate: "total_investable_assets_cents", Object: "broker", Value: totalAssets},
		{Subject: "portfolio", Predicate: "asset_allocations", Object: "broker", Value: allocations},
		{Subject: "portfolio", Predicate: "target_allocations", Object: "broker", Value: targets},
		{Subject: "portfolio", Predicate: "allocation_drift", Object: "broker", Value: drift},
	})
}

func extractTaxClaims(data []byte) ([]EvidenceClaim, error) {
	text := strings.ToLower(string(data))
	childcareSignal := strings.Contains(text, "childcare") || strings.Contains(text, "dependent care")
	reason := "no family-related tax signal found"
	if childcareSignal {
		reason = "family-related tax benefit keywords found in document"
	}
	return claimsFromPairs([]claimPair{
		{Subject: "tax", Predicate: "childcare_tax_signal", Object: "tax_document", Value: childcareSignal},
		{Subject: "tax", Predicate: "tax_signal_reason", Object: "tax_document", Value: reason},
	})
}

type claimPair struct {
	Subject   string
	Predicate string
	Object    string
	Value     any
}

func claimsFromPairs(items []claimPair) ([]EvidenceClaim, error) {
	claims := make([]EvidenceClaim, 0, len(items))
	for _, item := range items {
		payload, err := json.Marshal(item.Value)
		if err != nil {
			return nil, err
		}
		claims = append(claims, EvidenceClaim{
			Subject:   item.Subject,
			Predicate: item.Predicate,
			Object:    item.Object,
			ValueJSON: string(payload),
		})
	}
	return claims, nil
}

func evidenceTypeForDocumentKind(kind DocumentKind) EvidenceType {
	switch kind {
	case DocumentKindPayslip:
		return EvidenceTypePayslipStatement
	case DocumentKindCreditCardStatement:
		return EvidenceTypeCreditCardStatement
	case DocumentKindTaxDocument:
		return EvidenceTypeTaxDocument
	case DocumentKindBrokerStatement:
		return EvidenceTypeBrokerStatement
	default:
		return EvidenceTypeDocumentStatement
	}
}

func filterTransactions(records []LedgerTransactionRecord, userID string, start *time.Time, end *time.Time) []LedgerTransactionRecord {
	filtered := make([]LedgerTransactionRecord, 0, len(records))
	for _, record := range records {
		if userID != "" && record.UserID != userID {
			continue
		}
		if start != nil && record.OccurredAt.Before(*start) {
			continue
		}
		if end != nil && record.OccurredAt.After(*end) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func filterDebts(records []DebtRecord, userID string, end *time.Time) []DebtRecord {
	filtered := make([]DebtRecord, 0, len(records))
	for _, record := range records {
		if userID != "" && record.UserID != userID {
			continue
		}
		if end != nil && record.SnapshotAt.After(*end) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func filterHoldings(records []HoldingRecord, userID string, end *time.Time) []HoldingRecord {
	filtered := make([]HoldingRecord, 0, len(records))
	for _, record := range records {
		if userID != "" && record.UserID != userID {
			continue
		}
		if end != nil && record.SnapshotAt.After(*end) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func parseRequestTimeRange(params map[string]string) (*time.Time, *time.Time) {
	if params == nil {
		return nil, nil
	}
	parse := func(value string) *time.Time {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		layouts := []string{time.RFC3339, "2006-01-02"}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, value); err == nil {
				utc := parsed.UTC()
				return &utc
			}
		}
		return nil
	}
	return parse(params["start"]), parse(params["end"])
}

func recurringSubscriptionMerchants(transactions []LedgerTransactionRecord) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, tx := range transactions {
		if strings.EqualFold(tx.Direction, "inflow") {
			continue
		}
		if !isSubscriptionTransaction(tx) {
			continue
		}
		merchant := strings.TrimSpace(tx.Merchant)
		if merchant == "" {
			merchant = strings.TrimSpace(tx.Description)
		}
		if merchant == "" {
			continue
		}
		key := strings.ToLower(merchant)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, merchant)
	}
	return result
}

func outgoingTransactions(transactions []LedgerTransactionRecord) []LedgerTransactionRecord {
	result := make([]LedgerTransactionRecord, 0, len(transactions))
	for _, tx := range transactions {
		if strings.EqualFold(tx.Direction, "outflow") {
			result = append(result, tx)
		}
	}
	return result
}

func totalMonthlyInflowFromTransactions(transactions []LedgerTransactionRecord, userID string, start *time.Time, end *time.Time) int64 {
	filtered := filterTransactions(transactions, userID, start, end)
	inflow := int64(0)
	for _, tx := range filtered {
		if strings.EqualFold(tx.Direction, "inflow") {
			inflow += tx.AmountCents
		}
	}
	return inflow
}

func totalMonthlyOutflowFromTransactions(transactions []LedgerTransactionRecord) int64 {
	outflow := int64(0)
	for _, tx := range transactions {
		if strings.EqualFold(tx.Direction, "outflow") {
			outflow += tx.AmountCents
		}
	}
	return outflow
}

func isFixedExpenseCategory(category string) bool {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "subscription", "rent", "utilities", "insurance", "childcare", "loan_payment":
		return true
	default:
		return false
	}
}

func isSubscriptionTransaction(tx LedgerTransactionRecord) bool {
	if strings.EqualFold(tx.Category, "subscription") {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(tx.Description + " " + tx.Merchant))
	keywords := []string{"subscription", "netflix", "spotify", "icloud", "gym"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func inferCanonicalUnit(claims []EvidenceClaim) string {
	for _, claim := range claims {
		switch {
		case strings.Contains(claim.Predicate, "cents"):
			return "cents"
		case strings.Contains(claim.Predicate, "rate"), strings.Contains(claim.Predicate, "ratio"), strings.Contains(claim.Predicate, "frequency"), strings.Contains(claim.Predicate, "months"), strings.Contains(claim.Predicate, "drift"), strings.Contains(claim.Predicate, "allocations"):
			return "ratio"
		}
	}
	return "canonical"
}

func normalizeToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func roundTo(value float64, precision int) float64 {
	scale := math.Pow10(precision)
	return math.Round(value*scale) / scale
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
