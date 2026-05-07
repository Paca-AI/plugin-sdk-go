//go:build wasip1

package plugin

import (
	"encoding/json"
	"unsafe"
)

// ── Memory management ─────────────────────────────────────────────────────────

// mallocBuffer is a pre-allocated buffer for host-managed memory allocations.
// The host writes request data into this buffer and reads response data from it.
var mallocBuffer [10 * 1024 * 1024]byte

// mallocOffset tracks the next free position in mallocBuffer.
var mallocOffset int32

// mallocBase is the absolute linear-memory offset of mallocBuffer[0].
var mallocBase int32

// wasmMalloc allocates space in mallocBuffer and returns the offset.
func wasmMalloc(size int32) int32 {
	if size <= 0 {
		return 0
	}
	if mallocBase == 0 {
		mallocBase = int32(uintptr(unsafe.Pointer(&mallocBuffer[0])))
	}

	bufOffset := mallocOffset
	if bufOffset+size > int32(len(mallocBuffer)) {
		return 0
	}
	mallocOffset += size

	ptr := mallocBase + bufOffset
	return ptr
}

// wasmResetAllocator resets the malloc offset to allow reuse of the buffer.
// This should be called after the host has copied out the response data.
// SAFETY: The host MUST copy out all allocated data before calling this.
func wasmResetAllocator() {
	mallocOffset = 0
}

// ── WASM DB backend ───────────────────────────────────────────────────────────

type wasmDBBackend struct{}

func newWASMDBBackend() DBBackend { return &wasmDBBackend{} }

