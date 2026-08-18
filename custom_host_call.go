//go:build wasip1

package plugin

import (
	"encoding/json"
	"fmt"
)

// CustomHostImport is the calling convention every custom `go:wasmimport`
// host function must use to work with [CallHostFunction]:
// (reqPtr, reqLen, resPtrPtr, resLenPtr int64), no return value — the same
// convention Fetch uses. See Fetch's implementation in this package for the
// exact host-side counterpart shape to mirror.
type CustomHostImport func(reqPtr, reqLen, resPtrPtr, resLenPtr int64)

// CallHostFunction is a low-level escape hatch for a plugin that needs a
// host capability beyond what this SDK exposes directly — e.g. a capability
// only relevant to one kind of plugin (like an SMTP-sending mail plugin's
// outbound socket access), which the host implements and gates behind a
// manifest permission the plugin declares for itself, but which doesn't
// belong in this SDK's general-purpose surface.
//
// The plugin author still writes their own `//go:wasmimport` declaration —
// import bindings are static per-symbol and can't be provided generically —
// following the (reqPtr, reqLen, resPtrPtr, resLenPtr int64) convention.
// CallHostFunction then does the JSON marshal/unmarshal and WASM
// linear-memory pointer plumbing this SDK already uses internally for
// Fetch/DB/etc., so plugin authors don't have to hand-roll unsafe pointer
// arithmetic themselves.
//
// request is marshaled to JSON and passed as the call's request payload.
// response, if non-nil, is populated by unmarshaling the call's JSON
// response into it (pass a pointer, as with json.Unmarshal).
func CallHostFunction(hostFn CustomHostImport, request any, response any) error {
	reqJSON, err := json.Marshal(request)
	if err != nil {
		return err
	}
	outputBuf := make([]byte, 8)
	hostFn(
		int64(ptrOf(reqJSON)), int64(len(reqJSON)),
		int64(ptrOf(outputBuf)), int64(ptrOf(outputBuf[4:])),
	)
	resPtr := int32(uint32(outputBuf[0]) | uint32(outputBuf[1])<<8 | uint32(outputBuf[2])<<16 | uint32(outputBuf[3])<<24)
	resLen := int32(uint32(outputBuf[4]) | uint32(outputBuf[5])<<8 | uint32(outputBuf[6])<<16 | uint32(outputBuf[7])<<24)
	if resLen == 0 {
		return fmt.Errorf("plugin: custom host call: empty response from host")
	}
	resBytes := append([]byte(nil), wasmSlice(resPtr, resLen)...)
	wasmResetAllocator()
	if response != nil {
		if err := json.Unmarshal(resBytes, response); err != nil {
			return fmt.Errorf("plugin: custom host call: decode response: %w", err)
		}
	}
	return nil
}
