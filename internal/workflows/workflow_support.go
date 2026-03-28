package workflows

import (
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/protocol"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func observationInput(spec taskspec.TaskSpec, userID string) map[string]string {
	input := map[string]string{
		"task_id": spec.ID,
		"user_id": userID,
	}
	if spec.Scope.Start != nil {
		input["start"] = spec.Scope.Start.UTC().Format(time.RFC3339)
	}
	if spec.Scope.End != nil {
		input["end"] = spec.Scope.End.UTC().Format(time.RFC3339)
	}
	return input
}

func dedupeEvidence(records []observation.EvidenceRecord) []observation.EvidenceRecord {
	seen := make(map[observation.EvidenceID]struct{}, len(records))
	result := make([]observation.EvidenceRecord, 0, len(records))
	for _, record := range records {
		if _, ok := seen[record.ID]; ok {
			continue
		}
		seen[record.ID] = struct{}{}
		result = append(result, record)
	}
	return result
}

func collectEvidenceIDs(records []observation.EvidenceRecord) []observation.EvidenceID {
	result := make([]observation.EvidenceID, 0, len(records))
	for _, record := range records {
		result = append(result, record.ID)
	}
	return result
}

func systemStepMeta(workflowID string, sender string, spec taskspec.TaskSpec, current state.FinancialWorldState, correlationID string, causationID string) agents.SystemStepMetadata {
	return agents.SystemStepMetadata{
		WorkflowID:    workflowID,
		Sender:        sender,
		Task:          spec,
		StateRef:      protocolStateRef(current),
		CorrelationID: correlationID,
		CausationID:   causationID,
	}
}

func protocolStateRef(current state.FinancialWorldState) protocol.StateReference {
	return protocol.StateReference{
		UserID:     current.UserID,
		SnapshotID: current.Version.SnapshotID,
		Version:    current.Version.Sequence,
	}
}

func updateCausation(meta agents.SystemStepMetadata, responseMetadata protocol.ProtocolMetadata, current state.FinancialWorldState) agents.SystemStepMetadata {
	meta.CausationID = responseMetadata.MessageID
	meta.StateRef = protocolStateRef(current)
	return meta
}

func handleAgentFailure(workflowRuntime runtimepkg.WorkflowRuntime, execCtx runtimepkg.ExecutionContext, current runtimepkg.WorkflowExecutionState, err error, summary string) error {
	if workflowRuntime == nil {
		return err
	}
	_, _, runtimeErr := runtimepkg.HandleAgentExecutionFailure(workflowRuntime, execCtx, current, err, summary)
	if runtimeErr != nil && runtimeErr != err {
		return runtimeErr
	}
	return err
}

func monthlyReviewReportFromPayload(payload reporting.ReportPayload) (MonthlyReviewReport, error) {
	if payload.MonthlyReview == nil {
		return MonthlyReviewReport{}, fmt.Errorf("monthly review report payload is missing")
	}
	return *payload.MonthlyReview, nil
}

func debtDecisionReportFromPayload(payload reporting.ReportPayload) (DebtDecisionReport, error) {
	if payload.DebtDecision == nil {
		return DebtDecisionReport{}, fmt.Errorf("debt decision report payload is missing")
	}
	return *payload.DebtDecision, nil
}
