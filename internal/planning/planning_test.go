package planning

import "testing"

func TestPlanStateMachineRejectsInvalidTransition(t *testing.T) {
	machine := PlanStateMachine{Current: PlanPhasePlan}
	if err := machine.Transition(PlanPhaseVerify); err == nil {
		t.Fatalf("expected invalid transition to fail")
	}
}
