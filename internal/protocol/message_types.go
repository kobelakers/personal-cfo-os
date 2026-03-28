package protocol

type MessageKind string

const (
	MessageKindPlanRequest                 MessageKind = "plan_request"
	MessageKindPlanResult                  MessageKind = "plan_result"
	MessageKindMemorySyncRequest           MessageKind = "memory_sync_request"
	MessageKindMemorySyncResult            MessageKind = "memory_sync_result"
	MessageKindReportDraftRequest          MessageKind = "report_draft_request"
	MessageKindReportDraftResult           MessageKind = "report_draft_result"
	MessageKindVerificationRequest         MessageKind = "verification_request"
	MessageKindVerificationResult          MessageKind = "verification_result"
	MessageKindGovernanceEvaluationRequest MessageKind = "governance_evaluation_request"
	MessageKindGovernanceEvaluationResult  MessageKind = "governance_evaluation_result"
	MessageKindReportFinalizeRequest       MessageKind = "report_finalize_request"
	MessageKindReportFinalizeResult        MessageKind = "report_finalize_result"
)

func (k MessageKind) IsRequest() bool {
	switch k {
	case MessageKindPlanRequest,
		MessageKindMemorySyncRequest,
		MessageKindReportDraftRequest,
		MessageKindVerificationRequest,
		MessageKindGovernanceEvaluationRequest,
		MessageKindReportFinalizeRequest:
		return true
	default:
		return false
	}
}

func (k MessageKind) IsResult() bool {
	switch k {
	case MessageKindPlanResult,
		MessageKindMemorySyncResult,
		MessageKindReportDraftResult,
		MessageKindVerificationResult,
		MessageKindGovernanceEvaluationResult,
		MessageKindReportFinalizeResult:
		return true
	default:
		return false
	}
}

func ExpectedResultKind(requestKind MessageKind) MessageKind {
	switch requestKind {
	case MessageKindPlanRequest:
		return MessageKindPlanResult
	case MessageKindMemorySyncRequest:
		return MessageKindMemorySyncResult
	case MessageKindReportDraftRequest:
		return MessageKindReportDraftResult
	case MessageKindVerificationRequest:
		return MessageKindVerificationResult
	case MessageKindGovernanceEvaluationRequest:
		return MessageKindGovernanceEvaluationResult
	case MessageKindReportFinalizeRequest:
		return MessageKindReportFinalizeResult
	default:
		return ""
	}
}
