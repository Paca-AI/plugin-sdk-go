//go:build wasip1

// Package plugin — WASM export layer.
//
// This file provides the four exported functions that the paca host runtime
// expects: Init, HandleRequest, HandleEvent, Shutdown, plus paca_malloc/
// paca_free for host-managed memory allocation.
package plugin

// ── Memory management ─────────────────────────────────────────────────────────

// Exported as paca_malloc/paca_free, not malloc/free: TinyGo's own bundled
// wasi-libc allocator already exports functions literally named "malloc" and
// "free", so exporting under those same names produces a WASM module with
// duplicate export names — invalid per spec, and rejected at load time. The
// host's export lookup (services/api/internal/platform/plugin/runtime.go)
// must use these same names.

// nolint // exported function used by host runtime via WASM interface
//
//go:wasmexport paca_malloc
func malloc(size int32) int32 {
	return wasmMalloc(size)
}

// nolint // exported function used by host runtime via WASM interface
//
//go:wasmexport paca_free
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
	return packWASMResult(result)
}

//go:wasmexport ResetAllocator
func ResetAllocator() {
	wasmResetAllocator()
}

//go:wasmexport EvaluateCondition
func EvaluateCondition(ptr, length int32) int64 {
	if globalDispatcher == nil {
		return 0
	}
	payload := wasmSlice(ptr, length)
	result := globalDispatcher.evaluateCondition(payload)
	return packWASMResult(result)
}

//go:wasmexport RunAction
func RunAction(ptr, length int32) int64 {
	if globalDispatcher == nil {
		return 0
	}
	payload := wasmSlice(ptr, length)
	result := globalDispatcher.runAction(payload)
	return packWASMResult(result)
}

// packWASMResult allocates space in mallocBuffer for result, copies it in,
// and returns the packed (ptr<<32)|len combined offset+length the host
// expects — the same packing HandleRequest, EvaluateCondition, and RunAction
// all use. NOTE: Host MUST copy out the response before calling
// ResetAllocator, which is called after each export call completes.
func packWASMResult(result []byte) int64 {
	if len(result) == 0 {
		return 0
	}
	outPtr := wasmMalloc(int32(len(result)))
	if outPtr == 0 {
		return 0
	}
	out := wasmSlice(outPtr, int32(len(result)))
	if len(out) != len(result) {
		return 0
	}
	copy(out, result)
	return (int64(outPtr) << 32) | int64(len(result))
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
