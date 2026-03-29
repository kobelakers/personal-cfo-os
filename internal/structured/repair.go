package structured

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

type RepairPolicy struct {
	MaxAttempts int `json:"max_attempts"`
}

func DefaultRepairPolicy() RepairPolicy {
	return RepairPolicy{MaxAttempts: 1}
}

func buildRepairRequest(request model.StructuredGenerationRequest, schemaName string, raw string, diagnostics []string, attempt int) model.StructuredGenerationRequest {
	detailBytes, _ := json.MarshalIndent(diagnostics, "", "  ")
	user := strings.Builder{}
	user.WriteString("The previous structured output was invalid.\n")
	user.WriteString("Schema: " + schemaName + "\n")
	user.WriteString("Diagnostics:\n")
	user.Write(detailBytes)
	user.WriteString("\nReturn only corrected JSON. Do not add explanation.\n")
	user.WriteString("Original invalid output:\n")
	user.WriteString(raw)
	repairRequest := request
	repairRequest.ModelRequest.PromptID = request.ModelRequest.PromptID + ".repair"
	repairRequest.ModelRequest.PromptVersion = request.ModelRequest.PromptVersion
	repairRequest.ModelRequest.GenerationPhase = model.GenerationPhaseRepair
	repairRequest.ModelRequest.AttemptIndex = attempt
	repairRequest.ModelRequest.Messages = []model.Message{
		{
			Role:    model.MessageRoleSystem,
			Content: "You repair malformed or schema-invalid structured JSON outputs. Return only corrected JSON.",
		},
		{
			Role:    model.MessageRoleUser,
			Content: user.String(),
		},
	}
	return repairRequest
}

type TraceRecord struct {
	SchemaName            string                `json:"schema_name"`
	TraceID               string                `json:"trace_id,omitempty"`
	WorkflowID            string                `json:"workflow_id,omitempty"`
	TaskID                string                `json:"task_id,omitempty"`
	Agent                 string                `json:"agent,omitempty"`
	PromptID              string                `json:"prompt_id,omitempty"`
	PromptVersion         string                `json:"prompt_version,omitempty"`
	GenerationPhase       model.GenerationPhase `json:"generation_phase,omitempty"`
	AttemptIndex          int                   `json:"attempt_index,omitempty"`
	ParseAttempts         int                   `json:"parse_attempts"`
	RepairAttempts        int                   `json:"repair_attempts"`
	FailureCategory       FailureCategory       `json:"failure_category,omitempty"`
	ValidationDiagnostics []string              `json:"validation_diagnostics,omitempty"`
	FallbackUsed          bool                  `json:"fallback_used"`
	FallbackReason        string                `json:"fallback_reason,omitempty"`
	RecordedAt            time.Time             `json:"recorded_at"`
}

type TraceRecorder interface {
	RecordStructuredOutput(record TraceRecord)
}

func recordTrace(recorder TraceRecorder, request model.StructuredGenerationRequest, record TraceRecord) {
	if recorder == nil {
		return
	}
	record.TraceID = request.ModelRequest.TraceID
	record.WorkflowID = request.ModelRequest.WorkflowID
	record.TaskID = request.ModelRequest.TaskID
	record.Agent = request.ModelRequest.Agent
	record.PromptID = request.ModelRequest.PromptID
	record.PromptVersion = request.ModelRequest.PromptVersion
	record.GenerationPhase = request.ModelRequest.GenerationPhase
	if record.GenerationPhase == "" {
		record.GenerationPhase = model.GenerationPhaseInitial
	}
	record.AttemptIndex = request.ModelRequest.AttemptIndex
	if record.AttemptIndex == 0 {
		record.AttemptIndex = 1
	}
	record.RecordedAt = time.Now().UTC()
	recorder.RecordStructuredOutput(record)
}

func callGenerator(ctx context.Context, generator model.StructuredGenerator, request model.StructuredGenerationRequest) (model.StructuredGenerationResult, error) {
	if generator == nil {
		return model.StructuredGenerationResult{}, fmt.Errorf("structured generator is required")
	}
	return generator.Generate(ctx, request)
}
