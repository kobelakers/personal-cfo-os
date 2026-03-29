package verification

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
)

var numericTokenPattern = regexp.MustCompile(`-?\d+(?:,\d{3})*(?:\.\d+)?`)

type cashflowNarrativeField struct {
	Name string
	Text string
}

type cashflowMetricRule struct {
	Name          string
	Keywords      []string
	ExpectedForms map[string]struct{}
}

func cashflowNumericGroundingDiagnostics(candidate analysis.CashflowStructuredCandidate, metrics analysis.CashflowDeterministicMetrics) []string {
	allowedTokens := allowedCashflowNumericTokens(metrics)
	rules := cashflowMetricRules(metrics)
	diagnostics := make([]string, 0)

	for _, field := range cashflowNarrativeFields(candidate) {
		if strings.TrimSpace(field.Text) == "" {
			continue
		}
		for _, rawToken := range numericTokenPattern.FindAllString(field.Text, -1) {
			token := normalizeNumericToken(rawToken)
			if token == "" {
				continue
			}
			if _, ok := allowedTokens[token]; !ok {
				diagnostics = append(diagnostics, fmt.Sprintf("unsupported numeric claim %s in %s", rawToken, field.Name))
			}
		}
		for _, rule := range rules {
			for _, claim := range extractKeywordNumericClaims(field.Text, rule.Keywords) {
				if _, ok := rule.ExpectedForms[claim]; ok {
					continue
				}
				diagnostics = append(diagnostics, fmt.Sprintf("%s claim %s in %s is inconsistent with deterministic metrics", rule.Name, claim, field.Name))
			}
		}
	}

	return dedupeDiagnostics(diagnostics)
}

func cashflowNarrativeFields(candidate analysis.CashflowStructuredCandidate) []cashflowNarrativeField {
	fields := []cashflowNarrativeField{
		{Name: "summary", Text: candidate.Summary},
	}
	for index, finding := range candidate.KeyFindings {
		fields = append(fields, cashflowNarrativeField{
			Name: fmt.Sprintf("key_findings[%d]", index),
			Text: finding,
		})
	}
	for index, item := range candidate.GroundedRecommendations {
		fields = append(fields,
			cashflowNarrativeField{Name: fmt.Sprintf("grounded_recommendations[%d].title", index), Text: item.Title},
			cashflowNarrativeField{Name: fmt.Sprintf("grounded_recommendations[%d].detail", index), Text: item.Detail},
		)
	}
	for index, flag := range candidate.RiskFlags {
		fields = append(fields, cashflowNarrativeField{
			Name: fmt.Sprintf("risk_flags[%d].detail", index),
			Text: flag.Detail,
		})
	}
	return fields
}

func allowedCashflowNumericTokens(metrics analysis.CashflowDeterministicMetrics) map[string]struct{} {
	allowed := make(map[string]struct{})
	addAllowedInt(allowed, metrics.MonthlyInflowCents)
	addAllowedInt(allowed, metrics.MonthlyOutflowCents)
	addAllowedInt(allowed, metrics.MonthlyNetIncomeCents)
	addAllowedFloat(allowed, metrics.SavingsRate)
	addAllowedFloat(allowed, metrics.SavingsRate*100)
	addAllowedInt(allowed, int64(metrics.DuplicateSubscriptionCount))
	addAllowedFloat(allowed, metrics.LateNightSpendingFrequency)
	addAllowedFloat(allowed, metrics.LateNightSpendingFrequency*100)
	return allowed
}

func cashflowMetricRules(metrics analysis.CashflowDeterministicMetrics) []cashflowMetricRule {
	return []cashflowMetricRule{
		{
			Name:          "monthly_inflow",
			Keywords:      []string{"流入", "monthly inflow"},
			ExpectedForms: singleIntForm(metrics.MonthlyInflowCents),
		},
		{
			Name:          "monthly_outflow",
			Keywords:      []string{"流出", "支出", "monthly outflow"},
			ExpectedForms: singleIntForm(metrics.MonthlyOutflowCents),
		},
		{
			Name:          "monthly_net",
			Keywords:      []string{"净结余", "净收入", "月度结余", "monthly net", "monthly net income"},
			ExpectedForms: singleIntForm(metrics.MonthlyNetIncomeCents),
		},
		{
			Name:          "savings_rate",
			Keywords:      []string{"储蓄率", "savings rate"},
			ExpectedForms: floatAndPercentForms(metrics.SavingsRate),
		},
		{
			Name:          "duplicate_subscription_count",
			Keywords:      []string{"重复订阅", "duplicate subscription"},
			ExpectedForms: singleIntForm(int64(metrics.DuplicateSubscriptionCount)),
		},
		{
			Name:          "late_night_spending_frequency",
			Keywords:      []string{"深夜消费频率", "夜间消费频率", "late-night spending frequency"},
			ExpectedForms: floatAndPercentForms(metrics.LateNightSpendingFrequency),
		},
	}
}

func extractKeywordNumericClaims(text string, keywords []string) []string {
	lowered := strings.ToLower(text)
	claims := make([]string, 0)
	seen := make(map[string]struct{})
	for _, keyword := range keywords {
		needle := strings.ToLower(keyword)
		remaining := lowered
		offset := 0
		for {
			index := strings.Index(remaining, needle)
			if index < 0 {
				break
			}
			start := offset + index + len(needle)
			windowEnd := start + 24
			if windowEnd > len(lowered) {
				windowEnd = len(lowered)
			}
			window := lowered[start:windowEnd]
			token := normalizeNumericToken(numericTokenPattern.FindString(window))
			if token != "" {
				if _, ok := seen[token]; !ok {
					seen[token] = struct{}{}
					claims = append(claims, token)
				}
			}
			offset = start
			remaining = lowered[offset:]
		}
	}
	return claims
}

func singleIntForm(value int64) map[string]struct{} {
	forms := make(map[string]struct{})
	addAllowedInt(forms, value)
	return forms
}

func floatAndPercentForms(value float64) map[string]struct{} {
	forms := make(map[string]struct{})
	addAllowedFloat(forms, value)
	addAllowedFloat(forms, value*100)
	return forms
}

func addAllowedInt(set map[string]struct{}, value int64) {
	set[strconv.FormatInt(value, 10)] = struct{}{}
}

func addAllowedFloat(set map[string]struct{}, value float64) {
	token := normalizeNumericToken(strconv.FormatFloat(value, 'f', 4, 64))
	if token == "" {
		return
	}
	set[token] = struct{}{}
}

func normalizeNumericToken(token string) string {
	token = strings.TrimSpace(strings.ReplaceAll(token, ",", ""))
	if token == "" {
		return ""
	}
	if strings.Contains(token, ".") {
		token = strings.TrimRight(token, "0")
		token = strings.TrimRight(token, ".")
	}
	if token == "-0" {
		return "0"
	}
	return token
}

func dedupeDiagnostics(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
