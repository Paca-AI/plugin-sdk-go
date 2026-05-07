package plugin

import "encoding/json"

// Event represents an inbound event delivered to the plugin from the paca
// event bus.
type Event struct {
	// Topic is the event topic, e.g. "task.deleted".
	Topic string
	// Payload is the raw JSON-encoded payload of the event.
	Payload []byte
}

// JSONPayload decodes the event payload as JSON into a value of type T.
// Returns the zero value of T when the payload is empty.
func JSONPayload[T any](evt *Event) (T, error) {
	var dst T
	if len(evt.Payload) == 0 {
		return dst, nil
	}
	err := json.Unmarshal(evt.Payload, &dst)
	return dst, err
}
