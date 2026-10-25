package httpx

import "encoding/json"

// Nullable is a JSON value that can be absent, null or set, so a request can clear a field without mistaking it for one it left out.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON marks the field as sent and keeps its value, nil for JSON null.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}
