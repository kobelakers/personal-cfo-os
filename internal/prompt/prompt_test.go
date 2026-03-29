package prompt

import "testing"

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
}
