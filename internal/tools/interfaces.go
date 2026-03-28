package tools

import "context"

type QueryTool interface {
	Name() string
	Query(ctx context.Context, input map[string]string) (string, error)
}

type SimulationTool interface {
	Name() string
	Simulate(ctx context.Context, input map[string]string) (string, error)
}

type ParsingTool interface {
	Name() string
	Parse(ctx context.Context, input []byte) (string, error)
}

type ActionTool interface {
	Name() string
	Execute(ctx context.Context, input map[string]string) (string, error)
}
