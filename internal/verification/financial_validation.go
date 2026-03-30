package verification

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

var numericClaimPattern = regexp.MustCompile(`(?i)(?:\$|¥)?\d[\d,]*(?:\.\d+)?%?`)

type trustReportData struct {
	Recommendations []analysis.Recommendation
	RiskFlags       []analysis.RiskFlag
	MetricRecords   []finance.MetricRecord
}

type FinancialGroundingValidator struct{}

func (FinancialGroundingValidator) Validate(
	_ context.Context,
	spec taskspec.TaskSpec,
	_ state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	finalContext contextview.BlockVerificationContext,
	output any,
) ([]VerificationResult, error) {
	payload, err := reportPayloadFromOutput(output)
	if err != nil {
		return nil, err
	}
	allowedMetricRefs := setFromMetricRecords(payload.MetricRecords)
	allowedEvidenceRefs := setFromEvidence(evidence)
	allowedMemoryRefs := setFromMemories(memories)
	for _, id := range finalContext.SelectedEvidenceIDs {
		allowedEvidenceRefs[string(id)] = struct{}{}
	}
	for _, id := range finalContext.SelectedMemoryIDs {
		allowedMemoryRefs[id] = struct{}{}
	}

	diagnostics := make([]ValidationDiagnostic, 0)
	recommendations := payload.Recommendations
	for idx, recommendation := range recommendations {
		if len(recommendation.GroundingRefs) == 0 &&
			len(recommendation.MetricRefs) == 0 &&
			len(recommendation.EvidenceRefs) == 0 &&
			len(recommendation.MemoryRefs) == 0 {
			diagnostics = append(diagnostics, diagnosticAt(
				"missing_grounding_refs",
				ValidationCategoryGrounding,
				ValidationSeverityError,
				"recommendation is missing metric/evidence/memory grounding",
				idx,
				nil,
				nil,
				nil,
				nil,
				nil,
			))
		}
		if unknown := unknownRefs(recommendation.MetricRefs, allowedMetricRefs); len(unknown) > 0 {
			diagnostics = append(diagnostics, diagnosticAt(
				"unsupported_metric_ref",
				ValidationCategoryGrounding,
				ValidationSeverityError,
				fmt.Sprintf("recommendation references unsupported metrics: %s", strings.Join(unknown, ", ")),
				idx,
				unknown,
				nil,
				nil,
				nil,
				nil,
			))
		}
		if unknown := unknownRefs(recommendation.EvidenceRefs, allowedEvidenceRefs); len(unknown) > 0 {
			diagnostics = append(diagnostics, diagnosticAt(
				"unsupported_evidence_ref",
				ValidationCategoryGrounding,
				ValidationSeverityError,
				fmt.Sprintf("recommendation references unsupported evidence: %s", strings.Join(unknown, ", ")),
				idx,
				nil,
				unknown,
				nil,
				nil,
				nil,
			))
		}
		if unknown := unknownRefs(recommendation.MemoryRefs, allowedMemoryRefs); len(unknown) > 0 {
			diagnostics = append(diagnostics, diagnosticAt(
				"unsupported_memory_ref",
				ValidationCategoryGrounding,
				ValidationSeverityError,
				fmt.Sprintf("recommendation references unsupported memory: %s", strings.Join(unknown, ", ")),
				idx,
				nil,
				nil,
				unknown,
				nil,
				nil,
			))
		}
		if requiresDisclosure(recommendation) && len(recommendation.Caveats) == 0 {
			diagnostics = append(diagnostics, diagnosticAt(
				"missing_caveat",
				ValidationCategoryGrounding,
				ValidationSeverityCritical,
				"high-risk recommendation is missing required caveat/disclosure",
				idx,
				recommendation.MetricRefs,
				append([]string{}, recommendation.EvidenceRefs...),
				recommendation.MemoryRefs,
				recommendation.GroundingRefs,
				recommendation.PolicyRuleRefs,
			))
		}
	}

	return []VerificationResult{buildValidatorResult(
		spec.ID,
		VerificationStatusFromDiagnostics(diagnostics),
		ValidationCategoryGrounding,
		"financial_grounding_validator",
		"finance recommendation grounding checks passed",
		"repair unsupported refs or add required grounding/caveats before publishing the recommendation",
		diagnostics,
	)}, nil
}

type FinancialNumericConsistencyValidator struct{}

