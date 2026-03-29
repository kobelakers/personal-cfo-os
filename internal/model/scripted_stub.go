package model

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ScriptedChatModel is a deterministic test double for exercising
// provider-backed planning/analysis paths, especially malformed -> repair ->
// success and malformed -> repair -> fallback sequences.
type ScriptedChatModel struct {
	mu            sync.Mutex
	Provider      string
	Steps         []ScriptedChatStep
	CallRecorder  CallRecorder
	UsageRecorder UsageRecorder
	index         int
}

type ScriptedChatStep struct {
	ExpectPromptIDPrefix string
	ExpectPhase          GenerationPhase
	Response             ModelResponse
	Err                  error
}

func (m *ScriptedChatModel) Generate(_ context.Context, request ModelRequest) (ModelResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.index >= len(m.Steps) {
		return ModelResponse{}, fmt.Errorf("scripted chat model exhausted at step %d", m.index)
	}
	step := m.Steps[m.index]
	m.index++

	if prefix := strings.TrimSpace(step.ExpectPromptIDPrefix); prefix != "" && !strings.HasPrefix(request.PromptID, prefix) {
		return ModelResponse{}, fmt.Errorf("scripted chat model expected prompt prefix %q, got %q", prefix, request.PromptID)
	}
	phase := request.GenerationPhase
	if phase == "" {
		phase = GenerationPhaseInitial
	}
	attemptIndex := request.AttemptIndex
	if attemptIndex == 0 {
		attemptIndex = 1
	}
	if step.ExpectPhase != "" && phase != step.ExpectPhase {
		return ModelResponse{}, fmt.Errorf("scripted chat model expected phase %q, got %q", step.ExpectPhase, phase)
	}

	start := time.Now().UTC()
	response := step.Response
	if response.Provider == "" {
		response.Provider = m.provider()
	}
	if response.Model == "" {
		response.Model = chooseScriptedModel(request.Profile)
	}
	if response.Profile == "" {
		response.Profile = request.Profile
	}
	if response.Latency == 0 {
		response.Latency = 5 * time.Millisecond
	}
	if response.Usage.TotalTokens == 0 {
		response.Usage = UsageStats{
			PromptTokens:     maxInt(estimateScriptedPromptTokens(request.Messages), 1),
			CompletionTokens: maxInt(len(response.Content)/4, 1),
		}
		response.Usage.TotalTokens = response.Usage.PromptTokens + response.Usage.CompletionTokens
		response.Usage.EstimatedCostUSD = estimateCostUSD(response.Model, request.Profile, response.Usage.PromptTokens, response.Usage.CompletionTokens)
	}

	record := CallRecord{
		Provider:        response.Provider,
		Model:           response.Model,
		Profile:         request.Profile,
		WorkflowID:      request.WorkflowID,
		TaskID:          request.TaskID,
		TraceID:         request.TraceID,
		Agent:           request.Agent,
		PromptID:        request.PromptID,
		PromptVersion:   request.PromptVersion,
		GenerationPhase: phase,
		AttemptIndex:    attemptIndex,
		LatencyMS:       response.Latency.Milliseconds(),
		StartedAt:       start,
		CompletedAt:     time.Now().UTC(),
	}
	if step.Err != nil {
		if providerErr, ok := step.Err.(*ProviderError); ok {
			record.ErrorCategory = providerErr.Category
			record.StatusCode = providerErr.StatusCode
		} else {
			record.ErrorCategory = ProviderErrorTransport
		}
		if m.CallRecorder != nil {
			m.CallRecorder.RecordCall(record)
		}
		return ModelResponse{}, step.Err
	}
	if m.CallRecorder != nil {
		m.CallRecorder.RecordCall(record)
	}
	if m.UsageRecorder != nil {
		m.UsageRecorder.RecordUsage(UsageRecord{
			Provider:         response.Provider,
			Model:            response.Model,
			Profile:          request.Profile,
			WorkflowID:       request.WorkflowID,
			TaskID:           request.TaskID,
			TraceID:          request.TraceID,
			Agent:            request.Agent,
			PromptID:         request.PromptID,
			PromptVersion:    request.PromptVersion,
			GenerationPhase:  phase,
			AttemptIndex:     attemptIndex,
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
			EstimatedCostUSD: response.Usage.EstimatedCostUSD,
			RecordedAt:       time.Now().UTC(),
		})
	}
	return response, nil
}

func (m *ScriptedChatModel) provider() string {
	if m == nil || strings.TrimSpace(m.Provider) == "" {
		return "scripted-test-provider"
	}
	return m.Provider
}

func chooseScriptedModel(profile ModelProfile) string {
	switch profile {
	case ModelProfilePlannerReasoning:
		return "scripted-reasoning-model"
	case ModelProfileCashflowFast:
		return "scripted-fast-model"
	default:
		return "scripted-generic-model"
	}
}

func estimateScriptedPromptTokens(messages []Message) int {
	total := 0
	for _, message := range messages {
		total += len([]rune(message.Content)) / 4
	}
	if total < 1 {
		return 1
	}
	return total
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
