package structured

type Validator[T any] interface {
	Validate(value T) []string
}

type ValidatorFunc[T any] func(value T) []string

func (f ValidatorFunc[T]) Validate(value T) []string {
	if f == nil {
		return nil
	}
	return f(value)
}