func (FinancialNumericConsistencyValidator) Validate(
	_ context.Context,
	spec taskspec.TaskSpec,
	_ state.FinancialWorldState,
	_ []observation.EvidenceRecord,
	_ []memory.MemoryRecord,
	_ contextview.BlockVerificationContext,
	output any,
) ([]VerificationResult, error) {
	payload, err := reportPayloadFromOutput(output)
	if err != nil {
		return nil, err
	}
	metricIndex := metricRecordIndex(payload.MetricRecords)
	allowedNumericClaims := allowedNumericClaims(metricIndex)
	diagnostics := make([]ValidationDiagnostic, 0)

	checkTexts := func(prefix string, refs []string, text string, idx *int) {
		claims := numericClaimPattern.FindAllString(text, -1)
		for _, claim := range claims {
			normalized := normalizeNumericClaim(claim)
			if normalized == "" {
				continue
			}
			if _, ok := allowedNumericClaims[normalized]; ok {
				continue
			}
			// If the recommendation already points at metrics, allow claim only when one of them matches.
			if numericClaimMatchesRefs(normalized, refs, metricIndex) {
				continue
			}
			message := fmt.Sprintf("%s contains unsupported numeric claim %q", prefix, claim)
			diagnostics = append(diagnostics, ValidationDiagnostic{
				Code:                "unsupported_numeric_claim",
				Category:            ValidationCategoryNumeric,
				Severity:            ValidationSeverityError,
				Message:             message,
				MetricRefs:          append([]string{}, refs...),
				RecommendationIndex: idx,
			})
		}
	}

	recommendations := payload.Recommendations
	for idx, recommendation := range recommendations {
		index := idx
		checkTexts("recommendation title", recommendation.MetricRefs, recommendation.Title, &index)
		checkTexts("recommendation detail", recommendation.MetricRefs, recommendation.Detail, &index)
	}
	for _, risk := range payload.RiskFlags {
		checkTexts("risk flag detail", risk.MetricRefs, risk.Detail, nil)
	}

	return []VerificationResult{buildValidatorResult(
		spec.ID,
		VerificationStatusFromDiagnostics(diagnostics),
		ValidationCategoryNumeric,
		"financial_numeric_consistency_validator",
		"numeric consistency checks passed",
		"remove unsupported numeric claims or align the recommendation text with deterministic metric records",
		diagnostics,
	)}, nil
}

type TrustBusinessRuleValidator struct{}

