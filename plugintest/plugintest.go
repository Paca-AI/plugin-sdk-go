// Package plugintest provides helpers for unit-testing Paca backend plugins
// without a live database or WASM runtime.
//
// Quick start:
//
//	func TestMyRoute(t *testing.T) {
//	    ctx := plugintest.NewContext(t)
//
//	    // Seed test data
//	    ctx.DB.SeedRows("items", []string{"id", "title"}, [][]any{
//	        {"1", "First item"},
//	        {"2", "Second item"},
//	    })
//
//	    // Register your plugin's routes
//	    var p myPlugin
//	    if err := p.Init(ctx.PluginContext()); err != nil {
//	        t.Fatal(err)
//	    }
//
//	    // Call a route and inspect the response
//	    res := ctx.Call("GET", "/items", plugintest.Request{})
//	    if res.StatusCode != 200 {
//	        t.Fatalf("expected 200, got %d: %s", res.StatusCode, res.BodyString())
//	    }
//	}
package plugintest

import (
	"encoding/json"
	"testing"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

// Context is a test harness that wraps a plugin.Context with in-memory
// backends for DB, KV, Logger, and Config.
type Context struct {
	// DB is the in-memory SQL-like query backend.
	DB *InMemoryDB
	// KV is the in-memory key-value store.
	KV *InMemoryKV
	// Cache is the in-memory TTL cache store.
	Cache *InMemoryCache
	// Log captures log messages emitted by the plugin.
	Log *CapturingLogger
	// Config is an in-memory config store.
	Config *InMemoryConfig
	// Permissions controls which permissions plugin.Context.Permissions().Check
	// reports as held by the caller. All permissions are denied by default.
	Permissions *FakePermissions

	pluginCtx *plugin.Context
	// routes registered by Plugin.Init (via the plugin.Context)
	dispatcher *testDispatcher
	t          testing.TB
}

// NewContext creates a fresh test Context for a single test case.
// Cleanup is registered automatically via t.Cleanup().
func NewContext(t testing.TB) *Context {
	t.Helper()
	db := newInMemoryDB()
	kv := newInMemoryKV()
	cache := newInMemoryCache()
	log := newCapturingLogger()
	cfg := newInMemoryConfig()
	perm := newFakePermissions()

	pCtx := plugin.NewContextForTest(db, kv, cache, log, cfg, perm)
	d := &testDispatcher{pluginCtx: pCtx}

	tc := &Context{
		DB:          db,
		KV:          kv,
		Cache:       cache,
		Log:         log,
		Config:      cfg,
		Permissions: perm,
		pluginCtx:   pCtx,
		dispatcher:  d,
		t:           t,
	}
	t.Cleanup(func() { _ = tc })
	return tc
}

// PluginContext returns the underlying plugin.Context to pass to Plugin.Init.
func (c *Context) PluginContext() *plugin.Context { return c.pluginCtx }

// Call dispatches a request to the route registered at method+path.
// It returns the plugin.Response the handler wrote.
func (c *Context) Call(method, path string, req Request) *plugin.Response {
	c.t.Helper()
	return c.dispatcher.call(method, path, req)
}

// EvaluateCondition dispatches to the Condition handler registered for
// nodeType (via [plugin.Context.Condition] in the plugin's Init) and
// returns its result. It fails the test via t.Fatalf if no handler is
// registered for nodeType.
func (c *Context) EvaluateCondition(nodeType string, req ConditionRequest) plugin.ConditionResult {
	c.t.Helper()
	pluginReq := &plugin.ConditionRequest{
		NodeType:  nodeType,
		Config:    req.Config,
		Task:      req.Task,
		ProjectID: req.ProjectID,
	}
	result, ok := plugin.DispatchCondition(c.pluginCtx, pluginReq)
	if !ok {
		c.t.Fatalf("plugintest: no condition handler registered for node type %q", nodeType)
	}
	return result
}

// RunAction dispatches to the Action handler registered for nodeType (via
// [plugin.Context.Action] in the plugin's Init) and returns its result. It
// fails the test via t.Fatalf if no handler is registered for nodeType.
func (c *Context) RunAction(nodeType string, req ActionRequest) plugin.ActionResult {
	c.t.Helper()
	pluginReq := &plugin.ActionRequest{
		NodeType:       nodeType,
		Config:         req.Config,
		Task:           req.Task,
		ProjectID:      req.ProjectID,
		IdempotencyKey: req.IdempotencyKey,
	}
	result, ok := plugin.DispatchAction(c.pluginCtx, pluginReq)
	if !ok {
		c.t.Fatalf("plugintest: no action handler registered for node type %q", nodeType)
	}
	return result
}

// ── Request ───────────────────────────────────────────────────────────────────

// Request represents a test HTTP request.
type Request struct {
	// PathParams are named route parameter values, e.g. {"id": "abc"}.
	PathParams map[string]string
	// Query holds URL query parameter values.
	Query map[string]string
	// Headers holds request headers (lower-cased keys).
	Headers map[string]string
	// Body is the raw request body bytes.
	Body []byte
	// Caller is the identity forwarded by the host.
	Caller plugin.CallerIdentity
}

// WithJSONBody sets the request body to the JSON-encoded form of v.
func (r Request) WithJSONBody(v any) Request {
	data, _ := json.Marshal(v)
	r.Body = data
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Headers["content-type"] = "application/json"
	return r
}

// ── Condition / Action ───────────────────────────────────────────────────────

// ConditionRequest represents a test automation Condition evaluation. Config
// and Task are forwarded verbatim to the registered handler; the node type
// is passed separately to [Context.EvaluateCondition].
type ConditionRequest struct {
	// Config is the node's raw JSON config, as it would come from the
	// automation graph.
	Config json.RawMessage
	// Task is the task snapshot the handler receives alongside Config.
	Task plugin.TaskSnapshot
	// ProjectID is the project the automation run belongs to, as the host
	// would supply it (see plugin.ConditionRequest.ProjectID).
	ProjectID string
}

// WithJSONConfig sets Config to the JSON-encoded form of v.
func (r ConditionRequest) WithJSONConfig(v any) ConditionRequest {
	data, _ := json.Marshal(v)
	r.Config = data
	return r
}

// ActionRequest represents a test automation Action execution. Config and
// Task are forwarded verbatim to the registered handler; the node type is
// passed separately to [Context.RunAction].
type ActionRequest struct {
	// Config is the node's raw JSON config, as it would come from the
	// automation graph.
	Config json.RawMessage
	// Task is the task snapshot the handler receives alongside Config.
	Task plugin.TaskSnapshot
	// ProjectID is the project the automation run belongs to, as the host
	// would supply it (see plugin.ActionRequest.ProjectID).
	ProjectID string
	// IdempotencyKey is the stable (run, node) key handed to the handler.
	IdempotencyKey string
}

// WithJSONConfig sets Config to the JSON-encoded form of v.
func (r ActionRequest) WithJSONConfig(v any) ActionRequest {
	data, _ := json.Marshal(v)
	r.Config = data
	return r
}

// ── testDispatcher ────────────────────────────────────────────────────────────

type testDispatcher struct {
	pluginCtx *plugin.Context
}

func (d *testDispatcher) call(method, path string, req Request) *plugin.Response {
	pluginReq := &plugin.Request{
		Method: method,
		Path:   path,
		Body:   req.Body,
		Caller: req.Caller,
	}
	if req.PathParams != nil {
		pluginReq.PathParams = req.PathParams
	} else {
		pluginReq.PathParams = make(map[string]string)
	}
	if req.Query != nil {
		pluginReq.Query = req.Query
	} else {
		pluginReq.Query = make(map[string]string)
	}
	if req.Headers != nil {
		pluginReq.Headers = req.Headers
	} else {
		pluginReq.Headers = make(map[string]string)
	}

	res := plugin.NewResponse()
	plugin.DispatchRoute(d.pluginCtx, method, path, pluginReq, res)
	return res
}
