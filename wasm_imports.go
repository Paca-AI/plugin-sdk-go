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
// Deprecated: has no error channel, so a query that fails to execute is
// indistinguishable from one that succeeded with zero rows. Use
// hostDBQuery2. Kept only so already-compiled plugin binaries that still
// import this signature keep working against newer hosts.
//
//go:wasmimport paca db_query
//go:noescape
func hostDBQuery(sqlPtr, sqlLen, paramsPtr, paramsLen, resultPtrPtr, resultLenPtr int64)

// paca.db_query2(sqlPtr i64, sqlLen i64, paramsPtr i64, paramsLen i64, resultPtrPtr i64, resultLenPtr i64, errPtrPtr i64, errLenPtr i64)
//
//go:wasmimport paca db_query2
//go:noescape
func hostDBQuery2(sqlPtr, sqlLen, paramsPtr, paramsLen, resultPtrPtr, resultLenPtr, errPtrPtr, errLenPtr int64)

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

// paca.cache_get(keyPtr i64, keyLen i64, valuePtrPtr i64, valueLenPtr i64)
//
//go:wasmimport paca cache_get
//go:noescape
func hostCacheGet(keyPtr, keyLen, valuePtrPtr, valueLenPtr int64)

// paca.cache_set(keyPtr i64, keyLen i64, valuePtr i64, valueLen i64, ttlSeconds i32) -> ok i32
//
//go:wasmimport paca cache_set
//go:noescape
func hostCacheSet(keyPtr, keyLen, valuePtr, valueLen int64, ttlSeconds int32) int32

// paca.cache_delete(keyPtr i64, keyLen i64) -> ok i32
//
//go:wasmimport paca cache_delete
//go:noescape
func hostCacheDelete(keyPtr, keyLen int64) int32

// paca.event_emit(topicPtr i64, topicLen i64, payloadPtr i64, payloadLen i64) -> ok i32
//
//go:wasmimport paca event_emit
//go:noescape
func hostEventEmit(topicPtr, topicLen, payloadPtr, payloadLen int64) int32

// paca.fetch(reqPtr i64, reqLen i64, resPtrPtr i64, resLenPtr i64)
//
//go:wasmimport paca fetch
//go:noescape
func hostFetch(reqPtr, reqLen, resPtrPtr, resLenPtr int64)

// paca.activity_record(payloadPtr i64, payloadLen i64) -> ok i32
//
//go:wasmimport paca activity_record
//go:noescape
func hostActivityRecord(payloadPtr, payloadLen int64) int32

// paca.config_get(keyPtr i64, keyLen i64, valuePtrPtr i64, valueLenPtr i64)
//
//go:wasmimport paca config_get
//go:noescape
func hostConfigGet(keyPtr, keyLen, valuePtrPtr, valueLenPtr int64)

// paca.permission_check(permissionPtr i64, permissionLen i64) -> ok i32
//
//go:wasmimport paca permission_check
//go:noescape
func hostPermissionCheck(permissionPtr, permissionLen int64) int32

// paca.settings_get(resPtrPtr i64, resLenPtr i64)
//
//go:wasmimport paca settings_get
//go:noescape
func hostSettingsGet(resPtrPtr, resLenPtr int64)