func (TrustBusinessRuleValidator) Validate(
	_ context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	_ []observation.EvidenceRecord,
	_ []memory.MemoryRecord,
	_ contextview.BlockVerificationContext,
	output any,
) ([]VerificationResult, error) {
	payload, err := reportPayloadFromOutput(output)
	if err != nil {
		return nil, err
	}
	metricIndex := metricRecordIndex(payload.MetricRecords)
	recommendations := payload.Recommendations
	diagnostics := make([]ValidationDiagnostic, 0)

	lowLiquidity := metricBelow(metricIndex, "emergency_fund_coverage_months", 3) || metricBelow(metricIndex, "liquidity_buffer_months", 1)
	highDebtPressure := metricAtLeast(metricIndex, "debt_pressure_score", 0.6) || metricAtLeast(metricIndex, "debt_payoff_pressure", 0.6)

	switch spec.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		for idx, recommendation := range recommendations {
			if isAggressiveRecommendation(recommendation.Type) && lowLiquidity {
				if recommendation.RiskLevel != taskspec.RiskLevelHigh && recommendation.RiskLevel != taskspec.RiskLevelCritical {
					diagnostics = append(diagnostics, diagnosticAt(
						"aggressive_low_liquidity_risk_missing",
						ValidationCategoryBusiness,
						ValidationSeverityCritical,
						"aggressive recommendation under low liquidity must be marked high risk",
						idx,
						recommendation.MetricRefs,
						append([]string{}, recommendation.EvidenceRefs...),
						recommendation.MemoryRefs,
						recommendation.GroundingRefs,
						recommendation.PolicyRuleRefs,
					))
				}
				if len(recommendation.Caveats) == 0 {
					diagnostics = append(diagnostics, diagnosticAt(
						"aggressive_low_liquidity_caveat_missing",
						ValidationCategoryBusiness,
						ValidationSeverityCritical,
						"aggressive recommendation under low liquidity must include caveat/disclosure",
						idx,
						recommendation.MetricRefs,
						append([]string{}, recommendation.EvidenceRefs...),
						recommendation.MemoryRefs,
						recommendation.GroundingRefs,
						recommendation.PolicyRuleRefs,
					))
				}
			}
			if currentState.LiabilityState.DebtBurdenRatio >= 0.2 && recommendation.Type == "" {
				diagnostics = append(diagnostics, diagnosticAt(
					"missing_recommendation_type",
					ValidationCategoryBusiness,
					ValidationSeverityError,
					"monthly review recommendation must declare recommendation type for downstream governance",
					idx,
					nil, nil, nil, nil, nil,
				))
			}
		}
	case taskspec.UserIntentDebtVsInvest:
		for idx, recommendation := range recommendations {
			if recommendation.Type != "invest_more" {
				continue
			}
			if lowLiquidity || highDebtPressure {
				if recommendation.RiskLevel != taskspec.RiskLevelHigh && recommendation.RiskLevel != taskspec.RiskLevelCritical {
					diagnostics = append(diagnostics, diagnosticAt(
						"invest_more_high_risk_required",
						ValidationCategoryBusiness,
						ValidationSeverityCritical,
						"aggressive invest_more recommendation under low buffer/high debt pressure must be high risk",
						idx,
						recommendation.MetricRefs,
						append([]string{}, recommendation.EvidenceRefs...),
						recommendation.MemoryRefs,
						recommendation.GroundingRefs,
						recommendation.PolicyRuleRefs,
					))
				}
				if !recommendation.ApprovalRequired {
					diagnostics = append(diagnostics, diagnosticAt(
						"invest_more_requires_approval",
						ValidationCategoryBusiness,
						ValidationSeverityCritical,
						"aggressive invest_more recommendation under low buffer/high debt pressure must require approval",
						idx,
						recommendation.MetricRefs,
						append([]string{}, recommendation.EvidenceRefs...),
						recommendation.MemoryRefs,
						recommendation.GroundingRefs,
						recommendation.PolicyRuleRefs,
					))
				}
			}
		}
	case taskspec.UserIntentTaxOptimization:
		for idx, recommendation := range recommendations {
			if recommendation.Type == "tax_action" && !containsAnyText(recommendation.Caveats, "deadline", "withholding", "compliance", "截止", "预扣", "合规") {
				diagnostics = append(diagnostics, diagnosticAt(
					"tax_action_caveat_required",
					ValidationCategoryBusiness,
					ValidationSeverityError,
					"tax action recommendations must include deadline/withholding/compliance caveat",
					idx,
					recommendation.MetricRefs,
					append([]string{}, recommendation.EvidenceRefs...),
					recommendation.MemoryRefs,
					recommendation.GroundingRefs,
					recommendation.PolicyRuleRefs,
				))
			}
		}
	case taskspec.UserIntentPortfolioRebalance:
		for idx, recommendation := range recommendations {
			if recommendation.Type == "portfolio_rebalance" && !containsAnyText(recommendation.Caveats, "drift", "liquidity", "tax", "偏离", "流动性", "税") {
				diagnostics = append(diagnostics, diagnosticAt(
					"portfolio_rebalance_caveat_required",
					ValidationCategoryBusiness,
					ValidationSeverityError,
					"portfolio rebalance recommendations must include drift/liquidity/tax caveat",
					idx,
					recommendation.MetricRefs,
					append([]string{}, recommendation.EvidenceRefs...),
					recommendation.MemoryRefs,
					recommendation.GroundingRefs,
					recommendation.PolicyRuleRefs,
				))
			}
		}
	}

	return []VerificationResult{buildValidatorResult(
		spec.ID,
		VerificationStatusFromDiagnostics(diagnostics),
		ValidationCategoryBusiness,
		"financial_business_rule_validator",
		"financial business-rule checks passed",
		"repair recommendation risk/caveat/approval semantics to satisfy finance business rules",
		diagnostics,
	)}, nil
}

func VerificationStatusFromDiagnostics(diagnostics []ValidationDiagnostic) VerificationStatus {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == ValidationSeverityError || diagnostic.Severity == ValidationSeverityCritical {
			return VerificationStatusFail
		}
	}
	return VerificationStatusPass
}

