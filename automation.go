package plugin

import "encoding/json"

func marshalConditionResult(r ConditionResult) []byte {
	data, _ := json.Marshal(r)
	return data
}

func marshalActionResult(r ActionResult) []byte {
	data, _ := json.Marshal(r)
	return data
}

// TaskSnapshot is the read-only task context the host includes alongside a
// plugin-contributed automation node's config when it dispatches an
// EvaluateCondition or RunAction call. It mirrors the subset of task fields
// the host's pluginNodePayload sends — see
// services/api/internal/worker/automation_consumer.go's pluginNodePayload.
type TaskSnapshot struct {
	ID           string         `json:"id"`
	StatusID     *string        `json:"status_id"`
	AssigneeIDs  []string       `json:"assignee_ids"`
	Importance   int            `json:"importance"`
	Tags         []string       `json:"tags"`
	CustomFields map[string]any `json:"custom_fields"`
}

// ConditionRequest is the payload a plugin's Condition handler receives:
// the automation node's own type (matching the Type declared in the
// plugin's manifest under automation.conditions — useful when a plugin
// registers more than one Condition handler and needs to disambiguate, but
// [Context.Condition] already dispatches by type so most handlers can
// ignore this field) and config (opaque to the host, validated only by the
// plugin) plus a snapshot of the task being evaluated.
type ConditionRequest struct {
	NodeType string          `json:"node_type"`
	Config   json.RawMessage `json:"config"`
	Task     TaskSnapshot    `json:"task"`
}

// ConditionResult is the response a Condition handler returns. Matched
// selects which outgoing edge the automation graph walk follows next: the
// node's "true" handle when true, its "else" handle otherwise.
type ConditionResult struct {
	Matched bool `json:"matched"`
}

// ActionRequest is the payload a plugin's Action handler receives: the
// automation node's own type (see [ConditionRequest.NodeType] for why this
// is included), config, plus a snapshot of the task, plus a stable (run,
// node) idempotency key. The platform cannot enforce idempotency inside a
// plugin's own WASM code, so IdempotencyKey is provided as something stable
// to dedupe against in the plugin's own schema-isolated tables, if it
// chooses to.
type ActionRequest struct {
	NodeType       string          `json:"node_type"`
	Config         json.RawMessage `json:"config"`
	Task           TaskSnapshot    `json:"task"`
	IdempotencyKey string          `json:"idempotency_key"`
}

// ActionResult is the response a plugin's Action handler returns. Applied
// reports whether the action actually changed anything (mirrors the
// idempotency-check pattern built-in actions use — e.g. "already set to
// this value" returns Applied: false with no Error). Error, when non-empty,
// stops the automation graph walk down this branch.
type ActionResult struct {
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

// ConditionHandler evaluates a plugin-contributed Condition automation
// node. Register one via [Context.Condition].
type ConditionHandler func(req *ConditionRequest) ConditionResult

// ActionHandler executes a plugin-contributed Action automation node.
// Register one via [Context.Action].
type ActionHandler func(req *ActionRequest) ActionResult

// Condition registers a handler for a plugin-contributed automation
// Condition node type. nodeType must match the Type declared in the
// plugin's manifest under automation.conditions (reverse-DNS namespaced
// under the plugin's own ID, e.g. "com.paca.github.pr_state").
//
// Only one handler may be registered per nodeType; registering the same
// nodeType twice replaces the previous handler.
func (c *Context) Condition(nodeType string, handler ConditionHandler) {
	if c.conditions == nil {
		c.conditions = make(map[string]ConditionHandler)
	}
	c.conditions[nodeType] = handler
}

// Action registers a handler for a plugin-contributed automation Action
// node type. nodeType must match the Type declared in the plugin's
// manifest under automation.actions (reverse-DNS namespaced under the
// plugin's own ID, e.g. "com.paca.github.merge_pr").
//
// Only one handler may be registered per nodeType; registering the same
// nodeType twice replaces the previous handler.
func (c *Context) Action(nodeType string, handler ActionHandler) {
	if c.actions == nil {
		c.actions = make(map[string]ActionHandler)
	}
	c.actions[nodeType] = handler
}
