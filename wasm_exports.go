//go:build wasip1

// Package plugin — WASM export layer.
//
// This file provides the four exported functions that the paca host runtime
// expects: Init, HandleRequest, HandleEvent, Shutdown, plus malloc/free for
// host-managed memory allocation.
package plugin

// ── Memory management ─────────────────────────────────────────────────────────

// nolint // exported function used by host runtime via WASM interface
//
//go:wasmexport malloc
func malloc(size int32) int32 {
	return wasmMalloc(size)
}

// nolint // exported function used by host runtime via WASM interface
//
//go:wasmexport free
func free(_ int32) {}

// ── Exported WASM functions ───────────────────────────────────────────────────

//go:wasmexport Init
func Init() int32 {
	if globalDispatcher == nil {
		return 1
	}
	if err := globalDispatcher.init(); err != nil {
		return 1
	}
	return 0
}

//go:wasmexport HandleRequest
func HandleRequest(ptr, length int32) int64 {
	if globalDispatcher == nil {
		return 0
	}
	payload := wasmSlice(ptr, length)
	result := globalDispatcher.handleRequest(payload)
	if len(result) == 0 {
		return 0
	}
	// Allocate space in mallocBuffer for the response
	outPtr := wasmMalloc(int32(len(result)))
	if outPtr == 0 {
		return 0
	}
	// Copy the result into allocated WASM memory.
	out := wasmSlice(outPtr, int32(len(result)))
	if len(out) != len(result) {
		return 0
	}
	copy(out, result)
	// Return offset and length combined into int64
	// NOTE: Host MUST copy out the response before calling ResetAllocator,
	// which is called after each HandleRequest completes.
	return (int64(outPtr) << 32) | int64(len(result))
}

//go:wasmexport ResetAllocator
func ResetAllocator() {
	wasmResetAllocator()
}

//go:wasmexport HandleEvent
func HandleEvent(topicPtr, topicLen, payloadPtr, payloadLen int32) {
	if globalDispatcher == nil {
		return
	}
	topic := string(wasmSlice(topicPtr, topicLen))
	payload := wasmSlice(payloadPtr, payloadLen)
	globalDispatcher.handleEvent(topic, payload)
}

//go:wasmexport Shutdown
func Shutdown() {
	if globalDispatcher != nil && globalDispatcher.plugin != nil {
		globalDispatcher.plugin.Shutdown()
	}
}
