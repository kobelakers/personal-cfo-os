package planning

type PlanPhase string

const (
	PlanPhasePlan     PlanPhase = "plan"
	PlanPhaseAct      PlanPhase = "act"
	PlanPhaseVerify   PlanPhase = "verify"
	PlanPhaseReplan   PlanPhase = "replan"
	PlanPhaseEscalate PlanPhase = "escalate"
	PlanPhaseAbort    PlanPhase = "abort"
)

type PlanStateMachine struct {
	Current PlanPhase   `json:"current"`
	History []PlanPhase `json:"history,omitempty"`
}
