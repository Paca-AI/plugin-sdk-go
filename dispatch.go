package plugin

import "strings"

// dispatcher wires the WASM host exports to the Plugin implementation.
// It is package-private; the host entry points in wasm_exports.go call it
// through the globalDispatcher variable.
type dispatcher struct {
	plugin  Plugin
	ctx     *Context
	initted bool
}

func newDispatcher(p Plugin) *dispatcher {
	return &dispatcher{plugin: p}
}

// init calls Plugin.Init with a fresh Context wired to WASM host backends.
func (d *dispatcher) init() error {
	if d.initted {
		return nil
	}
	d.ctx = newContext(
		newWASMDBBackend(),
		newWASMKVBackend(),
		newWASMLogBackend(),
		newWASMConfigBackend(),
	)
	if err := d.plugin.Init(d.ctx); err != nil {
		return err
	}
	d.initted = true
	return nil
}

// handleRequest deserialises the JSON-encoded host request, dispatches it to
// the matching route handler, and returns a JSON-encoded host response.
//
//nolint:unused // used by wasm_exports.go in WASM builds
func (d *dispatcher) handleRequest(payload []byte) []byte {
	type hostRequest struct {
		Method     string            `json:"method"`
		Path       string            `json:"path"`
		PathParams map[string]string `json:"path_params"`
		Query      map[string]string `json:"query"`
		ProjectID  string            `json:"project_id"`
		CallerID   string            `json:"caller_id"`
		CallerRole string            `json:"caller_role"`
		Headers    map[string]string `json:"headers"`
		Body       []byte            `json:"body"`
	}

	var hr hostRequest
	if err := unmarshalJSON(payload, &hr); err != nil {
		return errorResponse(400, "bad request payload: "+err.Error())
	}

	if err := d.init(); err != nil {
		return errorResponse(500, "plugin init failed: "+err.Error())
	}

	httpMethod := strings.ToUpper(hr.Method)

	req := &Request{
		Method:     httpMethod,
		Path:       hr.Path,
		PathParams: hr.PathParams,
		Query:      hr.Query,
		Headers:    hr.Headers,
		Body:       hr.Body,
		Caller: CallerIdentity{
			CallerID:   hr.CallerID,
			CallerRole: hr.CallerRole,
			ProjectID:  hr.ProjectID,
		},
	}
	if req.PathParams == nil {
		req.PathParams = make(map[string]string)
	}
	if req.Query == nil {
		req.Query = make(map[string]string)
	}

	handler, matchedParams, ok := d.ctx.matchRoute(req.Method, req.Path)
	if !ok {
		return errorResponse(404, "no handler for "+req.Method+" "+req.Path)
	}
	for key, value := range matchedParams {
		if _, exists := req.PathParams[key]; !exists {
			req.PathParams[key] = value
		}
	}

	res := NewResponse()
	handler(req, res)
	return marshalResponse(res)
}

// handleEvent deserialises the topic + JSON payload and calls the matching
// event handler.
//
//nolint:unused // used by wasm_exports.go in WASM builds
func (d *dispatcher) handleEvent(topic string, payload []byte) {
	if err := d.init(); err != nil {
		return
	}

	handler, ok := d.ctx.events[topic]
	if !ok {
		return
	}
	handler(&Event{Topic: topic, Payload: payload})
}
