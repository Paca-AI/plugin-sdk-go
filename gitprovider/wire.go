package gitprovider

import (
	"context"
	"encoding/json"
	"net/http"
)

// Operation is one entry of the wire contract.
type Operation struct {
	Name string
	// Capability is empty for required operations, else the Capabilities flag
	// (JSON name) that must be true for the operation to be available.
	Capability string
}

type opFunc func(ctx context.Context, p Provider, body []byte) (any, error)

type opEntry struct {
	Operation
	call opFunc
}

func decode[Req any](body []byte) (Req, error) {
	var r Req
	if len(body) == 0 {
		return r, nil
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return r, Errorf(CodeValidation, "malformed request body")
	}
	return r, nil
}

// required wraps a method of Provider.
func required[Req, Res any](name string, fn func(Provider, context.Context, Req) (Res, error)) opEntry {
	return opEntry{Operation{Name: name}, func(ctx context.Context, p Provider, body []byte) (any, error) {
		req, err := decode[Req](body)
		if err != nil {
			return nil, err
		}
		return fn(p, ctx, req)
	}}
}

// optional wraps a method of an optional interface; a provider that does not
// implement I answers [ErrUnsupported].
func optional[I any, Req, Res any](name, capability string, fn func(I, context.Context, Req) (Res, error)) opEntry {
	return opEntry{Operation{Name: name, Capability: capability}, func(ctx context.Context, p Provider, body []byte) (any, error) {
		impl, ok := p.(I)
		if !ok {
			return nil, ErrUnsupported
		}
		req, err := decode[Req](body)
		if err != nil {
			return nil, err
		}
		return fn(impl, ctx, req)
	}}
}

var operations = []opEntry{
	{Operation{Name: "describe"}, func(_ context.Context, p Provider, _ []byte) (any, error) { return p.Describe(), nil }},
	required("begin_identity_setup", Provider.BeginIdentitySetup),
	required("complete_identity_setup", Provider.CompleteIdentitySetup),
	required("list_repositories", Provider.ListRepositories),
	required("resolve_repository", Provider.ResolveRepository),
	required("verify_and_parse_webhook", Provider.VerifyAndParseWebhook),
	required("mint_credential", Provider.MintCredential),
	required("get_actor_permission", Provider.GetActorPermission),
	required("get_branch_protection", Provider.GetBranchProtection),
	required("get_change_request", Provider.GetChangeRequest),
	required("list_changed_files", Provider.ListChangedFiles),
	required("get_diff", Provider.GetDiff),
	required("list_threads", Provider.ListThreads),
	required("list_comments", Provider.ListComments),
	required("get_issue", Provider.GetIssue),
	required("get_checks", Provider.GetChecks),
	required("get_check_logs", Provider.GetCheckLogs),
	required("open_change_request", Provider.OpenChangeRequest),
	required("update_change_request", Provider.UpdateChangeRequest),
	required("comment", Provider.Comment),
	required("reply_thread", Provider.ReplyThread),
	required("submit_review", Provider.SubmitReview),
	required("set_status", Provider.SetStatus),
	optional("resolve_thread", "resolve_threads", ThreadResolver.ResolveThread),
	optional("get_file", "file_read", FileReader.GetFile),
	optional("search_code", "code_search", CodeSearcher.SearchCode),
	optional("add_labels", "labels", LabelManager.AddLabels),
	optional("request_reviewers", "reviewer_requests", ReviewerRequester.RequestReviewers),
	optional("set_assignees", "assignees", AssigneeSetter.SetAssignees),
	optional("merge_change_request", "merge", Merger.MergeChangeRequest),
	optional("rerun_checks", "rerun_checks", CheckRerunner.RerunChecks),
	optional("react", "reactions", Reactor.React),
	optional("ensure_webhook", "webhook_management", WebhookManager.EnsureWebhook),
	optional("remove_webhook", "webhook_management", WebhookManager.RemoveWebhook),
	optional("delete_identity", "identity_deletion", IdentityDeleter.DeleteIdentity),
	optional("begin_access_grant", "access_grants", AccessGranter.BeginAccessGrant),
	optional("complete_access_grant", "access_grants", AccessGranter.CompleteAccessGrant),
}

var opIndex = func() map[string]opEntry {
	m := make(map[string]opEntry, len(operations))
	for _, o := range operations {
		m[o.Name] = o
	}
	return m
}()

// Operations lists the wire contract, in a stable order.
func Operations() []Operation {
	out := make([]Operation, len(operations))
	for i, o := range operations {
		out[i] = o.Operation
	}
	return out
}

// Dispatch runs one operation by wire name against p and returns the HTTP
// status and the JSON [Envelope]. It never panics on bad input: unknown
// operations and malformed bodies become typed errors.
func Dispatch(ctx context.Context, p Provider, op string, body []byte) (int, []byte) {
	entry, ok := opIndex[op]
	if !ok {
		return fail(Errorf(CodeUnsupported, "unknown operation %q", op))
	}
	result, err := entry.call(ctx, p, body)
	if err != nil {
		return fail(AsError(err))
	}
	raw, mErr := json.Marshal(result)
	if mErr != nil {
		return fail(Errorf(CodeInternal, "could not encode result"))
	}
	out, _ := json.Marshal(Envelope{OK: true, Result: raw})
	return http.StatusOK, out
}

func fail(e *Error) (int, []byte) {
	out, _ := json.Marshal(Envelope{OK: false, Error: e})
	return e.Code.HTTPStatus(), out
}
