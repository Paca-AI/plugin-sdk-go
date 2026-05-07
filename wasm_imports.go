//go:build wasip1

// Package plugin — host function imports.
//
// These declarations bind Go functions to the "paca" WASM host module
// exported by the paca API server runtime (platform/plugin/runtime.go).
// They are only compiled when targeting GOOS=wasip1.
package plugin

// paca.log(level i32, msgPtr i64, msgLen i64)
//
//go:wasmimport paca log
//go:noescape
func hostLog(level int32, ptr, length int64)

// paca.db_query(sqlPtr i64, sqlLen i64, paramsPtr i64, paramsLen i64, resultPtrPtr i64, resultLenPtr i64)
//
//go:wasmimport paca db_query
//go:noescape
func hostDBQuery(sqlPtr, sqlLen, paramsPtr, paramsLen, resultPtrPtr, resultLenPtr int64)

// paca.db_exec(sqlPtr i64, sqlLen i64, paramsPtr i64, paramsLen i64, rowsAffectedPtr i64, errPtrPtr i64, errLenPtr i64)
//
//go:wasmimport paca db_exec
//go:noescape
func hostDBExec(sqlPtr, sqlLen, paramsPtr, paramsLen, rowsAffectedPtr, errPtrPtr, errLenPtr int64)

// paca.storage_get(keyPtr i64, keyLen i64, valuePtrPtr i64, valueLenPtr i64)
//
//go:wasmimport paca storage_get
//go:noescape
func hostStorageGet(keyPtr, keyLen, valuePtrPtr, valueLenPtr int64)

// paca.storage_set(keyPtr i64, keyLen i64, valuePtr i64, valueLen i64) -> ok i32
//
//go:wasmimport paca storage_set
//go:noescape
func hostStorageSet(keyPtr, keyLen, valuePtr, valueLen int64) int32

// paca.storage_delete(keyPtr i64, keyLen i64) -> ok i32
//
//go:wasmimport paca storage_delete
//go:noescape
func hostStorageDelete(keyPtr, keyLen int64) int32

// paca.event_emit(topicPtr i64, topicLen i64, payloadPtr i64, payloadLen i64) -> ok i32
//
//go:wasmimport paca event_emit
//go:noescape
func hostEventEmit(topicPtr, topicLen, payloadPtr, payloadLen int64) int32

// paca.config_get(keyPtr i64, keyLen i64, valuePtrPtr i64, valueLenPtr i64)
//
//go:wasmimport paca config_get
//go:noescape
func hostConfigGet(keyPtr, keyLen, valuePtrPtr, valueLenPtr int64)
