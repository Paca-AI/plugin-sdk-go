package plugin

import "fmt"

// maxResultBytes is the largest result (a response, a condition or an action
// result) a plugin hands back to the host. Results are returned from the
// guest's own heap, not from the fixed host-allocation arena, so this is a
// resource limit rather than a buffer size; it stays well below the host's
// linear-memory cap so building the result cannot exhaust it.
const maxResultBytes = 16 << 20

// boundResult returns result when it may be handed to the host. A larger one is
// replaced by what tooLarge builds for its size, so the caller learns why
// instead of receiving an empty result. Kept in a non-wasip1-tagged file so it
// can be unit tested on the host build; wasm_exports.go is the real caller.
func boundResult(result []byte, tooLarge func(msg string) []byte) []byte {
	if len(result) <= maxResultBytes {
		return result
	}
	return tooLarge(fmt.Sprintf("plugin: result of %d bytes exceeds the limit of %d bytes", len(result), maxResultBytes))
}
