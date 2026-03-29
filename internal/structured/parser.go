package structured

import "encoding/json"

type Parser[T any] interface {
	Parse(raw string) (T, error)
}

type JSONParser[T any] struct{}

func (JSONParser[T]) Parse(raw string) (T, error) {
	var value T
	err := json.Unmarshal([]byte(raw), &value)
	return value, err
}
