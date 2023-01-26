package server

import "encoding/json"

type event struct {
	id      string
	payload json.RawMessage
}

func (e event) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{e.id, e.payload})
}
