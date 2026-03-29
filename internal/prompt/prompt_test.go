package prompt

import (
	"strings"
	"testing"
)

func TestPromptRegistryAndRenderer(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	renderer := PromptRenderer{Registry: registry}
	rendered, err := renderer.Render("planner.monthly_review.v1", struct {
		Goal            string
		CandidateBlocks string
		CandidateSteps  string
		ContextSummary  string
	}{
		Goal:            "monthly review",
		CandidateBlocks: "cashflow-review,debt-review",
		CandidateSteps:  "collect-and-confirm,compute-metrics",
		ContextSummary:  "state and evidence summary",
	}, PromptTraceInput{EstimatedInputTokens: 128})
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}
	if rendered.ID == "" || rendered.Version == "" {
		t.Fatalf("expected rendered prompt metadata, got %+v", rendered)
	}
	if rendered.System == "" || rendered.User == "" {
		t.Fatalf("expected rendered system and user prompt")
	}
	if rendered.Trace.EstimatedInputTokens == 0 {
		t.Fatalf("expected prompt trace token estimate")
	}
	if rendered.Trace.AppliedPolicy == "" {
		t.Fatalf("expected applied render policy in trace")
	}
}

func TestPlannerPromptRenderPolicyAppliesContextBeforeCandidateCatalog(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	rendered, err := (PromptRenderer{Registry: registry}).Render("planner.monthly_review.v1", struct {
		Goal            string
		CandidateBlocks string
		CandidateSteps  string
		ContextSummary  string
	}{
		Goal:            "monthly review",
		CandidateBlocks: "cashflow-review,debt-review",
		CandidateSteps:  "compute-metrics,verify",
		ContextSummary:  "state/evidence context",
	}, PromptTraceInput{EstimatedInputTokens: 128})
	if err != nil {
		t.Fatalf("render planner prompt: %v", err)
	}
	contextPos := strings.Index(rendered.User, "上下文：")
	candidatePos := strings.Index(rendered.User, "候选 blocks：")
	if contextPos < 0 || candidatePos < 0 || contextPos > candidatePos {
		t.Fatalf("expected context section before candidate catalog, got %q", rendered.User)
	}
	if rendered.Trace.AppliedPolicy != ContextInjectionPolicyContextThenCandidateCatalog {
		t.Fatalf("expected planner render policy trace, got %+v", rendered.Trace)
	}
}

func TestCashflowPromptRenderPolicyAppliesContextBeforeGroundedMetrics(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	rendered, err := (PromptRenderer{Registry: registry}).Render("cashflow.monthly_review.v1", struct {
		Goal            string
		BlockID         string
		BlockKind       string
		MetricsSummary  string
		EvidenceSummary string
		MemorySummary   string
		ContextSummary  string
	}{
		Goal:            "cashflow review",
		BlockID:         "cashflow-review",
		BlockKind:       "cashflow_review_block",
		MetricsSummary:  "{\"monthly_net_income_cents\":644900}",
		EvidenceSummary: "evidence-1",
		MemorySummary:   "memory-1",
		ContextSummary:  "execution context",
	}, PromptTraceInput{EstimatedInputTokens: 192})
	if err != nil {
		t.Fatalf("render cashflow prompt: %v", err)
	}
	contextPos := strings.Index(rendered.User, "上下文：")
	metricsPos := strings.Index(rendered.User, "deterministic metrics：")
	if contextPos < 0 || metricsPos < 0 || contextPos > metricsPos {
		t.Fatalf("expected context section before metrics section, got %q", rendered.User)
	}
	if rendered.Trace.AppliedPolicy != ContextInjectionPolicyContextThenGroundedMetrics {
		t.Fatalf("expected cashflow render policy trace, got %+v", rendered.Trace)
	}
	if len(rendered.Trace.RenderDecisions) == 0 {
		t.Fatalf("expected render decisions in trace, got %+v", rendered.Trace)
	}
}
