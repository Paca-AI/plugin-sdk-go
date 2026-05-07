package plugin

// ── DB ────────────────────────────────────────────────────────────────────────

// DBBackend is the interface implemented by the WASM host runtime and test
// stubs to provide SQL query execution to plugin code.
type DBBackend interface {
	Query(sql string, params []any) (*DBQueryResult, error)
	Exec(sql string, params []any) (int64, error)
}

// DBQueryResult is the structured result returned by [DB.Query].
type DBQueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// DB provides SQL query helpers for plugin code.
// The host runtime enforces that all queries are scoped to the plugin's
// PostgreSQL schema and that DDL/DCL statements are rejected.
type DB struct {
	backend DBBackend
}

// Query executes a SELECT statement and returns the result set.
func (d *DB) Query(sql string, params ...any) (*DBQueryResult, error) {
	return d.backend.Query(sql, params)
}

// Exec executes a non-SELECT DML statement and returns the number of rows affected.
func (d *DB) Exec(sql string, params ...any) (int64, error) {
	return d.backend.Exec(sql, params)
}

// ── KV ────────────────────────────────────────────────────────────────────────

// KVBackend is the interface implemented by the WASM runtime and test stubs
// for the plugin key-value store.
type KVBackend interface {
	Get(key string) (string, bool)
	Set(key, value string)
	Delete(key string)
}

// KV is a simple string key-value store backed by the plugin's plugin_kv table.
type KV struct {
	backend KVBackend
}

// Get retrieves the value for key.  Returns ("", false) when the key does not exist.
func (k *KV) Get(key string) (string, bool) { return k.backend.Get(key) }

// Set stores a string value under key.
func (k *KV) Set(key, value string) { k.backend.Set(key, value) }

// Delete removes key from the store.
func (k *KV) Delete(key string) { k.backend.Delete(key) }

// ── Logger ────────────────────────────────────────────────────────────────────

// LogBackend is the interface used by [Logger] to emit log messages.
type LogBackend interface {
	Log(level int, msg string)
}

// Logger emits structured log messages through the host logging system.
type Logger struct {
	backend LogBackend
}

func (l *Logger) Debug(msg string) { l.backend.Log(0, msg) }
func (l *Logger) Info(msg string)  { l.backend.Log(1, msg) }
func (l *Logger) Warn(msg string)  { l.backend.Log(2, msg) }
func (l *Logger) Error(msg string) { l.backend.Log(3, msg) }

// ── Config ────────────────────────────────────────────────────────────────────

// ConfigBackend is the interface used by [Config] to read configuration values.
type ConfigBackend interface {
	Get(key string) (string, bool)
}

// Config provides read-only access to plugin configuration values supplied by
// the paca administrator.
type Config struct {
	backend ConfigBackend
}

// Get returns the value of a config key.  Returns ("", false) when not set.
func (c *Config) Get(key string) (string, bool) { return c.backend.Get(key) }
