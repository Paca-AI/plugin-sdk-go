package plugin

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeAutomationPlugin is a minimal Plugin used to exercise the
// dispatcher's evaluateCondition/runAction paths without a WASM host.
type fakeAutomationPlugin struct {
	initErr    error
	initCalled int
	condition  ConditionHandler
	action     ActionHandler
}

func (p *fakeAutomationPlugin) Init(ctx *Context) error {
	p.initCalled++
	if p.initErr != nil {
		return p.initErr
	}
	if p.condition != nil {
		ctx.Condition("test.cond", p.condition)
	}
	if p.action != nil {
		ctx.Action("test.action", p.action)
	}
	return nil
}

func (p *fakeAutomationPlugin) Shutdown() {}

func decodeConditionResult(t *testing.T, data []byte) ConditionResult {
	t.Helper()
	var got ConditionResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal ConditionResult: %v", err)
	}
	return got
}

func decodeActionResult(t *testing.T, data []byte) ActionResult {
	t.Helper()
	var got ActionResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal ActionResult: %v", err)
	}
	return got
}

func TestDispatcherEvaluateCondition(t *testing.T) {
	t.Run("dispatches to the registered handler", func(t *testing.T) {
		p := &fakeAutomationPlugin{
			condition: func(req *ConditionRequest) ConditionResult {
				return ConditionResult{Matched: req.Task.Importance > 5}
			},
		}
		d := newDispatcher(p)
		payload, _ := json.Marshal(ConditionRequest{NodeType: "test.cond", Task: TaskSnapshot{Importance: 10}})

		got := decodeConditionResult(t, d.evaluateCondition(payload))
		if !got.Matched || got.Error != "" {
			t.Fatalf("got %+v, want Matched=true Error=\"\"", got)
		}
	})

	t.Run("bad payload fails fast without initialising the plugin", func(t *testing.T) {
		p := &fakeAutomationPlugin{}
		d := newDispatcher(p)

		got := decodeConditionResult(t, d.evaluateCondition([]byte("not json")))
		if got.Matched {
			t.Fatalf("got Matched=true for a bad payload, want false")
		}
		if !strings.Contains(got.Error, "bad request payload") {
			t.Fatalf("got Error %q, want it to mention the bad payload", got.Error)
		}
		if p.initCalled != 0 {
			t.Fatalf("plugin Init called %d times for a payload that never parsed, want 0", p.initCalled)
		}
	})

	t.Run("plugin init failure is reported distinctly from a false match", func(t *testing.T) {
		p := &fakeAutomationPlugin{initErr: errors.New("boom")}
		d := newDispatcher(p)
		payload, _ := json.Marshal(ConditionRequest{NodeType: "test.cond"})

		got := decodeConditionResult(t, d.evaluateCondition(payload))
		if got.Matched || !strings.Contains(got.Error, "plugin init failed") {
			t.Fatalf("got %+v, want Matched=false and an init-failure Error", got)
		}
	})

	t.Run("unregistered node type is reported distinctly from a real false match", func(t *testing.T) {
		p := &fakeAutomationPlugin{
			condition: func(req *ConditionRequest) ConditionResult { return ConditionResult{Matched: false} },
		}
		d := newDispatcher(p)
		payload, _ := json.Marshal(ConditionRequest{NodeType: "unknown.type"})

		got := decodeConditionResult(t, d.evaluateCondition(payload))
		if got.Matched || !strings.Contains(got.Error, "no condition handler registered") {
			t.Fatalf("got %+v, want an unregistered-handler Error", got)
		}
	})
}

func TestDispatcherRunAction(t *testing.T) {
	t.Run("dispatches to the registered handler", func(t *testing.T) {
		p := &fakeAutomationPlugin{
			action: func(req *ActionRequest) ActionResult {
				return ActionResult{Applied: req.IdempotencyKey != ""}
			},
		}
		d := newDispatcher(p)
		payload, _ := json.Marshal(ActionRequest{NodeType: "test.action", IdempotencyKey: "run-1/node-2"})

		got := decodeActionResult(t, d.runAction(payload))
		if !got.Applied || got.Error != "" {
			t.Fatalf("got %+v, want Applied=true Error=\"\"", got)
		}
	})

	t.Run("bad payload fails fast without initialising the plugin", func(t *testing.T) {
		p := &fakeAutomationPlugin{}
		d := newDispatcher(p)

		got := decodeActionResult(t, d.runAction([]byte("not json")))
		if got.Applied || !strings.Contains(got.Error, "bad request payload") {
			t.Fatalf("got %+v, want a bad-payload Error", got)
		}
		if p.initCalled != 0 {
			t.Fatalf("plugin Init called %d times for a payload that never parsed, want 0", p.initCalled)
		}
	})

	t.Run("plugin init failure is reported", func(t *testing.T) {
		p := &fakeAutomationPlugin{initErr: errors.New("boom")}
		d := newDispatcher(p)
		payload, _ := json.Marshal(ActionRequest{NodeType: "test.action"})

		got := decodeActionResult(t, d.runAction(payload))
		if got.Applied || !strings.Contains(got.Error, "plugin init failed") {
			t.Fatalf("got %+v, want an init-failure Error", got)
		}
	})

	t.Run("unregistered node type is reported", func(t *testing.T) {
		p := &fakeAutomationPlugin{}
		d := newDispatcher(p)
		payload, _ := json.Marshal(ActionRequest{NodeType: "unknown.type"})

		got := decodeActionResult(t, d.runAction(payload))
		if got.Applied || !strings.Contains(got.Error, "no action handler registered") {
			t.Fatalf("got %+v, want an unregistered-handler Error", got)
		}
	})
}
