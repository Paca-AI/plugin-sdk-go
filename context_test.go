package plugin

import "testing"

// Regression test for a route-matching ambiguity: when a plugin registers
// both "/views/:viewId/panels/:panelId" and "/views/:viewId/panels/layout"
// under the same method, a request for ".../panels/layout" matches both
// patterns. matchRoute must always prefer the more specific (literal
// "layout") pattern over the wildcard ":panelId" one — previously this was
// resolved by Go's randomized map iteration order, so the "wrong" handler
// (treating "layout" as a panel ID) would win nondeterministically across
// calls. Note: plugintest.Context.Call bypasses this entirely by passing the
// literal ":panelId" pattern string as the path, so it never exercised this
// branch — this test calls matchRoute with a real resolved path instead.
func TestMatchRoute_PrefersLiteralSegmentOverWildcard(t *testing.T) {
	ctx := newContext(newWASMDBBackend(), newWASMKVBackend(), newWASMLogBackend(), newWASMConfigBackend(), newWASMPermissionBackend())

	var gotByPanelID, gotLayout bool
	ctx.Route("PATCH", "/views/:viewId/panels/:panelId", func(_ *Request, _ *Response) {
		gotByPanelID = true
	})
	ctx.Route("PATCH", "/views/:viewId/panels/layout", func(_ *Request, _ *Response) {
		gotLayout = true
	})

	for i := 0; i < 50; i++ {
		gotByPanelID, gotLayout = false, false
		handler, params, ok := ctx.matchRoute("PATCH", "/views/view-abc/panels/layout")
		if !ok {
			t.Fatalf("iteration %d: expected a route match, got none", i)
		}
		handler(nil, nil)
		if !gotLayout || gotByPanelID {
			t.Fatalf("iteration %d: expected the literal /panels/layout route to win, got panelId route instead (params=%v)", i, params)
		}
	}

	// Sanity check the wildcard route still matches a real panel ID.
	handler, params, ok := ctx.matchRoute("PATCH", "/views/view-abc/panels/panel-123")
	if !ok {
		t.Fatal("expected a route match for a real panel ID")
	}
	gotByPanelID, gotLayout = false, false
	handler(nil, nil)
	if !gotByPanelID || gotLayout {
		t.Fatalf("expected the :panelId route to win for a non-'layout' panel ID, got params=%v", params)
	}
	if params["panelId"] != "panel-123" {
		t.Fatalf("expected panelId param %q, got %q", "panel-123", params["panelId"])
	}
}
