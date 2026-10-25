package gitprovider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/fakeprovider"
)

func TestPermissionOrdering(t *testing.T) {
	order := []gp.Permission{gp.PermissionNone, gp.PermissionRead, gp.PermissionTriage, gp.PermissionWrite, gp.PermissionMaintain, gp.PermissionAdmin}
	for i, a := range order {
		for j, b := range order {
			if got, want := a.AtLeast(b), i >= j; got != want {
				t.Errorf("%s.AtLeast(%s) = %v, want %v", a, b, got, want)
			}
		}
	}
	if gp.Permission("owner").Valid() || gp.Permission("").AtLeast(gp.PermissionNone) {
		t.Error("an undefined permission must never satisfy a requirement, not even 'none'")
	}
}

func TestAsError_NeverLeaksInternals(t *testing.T) {
	e := gp.AsError(fmt.Errorf("dial tcp 10.0.0.5: token ghs_secret rejected"))
	if e.Code != gp.CodeInternal || e.Message == "" || contains(e.Message, "ghs_secret") || contains(e.Message, "10.0.0.5") {
		t.Errorf("unclassified errors must become a generic internal error, got %+v", e)
	}
	if !errors.Is(gp.Errorf(gp.CodeUnsupported, "x"), gp.ErrUnsupported) {
		t.Error("errors.Is must match on code")
	}
	if !gp.RateLimited(30, "slow down").Retryable() || gp.Errorf(gp.CodeForbidden, "no").Retryable() {
		t.Error("only rate_limited and transient are retryable")
	}
}

func TestOperationsTable(t *testing.T) {
	// Every optional operation names a capability that exists in Capabilities.
	raw, _ := json.Marshal(gp.Capabilities{})
	var caps map[string]any
	_ = json.Unmarshal(raw, &caps)
	seen := map[string]bool{}
	for _, op := range gp.Operations() {
		if seen[op.Name] {
			t.Errorf("duplicate operation %q", op.Name)
		}
		seen[op.Name] = true
		if op.Capability != "" {
			if _, ok := caps[op.Capability]; !ok {
				t.Errorf("%s references capability %q which is not a Capabilities field", op.Name, op.Capability)
			}
		}
	}
	// Every capability that gates an operation is covered by the table, so a new
	// optional interface cannot be added without a wire operation.
	for _, flag := range []string{"resolve_threads", "file_read", "code_search", "labels", "reviewer_requests", "assignees", "merge", "rerun_checks", "reactions", "webhook_management", "identity_deletion", "access_grants"} {
		found := false
		for _, op := range gp.Operations() {
			if op.Capability == flag {
				found = true
			}
		}
		if !found {
			t.Errorf("capability %q gates no operation", flag)
		}
	}
	if impl := gp.Implemented(fakeprovider.New()); !impl.Merge || !impl.WebhookManagement || !impl.IdentityDeletion {
		t.Error("Implemented() must see the full fake's optional interfaces")
	}
	if impl := gp.Implemented(fakeprovider.NewMinimal()); impl.Merge || impl.FileRead {
		t.Error("Implemented() must not report optional interfaces the minimal fake lacks")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

type panicProvider struct{ gp.Provider }

func (panicProvider) Describe() gp.Info { panic("boom") }

func TestDispatch_RecoversFromPanic(t *testing.T) {
	status, body := gp.Dispatch(context.Background(), panicProvider{}, "describe", nil)
	if status != 500 || strings.Contains(string(body), "boom") {
		t.Fatalf("status %d body %s; want a generic 500", status, body)
	}
}

// undeclared embeds a provider but reports the merge capability as off.
type undeclared struct{ gp.Provider }

func (u undeclared) Describe() gp.Info {
	i := u.Provider.Describe()
	i.Capabilities.Merge = false
	return i
}

func TestDispatch_UndeclaredCapabilityIsUnsupported(t *testing.T) {
	status, _ := gp.Dispatch(context.Background(), undeclared{fakeprovider.New()}, "merge_change_request", []byte(`{}`))
	if status != 501 {
		t.Fatalf("status %d; want 501 for an undeclared capability", status)
	}
}
