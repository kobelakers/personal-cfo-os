package planning

import "github.com/kobelakers/personal-cfo-os/internal/taskspec"

type Planner interface {
	Transition(next PlanPhase) error
	CurrentPhase() PlanPhase
	BindTask(spec taskspec.TaskSpec) error
}
