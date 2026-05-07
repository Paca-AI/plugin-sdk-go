//go:build !wasip1

// Package plugin — stub backends for non-WASM builds.
//
// These stubs satisfy the DBBackend/KVBackend/LogBackend/ConfigBackend
// interfaces and the EmitEvent function when compiling for native platforms
// (e.g. go test, go vet).  They are intentionally no-ops / error-returners
// so that tests must inject real implementations via plugintest.NewContext().
package plugin

import "errors"

var errNotWASM = errors.New("plugin: this function is only available in a WASM module")

// ── Stubs ─────────────────────────────────────────────────────────────────────

func newWASMDBBackend() DBBackend         { return &stubDBBackend{} }
func newWASMKVBackend() KVBackend         { return &stubKVBackend{} }
func newWASMLogBackend() LogBackend       { return &stubLogBackend{} }
func newWASMConfigBackend() ConfigBackend { return &stubConfigBackend{} }

type stubDBBackend struct{}

func (b *stubDBBackend) Query(_ string, _ []any) (*DBQueryResult, error) {
	return nil, errNotWASM
}
func (b *stubDBBackend) Exec(_ string, _ []any) (int64, error) {
	return 0, errNotWASM
}

type stubKVBackend struct{}

func (b *stubKVBackend) Get(_ string) (string, bool) { return "", false }
func (b *stubKVBackend) Set(_, _ string)             {}
func (b *stubKVBackend) Delete(_ string)             {}

type stubLogBackend struct{}

func (b *stubLogBackend) Log(_ int, _ string) {}

type stubConfigBackend struct{}

func (b *stubConfigBackend) Get(_ string) (string, bool) { return "", false }

// EmitEvent is a no-op outside WASM.
func EmitEvent(_ string, _ any) {}

// ptrOf and hostError are used by wasm_backends.go (wasip1 only); provide
// stubs here so the non-WASM build does not need them.
//nolint:unused // used in wasm_backends.go in WASM builds
type hostError struct{ msg string }

//nolint:unused // used in wasm_backends.go in WASM builds
func (e *hostError) Error() string { return e.msg }
