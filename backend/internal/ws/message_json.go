package ws

import (
	"encoding/json"

	"caiyun/internal/presentation"
)

// MarshalJSON is shared by WebSocket delivery, SSE, and durable-message replay.
// The original payload remains available to server-side diagnostics.
func (message Message) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(message.Data)
	if err != nil {
		return nil, err
	}
	data, err = presentation.MessageJSON(message.Type, data)
	if err != nil {
		return nil, err
	}
	type wireMessage Message
	public := wireMessage(message)
	public.Data = json.RawMessage(data)
	return json.Marshal(public)
}