func (b *wasmDBBackend) Query(sql string, params []any) (*DBQueryResult, error) {
	sqlBytes := []byte(sql)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	outputBuf := make([]byte, 8)
	hostDBQuery(
		int64(ptrOf(sqlBytes)), int64(len(sqlBytes)),
		int64(ptrOf(paramsJSON)), int64(len(paramsJSON)),
		int64(ptrOf(outputBuf)), int64(ptrOf(outputBuf[4:])),
	)
	resPtr := int32(uint32(outputBuf[0]) | uint32(outputBuf[1])<<8 | uint32(outputBuf[2])<<16 | uint32(outputBuf[3])<<24)
	resLen := int32(uint32(outputBuf[4]) | uint32(outputBuf[5])<<8 | uint32(outputBuf[6])<<16 | uint32(outputBuf[7])<<24)
	if resLen == 0 {
		return &DBQueryResult{}, nil
	}
	resBytes := wasmSlice(resPtr, resLen)

	var result DBQueryResult
	if err := json.Unmarshal(resBytes, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (b *wasmDBBackend) Exec(sql string, params []any) (int64, error) {
	sqlBytes := []byte(sql)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return 0, err
	}

	outputBuf := make([]byte, 16)
	hostDBExec(
		int64(ptrOf(sqlBytes)), int64(len(sqlBytes)),
		int64(ptrOf(paramsJSON)), int64(len(paramsJSON)),
		int64(ptrOf(outputBuf)), int64(ptrOf(outputBuf[8:])), int64(ptrOf(outputBuf[12:])),
	)
	rowsAffected := int64(uint64(outputBuf[0]) | uint64(outputBuf[1])<<8 | uint64(outputBuf[2])<<16 | uint64(outputBuf[3])<<24 | uint64(outputBuf[4])<<32 | uint64(outputBuf[5])<<40 | uint64(outputBuf[6])<<48 | uint64(outputBuf[7])<<56)
	errPtr := int32(uint32(outputBuf[8]) | uint32(outputBuf[9])<<8 | uint32(outputBuf[10])<<16 | uint32(outputBuf[11])<<24)
	errLen := int32(uint32(outputBuf[12]) | uint32(outputBuf[13])<<8 | uint32(outputBuf[14])<<16 | uint32(outputBuf[15])<<24)
	if errLen > 0 {
		return 0, &hostError{string(wasmSlice(errPtr, errLen))}
	}
	return rowsAffected, nil
}

// ── WASM KV backend ───────────────────────────────────────────────────────────

type wasmKVBackend struct{}

func newWASMKVBackend() KVBackend { return &wasmKVBackend{} }

func (b *wasmKVBackend) Get(key string) (string, bool) {
	keyBytes := []byte(key)
	outputBuf := make([]byte, 8)
	hostStorageGet(
		int64(ptrOf(keyBytes)), int64(len(keyBytes)),
		int64(ptrOf(outputBuf)), int64(ptrOf(outputBuf[4:])),
	)
	valPtr := int32(uint32(outputBuf[0]) | uint32(outputBuf[1])<<8 | uint32(outputBuf[2])<<16 | uint32(outputBuf[3])<<24)
	valLen := int32(uint32(outputBuf[4]) | uint32(outputBuf[5])<<8 | uint32(outputBuf[6])<<16 | uint32(outputBuf[7])<<24)
	if valLen == 0 {
		return "", false
	}
	return string(wasmSlice(valPtr, valLen)), true
}

func (b *wasmKVBackend) Set(key, value string) {
	keyBytes := []byte(key)
	valBytes := []byte(value)
	hostStorageSet(int64(ptrOf(keyBytes)), int64(len(keyBytes)), int64(ptrOf(valBytes)), int64(len(valBytes)))
}

func (b *wasmKVBackend) Delete(key string) {
	keyBytes := []byte(key)
	hostStorageDelete(int64(ptrOf(keyBytes)), int64(len(keyBytes)))
}

// ── WASM log backend ──────────────────────────────────────────────────────────

type wasmLogBackend struct{}

func newWASMLogBackend() LogBackend { return &wasmLogBackend{} }

func (b *wasmLogBackend) Log(level int, msg string) {
	msgBytes := []byte(msg)
	if len(msgBytes) == 0 {
		return
	}
	hostLog(int32(level), int64(ptrOf(msgBytes)), int64(len(msgBytes)))
}

// ── WASM config backend ───────────────────────────────────────────────────────

type wasmConfigBackend struct{}

func newWASMConfigBackend() ConfigBackend { return &wasmConfigBackend{} }

func (b *wasmConfigBackend) Get(key string) (string, bool) {
	keyBytes := []byte(key)
	outputBuf := make([]byte, 8)
	hostConfigGet(
		int64(ptrOf(keyBytes)), int64(len(keyBytes)),
		int64(ptrOf(outputBuf)), int64(ptrOf(outputBuf[4:])),
	)
	valPtr := int32(uint32(outputBuf[0]) | uint32(outputBuf[1])<<8 | uint32(outputBuf[2])<<16 | uint32(outputBuf[3])<<24)
	valLen := int32(uint32(outputBuf[4]) | uint32(outputBuf[5])<<8 | uint32(outputBuf[6])<<16 | uint32(outputBuf[7])<<24)
	if valLen == 0 {
		return "", false
	}
	return string(wasmSlice(valPtr, valLen)), true
}

// ── EmitEvent ─────────────────────────────────────────────────────────────────

// EmitEvent publishes an event to the paca event bus from WASM.
func EmitEvent(topic string, payload any) {
	topicBytes := []byte(topic)
	payloadBytes, _ := json.Marshal(payload)
	hostEventEmit(
		int64(ptrOf(topicBytes)), int64(len(topicBytes)),
		int64(ptrOf(payloadBytes)), int64(len(payloadBytes)),
	)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

//go:nocheckptr
func wasmSlice(ptr, length int32) []byte {
	if length == 0 || ptr == 0 {
		return nil
	}
	if mallocBase == 0 {
		mallocBase = int32(uintptr(unsafe.Pointer(&mallocBuffer[0])))
	}

	start := ptr - mallocBase
	if start < 0 || start+length > int32(len(mallocBuffer)) {
		return nil
	}
	return mallocBuffer[start : start+length]
}

func ptrOf(b []byte) int32 {
	if len(b) == 0 {
		return 0
	}
	// Return absolute linear-memory pointer expected by host imports.
	return int32(uintptr(unsafe.Pointer(&b[0])))
}

type hostError struct{ msg string }

func (e *hostError) Error() string { return e.msg }
