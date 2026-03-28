package observability

import "time"

type ReplayBundle struct {
	Scenario    string            `json:"scenario"`
	Trace       WorkflowTraceDump `json:"trace"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	GeneratedAt time.Time         `json:"generated_at"`
}

func NewReplayBundle(scenario string, trace WorkflowTraceDump, metadata map[string]string) ReplayBundle {
	return ReplayBundle{
		Scenario:    scenario,
		Trace:       trace,
		Metadata:    metadata,
		GeneratedAt: time.Now().UTC(),
	}
}
