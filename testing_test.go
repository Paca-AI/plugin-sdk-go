package plugin

import "testing"

func newTestContext() *Context {
	return NewContextForTest(
		newWASMDBBackend(),
		newWASMKVBackend(),
		newWASMCacheBackend(),
		newWASMLogBackend(),
		newWASMConfigBackend(),
		newWASMPermissionBackend(),
	)
}

func TestDispatchCondition(t *testing.T) {
	t.Run("found handler", func(t *testing.T) {
		ctx := newTestContext()
		ctx.Condition("test.cond", func(req *ConditionRequest) ConditionResult {
			return ConditionResult{Matched: req.ProjectID == "proj-1"}
		})

		result, ok := DispatchCondition(ctx, &ConditionRequest{NodeType: "test.cond", ProjectID: "proj-1"})
		if !ok {
			t.Fatal("expected a registered handler to be found")
		}
		if !result.Matched {
			t.Fatalf("expected Matched=true, got %+v", result)
		}
	})

	t.Run("no handler registered", func(t *testing.T) {
		ctx := newTestContext()
		_, ok := DispatchCondition(ctx, &ConditionRequest{NodeType: "missing.type"})
		if ok {
			t.Fatal("expected no handler to be found for an unregistered node type")
		}
	})
}

func TestDispatchAction(t *testing.T) {
	t.Run("found handler", func(t *testing.T) {
		ctx := newTestContext()
		ctx.Action("test.action", func(req *ActionRequest) ActionResult {
			return ActionResult{Applied: req.IdempotencyKey != ""}
		})

		result, ok := DispatchAction(ctx, &ActionRequest{NodeType: "test.action", IdempotencyKey: "k1"})
		if !ok {
			t.Fatal("expected a registered handler to be found")
		}
		if !result.Applied {
			t.Fatalf("expected Applied=true, got %+v", result)
		}
	})

	t.Run("no handler registered", func(t *testing.T) {
		ctx := newTestContext()
		_, ok := DispatchAction(ctx, &ActionRequest{NodeType: "missing.type"})
		if ok {
			t.Fatal("expected no handler to be found for an unregistered node type")
		}
	})
}