func buildValidatorResult(
	taskID string,
	status VerificationStatus,
	category ValidationCategory,
	validator string,
	passMessage string,
	recommendedAction string,
	diagnostics []ValidationDiagnostic,
) VerificationResult {
	message := passMessage
	failedRules := make([]string, 0, len(diagnostics))
	if status == VerificationStatusFail {
		message = diagnostics[0].Message
	}
	for _, diagnostic := range diagnostics {
		failedRules = append(failedRules, diagnostic.Code)
	}
	return VerificationResult{
		Status:                  status,
		Validator:               validator,
		Message:                 message,
		FailedRules:             failedRules,
		RecommendedReplanAction: recommendedAction,
		Severity:                severityForStatus(status),
		Category:                category,
		Diagnostics:             diagnostics,
		EvidenceCoverage:        fullCoverage(taskID),
		CheckedAt:               time.Now().UTC(),
	}
}

func diagnosticAt(
	code string,
	category ValidationCategory,
	severity ValidationSeverity,
	message string,
	index int,
	metricRefs []string,
	evidenceRefs []string,
	memoryRefs []string,
	groundingRefs []string,
	policyRuleRefs []string,
) ValidationDiagnostic {
	i := index
	return ValidationDiagnostic{
		Code:                code,
		Category:            category,
		Severity:            severity,
		Message:             message,
		MetricRefs:          append([]string{}, metricRefs...),
		EvidenceRefs:        append([]string{}, evidenceRefs...),
		MemoryRefs:          append([]string{}, memoryRefs...),
		GroundingRefs:       append([]string{}, groundingRefs...),
		PolicyRuleRefs:      append([]string{}, policyRuleRefs...),
		RecommendationIndex: &i,
	}
}

func reportPayloadFromOutput(output any) (trustReportData, error) {
	value := reflect.ValueOf(output)
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return trustReportData{}, fmt.Errorf("report payload is nil")
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return trustReportData{}, fmt.Errorf("unsupported report output type %T", output)
	}
	if data, ok := extractTrustReportData(value); ok {
		return data, nil
	}
	for _, name := range []string{"MonthlyReview", "DebtDecision", "TaxOptimization", "PortfolioRebalance", "BehaviorIntervention"} {
		field := value.FieldByName(name)
		if !field.IsValid() || (field.Kind() == reflect.Pointer && field.IsNil()) {
			continue
		}
		for field.Kind() == reflect.Pointer {
			field = field.Elem()
		}
		if data, ok := extractTrustReportData(field); ok {
			return data, nil
		}
	}
	return trustReportData{}, fmt.Errorf("unsupported report output type %T", output)
}

func extractTrustReportData(value reflect.Value) (trustReportData, bool) {
	recommendationsField := value.FieldByName("Recommendations")
	riskFlagsField := value.FieldByName("RiskFlags")
	metricRecordsField := value.FieldByName("MetricRecords")
	if !recommendationsField.IsValid() || !riskFlagsField.IsValid() || !metricRecordsField.IsValid() {
		return trustReportData{}, false
	}
	recommendations, ok := reflectSlice[analysis.Recommendation](recommendationsField)
	if !ok {
		return trustReportData{}, false
	}
	riskFlags, ok := reflectSlice[analysis.RiskFlag](riskFlagsField)
	if !ok {
		return trustReportData{}, false
	}
	metricRecords, ok := reflectSlice[finance.MetricRecord](metricRecordsField)
	if !ok {
		return trustReportData{}, false
	}
	return trustReportData{
		Recommendations: recommendations,
		RiskFlags:       riskFlags,
		MetricRecords:   metricRecords,
	}, true
}

func reflectSlice[T any](value reflect.Value) ([]T, bool) {
	if !value.IsValid() || value.Kind() != reflect.Slice {
		return nil, false
	}
	items := make([]T, 0, value.Len())
	for i := 0; i < value.Len(); i++ {
		item, ok := value.Index(i).Interface().(T)
		if !ok {
			return nil, false
		}
		items = append(items, item)
	}
	return items, true
}

func metricRecordIndex(records []finance.MetricRecord) map[string]finance.MetricRecord {
	index := make(map[string]finance.MetricRecord, len(records))
	for _, record := range records {
		if record.Ref == "" {
			continue
		}
		index[record.Ref] = record
	}
	return index
}

