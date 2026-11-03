//go:build wasip1

// Package plugin — WASM export layer.
//
// This file provides the four exported functions that the paca host runtime
// expects: Init, HandleRequest, HandleEvent, Shutdown, plus paca_malloc/
// paca_free for host-managed memory allocation.
package plugin

import "unsafe"

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
	// handleRequest fully consumes payload via unmarshalJSON before doing
	// anything else, so the request bytes are no longer needed by the time
	// it returns. Reclaim the whole arena for the response instead of
	// stacking the response after the request — otherwise a request near
	// the arena's capacity would leave no room to allocate the response.
	wasmResetAllocator()
	return packWASMResult(result, func(msg string) []byte { return errorResponse(500, msg) })
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
	wasmResetAllocator() // see HandleRequest: payload is fully consumed by this point
	return packWASMResult(result, func(msg string) []byte {
		return marshalConditionResult(ConditionResult{Matched: false, Error: msg})
	})
}

//go:wasmexport RunAction
func RunAction(ptr, length int32) int64 {
	if globalDispatcher == nil {
		return 0
	}
	payload := wasmSlice(ptr, length)
	result := globalDispatcher.runAction(payload)
	wasmResetAllocator() // see HandleRequest: payload is fully consumed by this point
	return packWASMResult(result, func(msg string) []byte {
		return marshalActionResult(ActionResult{Applied: false, Error: msg})
	})
}

// resultBuf keeps the last result alive, outside mallocBuffer, until the next
// export call replaces it. The host reads a result from whatever linear-memory
// address it is given, so a result does not have to fit the fixed arena the host
// allocates request data in; the Go heap does not move objects, so the address
// stays valid until then.
var resultBuf []byte

// packWASMResult returns result as the packed (ptr<<32)|len pair the host
// expects. A result over maxResultBytes is replaced by what tooLarge builds, so
// the host gets an explanation instead of an empty result. NOTE: the host reads
// the result before the plugin's next export call.
func packWASMResult(result []byte, tooLarge func(msg string) []byte) int64 {
	resultBuf = nil // release the previous call's result
	result = boundResult(result, tooLarge)
	if len(result) == 0 {
		return 0
	}
	resultBuf = result
	return (int64(uintptr(unsafe.Pointer(&resultBuf[0]))) << 32) | int64(len(resultBuf))
}

//go:wasmexport HandleEvent
func HandleEvent(topicPtr, topicLen, payloadPtr, payloadLen int32) {
	if globalDispatcher == nil {
		return
	}
	topic := string(wasmSlice(topicPtr, topicLen))
	// Copy: the handler may call host functions (KV, DB, Fetch, ...) that
	// reset the shared arena once they have copied their own results out,
	// which would otherwise overwrite an Event.Payload still aliasing it.
	payload := append([]byte(nil), wasmSlice(payloadPtr, payloadLen)...)
	globalDispatcher.handleEvent(topic, payload)
}

//go:wasmexport Shutdown
func Shutdown() {
	if globalDispatcher != nil && globalDispatcher.plugin != nil {
		globalDispatcher.plugin.Shutdown()
	}
}
