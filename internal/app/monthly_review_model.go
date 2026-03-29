package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

func NewLiveMonthlyReviewChatModel(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
	config := model.OpenAICompatibleConfigFromEnv()
	config.CallRecorder = callRecorder
	config.UsageRecorder = usageRecorder
	return model.NewOpenAICompatibleChatModel(config)
}

func NewMockMonthlyReviewChatModel() model.ChatModel {
	return mockMonthlyReviewChatModel{}
}

func NewMockMonthlyReviewChatModelWithTrace(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
	return mockMonthlyReviewChatModel{
		callRecorder:  callRecorder,
		usageRecorder: usageRecorder,
	}
}

type mockMonthlyReviewChatModel struct {
	callRecorder  model.CallRecorder
	usageRecorder model.UsageRecorder
}

func (m mockMonthlyReviewChatModel) Generate(_ context.Context, request model.ModelRequest) (model.ModelResponse, error) {
	return m.generate(request)
}

func (m mockMonthlyReviewChatModel) generate(request model.ModelRequest) (model.ModelResponse, error) {
	start := time.Now().UTC()
	content := "{}"
	switch {
	case strings.HasPrefix(request.PromptID, "planner.monthly_review.v1"):
		content = `{
  "plan_summary": "本次月度复盘先确认现金流，再检查债务压力。",
  "rationale": "现金流对本月结余和后续债务判断的影响最大，因此先执行 cashflow block，再执行 debt block。",
  "verification_focus_notes": ["cashflow_grounding","debt_grounding"],
  "block_order": ["cashflow-review","debt-review"],
  "step_emphasis": [{"step_id":"compute-metrics","emphasis":"先确认结余和储蓄率，再检查债务最低还款压力。"}]
}`
	case strings.HasPrefix(request.PromptID, "cashflow.monthly_review.v1"):
		evidenceRefs := extractEvidenceRefs(request.Messages)
		if len(evidenceRefs) == 0 {
			evidenceRefs = []string{"evidence-transaction-batch-user-1-20260328140000"}
		}
		primary := evidenceRefs[0]
		content = fmt.Sprintf(`{
  "summary": "本月现金流整体为正，但重复订阅与夜间消费信号仍值得继续跟踪。",
  "key_findings": [
    "deterministic metrics 显示本月净结余为正，当前现金流没有失控。",
    "经常性订阅和夜间消费信号说明仍有可优化的可变支出。"
  ],
  "grounded_recommendations": [
    {
      "title": "继续清理低使用率订阅",
      "detail": "订阅信号已经进入 selected evidence，适合先做低成本的支出优化。",
      "severity": "low",
      "evidence_refs": [%q]
    }
  ],
  "risk_flags": [
    {
      "code": "cashflow_monitoring",
      "severity": "low",
      "detail": "当前结余为正，但消费波动仍需要继续监控。",
      "evidence_ids": [%q]
    }
  ],
  "metric_refs": ["monthly_net_income_cents","savings_rate","duplicate_subscription_count","late_night_spending_frequency"],
  "evidence_refs": [%s],
  "confidence": 0.84,
  "caveats": ["所有金额与比率以 deterministic metrics 为准。"]
}`, primary, primary, joinQuoted(evidenceRefs))
	}
	usage := model.UsageStats{
		PromptTokens:     estimatePromptTokens(request.Messages),
		CompletionTokens: max(64, len(content)/5),
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.EstimatedCostUSD = float64(usage.TotalTokens) / 1_000_000.0
	response := model.ModelResponse{
		Provider:     "mock-openai-compatible",
		Model:        chooseMockModel(request.Profile),
		Profile:      request.Profile,
		ResponseID:   "mock-" + request.PromptID,
		Content:      content,
		FinishReason: "stop",
		Usage:        usage,
		Latency:      20 * time.Millisecond,
		RawResponse:  content,
	}
	if m.callRecorder != nil {
		m.callRecorder.RecordCall(model.CallRecord{
			Provider:      response.Provider,
			Model:         response.Model,
			Profile:       request.Profile,
			WorkflowID:    request.WorkflowID,
			TaskID:        request.TaskID,
			TraceID:       request.TraceID,
			Agent:         request.Agent,
			PromptID:      request.PromptID,
			PromptVersion: request.PromptVersion,
			LatencyMS:     response.Latency.Milliseconds(),
			StartedAt:     start,
			CompletedAt:   time.Now().UTC(),
		})
	}
	if m.usageRecorder != nil {
		m.usageRecorder.RecordUsage(model.UsageRecord{
			Provider:         response.Provider,
			Model:            response.Model,
			Profile:          request.Profile,
			WorkflowID:       request.WorkflowID,
			TaskID:           request.TaskID,
			TraceID:          request.TraceID,
			Agent:            request.Agent,
			PromptID:         request.PromptID,
			PromptVersion:    request.PromptVersion,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			EstimatedCostUSD: usage.EstimatedCostUSD,
			RecordedAt:       time.Now().UTC(),
		})
	}
	return response, nil
}

var evidenceRefPattern = regexp.MustCompile(`([a-z]+(?:-[a-z0-9]+)+)`)

func extractEvidenceRefs(messages []model.Message) []string {
	if refs := extractEvidenceRefsFromSection(messages); len(refs) > 0 {
		return refs
	}
	refs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, message := range messages {
		matches := evidenceRefPattern.FindAllString(message.Content, -1)
		for _, match := range matches {
			if !strings.HasPrefix(match, "evidence-") && !strings.HasPrefix(match, "doc-") {
				continue
			}
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			refs = append(refs, match)
		}
	}
	if len(refs) > 2 {
		refs = refs[:2]
	}
	return refs
}

func extractEvidenceRefsFromSection(messages []model.Message) []string {
	refs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, message := range messages {
		content := message.Content
		start := strings.Index(content, "selected evidence：")
		if start < 0 {
			continue
		}
		section := content[start+len("selected evidence："):]
		if end := strings.Index(section, "\n\nselected memories："); end >= 0 {
			section = section[:end]
		}
		for _, line := range strings.Split(section, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			id := line
			if cut, _, ok := strings.Cut(line, "|"); ok {
				id = strings.TrimSpace(cut)
			}
			if !strings.HasPrefix(id, "evidence-") && !strings.HasPrefix(id, "doc-") {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			refs = append(refs, id)
		}
	}
	if len(refs) > 2 {
		refs = refs[:2]
	}
	return refs
}

func joinQuoted(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		payload, _ := json.Marshal(item)
		quoted = append(quoted, string(payload))
	}
	return strings.Join(quoted, ",")
}

func estimatePromptTokens(messages []model.Message) int {
	total := 0
	for _, message := range messages {
		total += len([]rune(message.Content)) / 4
	}
	if total < 1 {
		return 1
	}
	return total
}

func chooseMockModel(profile model.ModelProfile) string {
	switch profile {
	case model.ModelProfilePlannerReasoning:
		return "mock-reasoning-model"
	case model.ModelProfileCashflowFast:
		return "mock-fast-model"
	default:
		return "mock-generic-model"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
