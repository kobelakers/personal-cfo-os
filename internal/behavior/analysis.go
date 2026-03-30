package behavior

import (
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
)

type Analyzer struct{}

func (Analyzer) Analyze(evidence BehaviorEvidence, selected skills.SkillSelection, memoryRefs []string, now time.Time) (AnalysisOutput, error) {
	if selected.Family == "" || selected.RecipeID == "" {
		return AnalysisOutput{}, fmt.Errorf("behavior analysis requires selected skill")
	}
	bundle := ComputeMetrics(evidence, now)
	evidenceRefs := collectEvidenceRefs(evidence.Evidence)
	recommendations, anomalies, trends := Recommend(bundle, selected, evidenceRefs, memoryRefs)
	summaryParts := []string{
		fmt.Sprintf("行为干预块选择了 %s/%s。", selected.Family, selected.RecipeID),
	}
	if len(anomalies) > 0 {
		codes := make([]string, 0, len(anomalies))
		for _, item := range anomalies {
			codes = append(codes, item.Code)
		}
		summaryParts = append(summaryParts, "当前识别到异常："+strings.Join(codes, "、")+"。")
	}
	return AnalysisOutput{
		Summary:         strings.Join(summaryParts, " "),
		Metrics:         bundle,
		Trends:          trends,
		Anomalies:       anomalies,
		Recommendations: recommendations,
		SelectedSkill:   selected,
		EvidenceRefs:    evidenceRefs,
		MemoryRefs:      append([]string{}, memoryRefs...),
		GeneratedAt:     now,
	}, nil
}

func (o AnalysisOutput) ToBlockResult(blockID string, memoryIDs []string, evidenceIDs []observation.EvidenceID, confidence float64) analysis.BehaviorBlockResult {
	riskFlags := make([]analysis.RiskFlag, 0, len(o.Anomalies))
	for _, anomaly := range o.Anomalies {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:       anomaly.Code,
			Severity:   anomaly.Severity,
			Detail:     anomaly.Detail,
			MetricRefs: append([]string{}, anomaly.MetricRefs...),
			MemoryRefs: append([]string{}, o.MemoryRefs...),
		})
	}
	recommendations := make([]analysis.Recommendation, 0, len(o.Recommendations))
	caveats := make([]string, 0, len(o.Recommendations))
	policyRefs := make([]string, 0)
	for idx, item := range o.Recommendations {
		recType := analysis.RecommendationTypeBehaviorGuardrail
		switch item.Type {
		case BehaviorRecommendationSubscriptionCleanup:
			recType = analysis.RecommendationTypeBehaviorSubscriptionCleanup
		case BehaviorRecommendationSpendNudge:
			recType = analysis.RecommendationTypeBehaviorSpendNudge
		}
		recommendations = append(recommendations, analysis.Recommendation{
			ID:               fmt.Sprintf("%s:behavior:%d", blockID, idx),
			Type:             recType,
			Title:            item.Title,
			Detail:           item.Detail,
			RiskLevel:        item.RiskLevel,
			GroundingRefs:    append([]string{}, item.MetricRefs...),
			MetricRefs:       append([]string{}, item.MetricRefs...),
			EvidenceRefs:     append([]string{}, item.EvidenceRefs...),
			MemoryRefs:       append([]string{}, item.MemoryRefs...),
			Caveats:          append([]string{}, item.Caveats...),
			ApprovalRequired: item.ApprovalRequired,
			ApprovalReason:   item.ApprovalReason,
			PolicyRuleRefs:   append([]string{}, item.PolicyRuleRefs...),
			Metadata: map[string]string{
				"skill_family": string(o.SelectedSkill.Family),
				"skill_version": string(o.SelectedSkill.Version),
				"recipe_id": o.SelectedSkill.RecipeID,
			},
		})
		caveats = append(caveats, item.Caveats...)
		policyRefs = append(policyRefs, item.PolicyRuleRefs...)
	}
	reasonDetails := make([]string, 0, len(o.SelectedSkill.Reasons))
	for _, reason := range o.SelectedSkill.Reasons {
		reasonDetails = append(reasonDetails, reason.Detail)
	}
	return analysis.BehaviorBlockResult{
		BlockID:               blockID,
		Summary:               o.Summary,
		KeyFindings:           trendDetails(o.Trends),
		DeterministicMetrics: analysis.BehaviorMetricBundle{
			DuplicateSubscriptionCount:        o.Metrics.Metrics.DuplicateSubscriptionCount,
			LateNightSpendCount:               o.Metrics.Metrics.LateNightSpendCount,
			LateNightSpendRatio:               o.Metrics.Metrics.LateNightSpendRatio,
			DiscretionaryPressureScore:        o.Metrics.Metrics.DiscretionaryPressureScore,
			RecurringSubscriptionCount:        o.Metrics.Metrics.RecurringSubscriptionCount,
			MonthlyVariableExpenseCents:       o.Metrics.Metrics.MonthlyVariableExpenseCents,
			MonthlyNetIncomeCents:             o.Metrics.Metrics.MonthlyNetIncomeCents,
		},
		MetricRecords:         append([]finance.MetricRecord{}, o.Metrics.Records...),
		EvidenceIDs:           append([]observation.EvidenceID{}, evidenceIDs...),
		MemoryIDsUsed:         append([]string{}, memoryIDs...),
		MetricRefs:            refs(o.Metrics),
		GroundingRefs:         append([]string{}, refs(o.Metrics)...),
		RiskFlags:             riskFlags,
		Recommendations:       recommendations,
		Caveats:               uniqueStrings(caveats),
		ApprovalRequired:      recommendationsRequireApproval(recommendations),
		ApprovalReason:        approvalReason(recommendations),
		PolicyRuleRefs:        uniqueStrings(policyRefs),
		SelectedSkill:         o.SelectedSkill,
		SkillSelectionReasons: reasonDetails,
		Confidence:            confidence,
	}
}

func collectEvidenceRefs(records []observation.EvidenceRecord) []string {
	result := make([]string, 0, len(records))
	for _, item := range records {
		result = append(result, string(item.ID))
	}
	return uniqueStrings(result)
}

func trendDetails(items []BehaviorTrend) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Detail)
	}
	return result
}

func recommendationsRequireApproval(items []analysis.Recommendation) bool {
	for _, item := range items {
		if item.ApprovalRequired {
			return true
		}
	}
	return false
}

func approvalReason(items []analysis.Recommendation) string {
	for _, item := range items {
		if item.ApprovalReason != "" {
			return item.ApprovalReason
		}
	}
	return ""
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