func setFromMetricRecords(records []finance.MetricRecord) map[string]struct{} {
	result := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record.Ref == "" {
			continue
		}
		result[record.Ref] = struct{}{}
	}
	return result
}

func setFromEvidence(records []observation.EvidenceRecord) map[string]struct{} {
	result := make(map[string]struct{}, len(records))
	for _, record := range records {
		result[string(record.ID)] = struct{}{}
	}
	return result
}

func setFromMemories(records []memory.MemoryRecord) map[string]struct{} {
	result := make(map[string]struct{}, len(records))
	for _, record := range records {
		result[record.ID] = struct{}{}
	}
	return result
}

func unknownRefs(items []string, allowed map[string]struct{}) []string {
	result := make([]string, 0)
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := allowed[item]; ok {
			continue
		}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func unknownRefsFromEvidence(items []observation.EvidenceID, allowed map[string]struct{}) []string {
	result := make([]string, 0)
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := allowed[string(item)]; ok {
			continue
		}
		result = append(result, string(item))
	}
	sort.Strings(result)
	return result
}

func evidenceRefsToStrings(items []observation.EvidenceID) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		result = append(result, string(item))
	}
	return result
}

func requiresDisclosure(recommendation analysis.Recommendation) bool {
	return recommendation.RiskLevel == taskspec.RiskLevelHigh || recommendation.RiskLevel == taskspec.RiskLevelCritical || recommendation.ApprovalRequired
}

func allowedNumericClaims(index map[string]finance.MetricRecord) map[string]struct{} {
	result := make(map[string]struct{}, len(index)*4)
	for _, record := range index {
		for _, claim := range metricClaims(record) {
			result[claim] = struct{}{}
		}
	}
	return result
}

func metricClaims(record finance.MetricRecord) []string {
	claims := make([]string, 0, 4)
	switch record.ValueType {
	case finance.MetricValueTypeFloat64:
		value := record.Float64Value
		claims = append(claims, normalizeNumericClaim(strconv.FormatFloat(value, 'f', -1, 64)))
		if record.Unit == "ratio" || record.Unit == "percent" {
			claims = append(claims, normalizeNumericClaim(strconv.FormatFloat(value*100, 'f', 2, 64)+"%"))
		}
	case finance.MetricValueTypeInt64:
		claims = append(claims, normalizeNumericClaim(strconv.FormatInt(record.Int64Value, 10)))
	case finance.MetricValueTypeString:
		claims = append(claims, normalizeNumericClaim(record.StringValue))
	}
	filtered := claims[:0]
	for _, claim := range claims {
		if claim == "" {
			continue
		}
		filtered = append(filtered, claim)
	}
	return slices.Compact(filtered)
}

func numericClaimMatchesRefs(claim string, refs []string, index map[string]finance.MetricRecord) bool {
	for _, ref := range refs {
		record, ok := index[ref]
		if !ok {
			continue
		}
		if slices.Contains(metricClaims(record), claim) {
			return true
		}
	}
	return false
}

func normalizeNumericClaim(claim string) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(claim, ",", ""))
	cleaned = strings.TrimPrefix(cleaned, "$")
	cleaned = strings.TrimPrefix(cleaned, "¥")
	return cleaned
}

func metricBelow(index map[string]finance.MetricRecord, ref string, threshold float64) bool {
	value, ok := metricFloat(index, ref)
	return ok && value < threshold
}

func metricAtLeast(index map[string]finance.MetricRecord, ref string, threshold float64) bool {
	value, ok := metricFloat(index, ref)
	return ok && value >= threshold
}

func metricFloat(index map[string]finance.MetricRecord, ref string) (float64, bool) {
	record, ok := index[ref]
	if !ok {
		return 0, false
	}
	switch record.ValueType {
	case finance.MetricValueTypeFloat64:
		return record.Float64Value, true
	case finance.MetricValueTypeInt64:
		return float64(record.Int64Value), true
	default:
		return 0, false
	}
}

func isAggressiveRecommendation(kind analysis.RecommendationType) bool {
	switch kind {
	case analysis.RecommendationTypeInvestMore, analysis.RecommendationTypePortfolioRebalance, analysis.RecommendationTypeDebtRestructure:
		return true
	default:
		return false
	}
}

func containsAnyText(items []string, candidates ...string) bool {
	joined := strings.ToLower(strings.Join(items, " "))
	for _, candidate := range candidates {
		if strings.Contains(joined, strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}
