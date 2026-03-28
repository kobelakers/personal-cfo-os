package planning

import (
	"errors"
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

var allowedTransitions = map[PlanPhase][]PlanPhase{
	PlanPhasePlan:   {PlanPhaseAct, PlanPhaseAbort},
	PlanPhaseAct:    {PlanPhaseVerify, PlanPhaseEscalate, PlanPhaseAbort},
	PlanPhaseVerify: {PlanPhaseReplan, PlanPhaseEscalate, PlanPhaseAbort},
	PlanPhaseReplan: {PlanPhaseAct, PlanPhaseAbort},
}

func (s *PlanStateMachine) Transition(next PlanPhase) error {
	if s == nil {
		return errors.New("plan state machine is nil")
	}
	if s.Current == "" {
		s.Current = PlanPhasePlan
	}
	allowed := allowedTransitions[s.Current]
	for _, candidate := range allowed {
		if candidate == next {
			s.History = append(s.History, s.Current)
			s.Current = next
			return nil
		}
	}
	return fmt.Errorf("invalid planning transition from %q to %q", s.Current, next)
}

func (s *PlanStateMachine) CurrentPhase() PlanPhase {
	if s.Current == "" {
		return PlanPhasePlan
	}
	return s.Current
}

func (s *PlanStateMachine) BindTask(_ taskspec.TaskSpec) error {
	return nil
}
