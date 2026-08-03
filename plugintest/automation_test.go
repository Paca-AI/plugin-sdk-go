package plugintest

import (
	"testing"

	plugin "github.com/Paca-AI/plugin-sdk-go"
)

func TestContext_EvaluateCondition(t *testing.T) {
	tc := NewContext(t)
	tc.PluginContext().Condition("test.high_importance", func(req *plugin.ConditionRequest) plugin.ConditionResult {
		return plugin.ConditionResult{Matched: req.Task.Importance >= 5}
	})

	result := tc.EvaluateCondition("test.high_importance", ConditionRequest{
		Task: plugin.TaskSnapshot{Importance: 8},
	})
	if !result.Matched {
		t.Fatalf("expected Matched=true for Importance=8, got %+v", result)
	}

	result = tc.EvaluateCondition("test.high_importance", ConditionRequest{
		Task: plugin.TaskSnapshot{Importance: 1},
	})
	if result.Matched {
		t.Fatalf("expected Matched=false for Importance=1, got %+v", result)
	}
}

func TestContext_RunAction(t *testing.T) {
	tc := NewContext(t)
	tc.PluginContext().Action("test.mark_done", func(req *plugin.ActionRequest) plugin.ActionResult {
		return plugin.ActionResult{Applied: req.IdempotencyKey != ""}
	})

	result := tc.RunAction("test.mark_done", ActionRequest{IdempotencyKey: "run-1/node-2"})
	if !result.Applied {
		t.Fatalf("expected Applied=true, got %+v", result)
	}
}
