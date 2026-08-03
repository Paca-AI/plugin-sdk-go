package plugin

// NewContextForTest constructs a [Context] with the provided backend
// implementations.  Pass it to [Plugin.Init] in unit tests.
//
// Production code should use [Run], which installs the WASM host-function
// backends automatically.  This function is safe to call from any build target.
func NewContextForTest(db DBBackend, kv KVBackend, cache CacheBackend, log LogBackend, cfg ConfigBackend, perm PermissionBackend) *Context {
	return newContext(db, kv, cache, log, cfg, perm)
}

// DispatchRoute calls the handler registered at method+path in ctx and writes
// the result to res.  Returns false (and sets a 404 error response) when no
// handler matches.
//
// Intended for use in plugin unit tests; production dispatch goes through the
// WASM HandleRequest export.
func DispatchRoute(ctx *Context, method, path string, req *Request, res *Response) bool {
	handler, matchedParams, ok := ctx.matchRoute(method, path)
	if !ok {
		res.Error(404, "no handler for "+method+" "+path)
		return false
	}
	if req.PathParams == nil {
		req.PathParams = make(map[string]string)
	}
	for key, value := range matchedParams {
		if _, exists := req.PathParams[key]; !exists {
			req.PathParams[key] = value
		}
	}
	handler(req, res)
	return true
}

// DispatchEvent calls the event handler registered for topic in ctx.
// Returns false when no handler is registered for that topic.
//
// Intended for use in plugin unit tests.
func DispatchEvent(ctx *Context, topic string, payload []byte) bool {
	handler, ok := ctx.events[topic]
	if !ok {
		return false
	}
	handler(&Event{Topic: topic, Payload: payload})
	return true
}

// DispatchCondition calls the Condition handler registered for nodeType in
// ctx (via [Context.Condition]) and returns its result. Returns false (with
// a zero-value ConditionResult) when no handler is registered for that
// node type.
//
// Intended for use in plugin unit tests; production dispatch goes through
// the WASM EvaluateCondition export.
func DispatchCondition(ctx *Context, req *ConditionRequest) (ConditionResult, bool) {
	handler, ok := ctx.conditions[req.NodeType]
	if !ok {
		return ConditionResult{}, false
	}
	return handler(req), true
}

// DispatchAction calls the Action handler registered for nodeType in ctx
// (via [Context.Action]) and returns its result. Returns false (with a
// zero-value ActionResult) when no handler is registered for that node
// type.
//
// Intended for use in plugin unit tests; production dispatch goes through
// the WASM RunAction export.
func DispatchAction(ctx *Context, req *ActionRequest) (ActionResult, bool) {
	handler, ok := ctx.actions[req.NodeType]
	if !ok {
		return ActionResult{}, false
	}
	return handler(req), true
}
