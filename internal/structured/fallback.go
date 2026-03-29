package structured

type FallbackPolicy[T any] struct {
	Name    string
	Execute func() (T, error)
}
