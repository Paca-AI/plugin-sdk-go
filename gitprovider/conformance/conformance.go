// Package conformance is the test suite every Git provider must pass.
//
// A provider author supplies a [Harness]: a constructor that returns the
// provider wired to a fake backend (recorded fixtures / a local HTTP stub) and
// the [Fixtures] describing that backend's seed data. Run executes the suite as
// subtests. The suite checks behaviour core depends on — not the platform's
// semantics — so a passing provider is one core can drive safely:
//
//   - capabilities are honest (every optional interface has its flag, and the
//     other way round);
//   - webhooks are verified, parsed deterministically, and never leak secrets;
//   - errors are classified onto the SPI error set and never contain secrets;
//   - reads and writes behave consistently (paging, truncation, idempotency,
//     validation, emulated batch review);
//   - unsupported operations answer "unsupported" instead of panicking.
//
// See gitprovider/fakeprovider for a harness that passes in full.
package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
)

// Harness wires a provider to its test backend.
type Harness struct {
	// New returns a fresh provider with the seed data in Fixtures. It is called
	// once per subtest so mutations never leak between tests.
	New      func(t *testing.T) gp.Provider
	Fixtures Fixtures
}

// Want is the expectation for one normalized event.
type Want struct {
	Kind          gp.EventKind
	Action        string
	RepoPath      string
	SubjectKind   gp.SubjectKind // empty: not checked
	SubjectNumber string         // empty: not checked
	ActorLogin    string
	ActorKind     gp.ActorKind
	// Addressed lists, in any order, how the fixture identity was addressed.
	Addressed []gp.AddressVia
	// CheckConclusion is checked for EventCheck.
	CheckConclusion string
}

// WebhookCase is one canned delivery and the events it must produce. An empty
// Want asserts the delivery is dropped (no events).
type WebhookCase struct {
	Name    string
	Request gp.WebhookRequest
	Want    []Want
}

// Fixtures describe the backend's seed data.
type Fixtures struct {
	Identity gp.IdentityRef
	// Repo is a repository the identity can see, holding the change request,
	// issue and thread below.
	Repo          gp.RepoRef
	InvisibleRepo gp.RepoRef // a repository the identity cannot see
	RepoURL       string     // a URL or path that ResolveRepository must resolve to Repo
	Candidates    []gp.IdentityRef
	// ExpectedRepoPaths are all repositories the identity can see (for paging).
	ExpectedRepoPaths []string

	ChangeRequest  string   // an existing change request number
	MissingCR      string   // a number that does not exist
	ChangedFiles   []string // all files of ChangeRequest
	HeadSHA        string   // ref with checks
	FailingCheckID string   // a check with logs
	IssueNumber    string
	ThreadID       string
	// Actors maps logins to the permission GetActorPermission must return.
	Actors map[string]gp.Permission

	// Webhooks are canned deliveries; BadSignature must fail verification.
	Webhooks     []WebhookCase
	BadSignature gp.WebhookRequest

	// Secrets are values that must never appear in events or error messages
	// (webhook secrets, stored tokens). List every token the harness uses: the
	// suite can only detect leaks of values it is told about.
	Secrets []string

	// Setup: a host and the parameters that complete setup successfully.
	SetupHost           string
	SetupCompleteParams map[string]string

	// Writes run only when true (point real providers at a sandbox).
	MutationsAllowed bool
	NewBranchHead    string // an existing branch with no open change request
	// DraftBranchHead and IdempotentBranchHead are further existing branches
	// with no open change request, used for the draft and idempotency checks.
	// Defaults ("draft-branch", "idem-branch") suit the fake backend only.
	DraftBranchHead      string
	IdempotentBranchHead string
	// NewBranchName is a branch name CreateBranch can create (providers
	// declaring Branches). Default "paca/conformance". The suite never deletes
	// it, so a sandbox run needs a fresh name each time or manual cleanup.
	NewBranchName string
	BaseBranch    string
	File          struct{ Ref, Path string }
	SearchQuery   string // a query that matches at least one file
	Reviewer      string // a login that can be requested as reviewer

	// AccessGrantParams are the callback parameters that complete an access
	// grant started by BeginAccessGrant (providers declaring AccessGrants).
	AccessGrantParams map[string]string
	AccessOwner       string // an owner to pass to BeginAccessGrant
}

// Run executes the suite.
func Run(t *testing.T, h Harness) {
	t.Helper()
	if h.New == nil {
		t.Fatal("conformance: Harness.New is required")
	}
	t.Run("describe", func(t *testing.T) { testDescribe(t, h) })
	t.Run("wire", func(t *testing.T) { testWire(t, h) })
	t.Run("setup", func(t *testing.T) { testSetup(t, h) })
	t.Run("repositories", func(t *testing.T) { testRepositories(t, h) })
	t.Run("webhooks", func(t *testing.T) { testWebhooks(t, h) })
	t.Run("credentials", func(t *testing.T) { testCredentials(t, h) })
	t.Run("policy-inputs", func(t *testing.T) { testPolicyInputs(t, h) })
	t.Run("reads", func(t *testing.T) { testReads(t, h) })
	t.Run("unsupported", func(t *testing.T) { testUnsupported(t, h) })
	t.Run("access-grants", func(t *testing.T) { testAccessGrants(t, h) })
	if h.Fixtures.MutationsAllowed {
		t.Run("writes", func(t *testing.T) { testWrites(t, h) })
		t.Run("optional-writes", func(t *testing.T) { testOptionalWrites(t, h) })
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func target(f Fixtures) gp.Target { return gp.Target{Identity: f.Identity, Repo: f.Repo} }

func mustCode(t *testing.T, f Fixtures, err error, codes ...gp.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with code %v, got success", codes)
	}
	var pe *gp.Error
	if !errors.As(err, &pe) {
		t.Fatalf("error is not a *gitprovider.Error (unclassified): %T %v — map platform failures with gitprovider.Errorf", err, err)
	}
	for _, c := range codes {
		if pe.Code == c {
			noLeak(t, f, pe.Message)
			return
		}
	}
	t.Fatalf("error code = %q, want one of %v (message %q)", pe.Code, codes, pe.Message)
}

func noLeak(t *testing.T, f Fixtures, text string) {
	t.Helper()
	for _, s := range f.Secrets {
		if s != "" && strings.Contains(text, s) {
			t.Fatalf("a secret leaked into output: %q", truncate(text))
		}
	}
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func truncate(s string) string {
	if len(s) > 120 {
		return strings.ToValidUTF8(s[:120], "") + "…"
	}
	return s
}

func isValidTrigger(k gp.TriggerKind) bool {
	for _, v := range gp.AllTriggerKinds {
		if v == k {
			return true
		}
	}
	return false
}

func validEventKind(k gp.EventKind) bool {
	for _, v := range gp.AllEventKinds {
		if v == k {
			return true
		}
	}
	return false
}

func ctx() context.Context { return context.Background() }

// ── describe ─────────────────────────────────────────────────────────────────

func testDescribe(t *testing.T, h Harness) {
	p := h.New(t)
	info := p.Describe()
	if info.SPIVersion != gp.SPIVersion {
		t.Errorf("SPIVersion = %q, want %q", info.SPIVersion, gp.SPIVersion)
	}
	if info.ID == "" || info.DisplayName == "" {
		t.Error("ID and DisplayName are required")
	}
	if len(info.Hosts) == 0 {
		t.Error("at least one default host is required")
	}
	if info.Terms.ChangeRequest == "" || info.Terms.ChangeRequestPlural == "" || info.Terms.Repository == "" {
		t.Error("Terms must name the change request (singular and plural) and repository")
	}
	c := info.Capabilities
	if len(c.IdentityKinds) == 0 {
		t.Error("IdentityKinds must not be empty")
	}
	for _, k := range c.IdentityKinds {
		if k != gp.IdentityApp && k != gp.IdentityBotUser && k != gp.IdentityToken {
			t.Errorf("unknown identity kind %q", k)
		}
	}
	if c.CredentialModel != gp.CredentialEphemeral && c.CredentialModel != gp.CredentialStatic {
		t.Errorf("CredentialModel %q is invalid", c.CredentialModel)
	}
	if c.WebhookScope != gp.WebhookApp && c.WebhookScope != gp.WebhookGroup && c.WebhookScope != gp.WebhookRepo {
		t.Errorf("WebhookScope %q is invalid", c.WebhookScope)
	}
	if c.StatusKind != gp.StatusCheckRun && c.StatusKind != gp.StatusCommitStatus && c.StatusKind != gp.StatusComment {
		t.Errorf("StatusKind %q is invalid", c.StatusKind)
	}
	seen := map[gp.TriggerKind]bool{}
	for _, k := range c.Triggers {
		if !isValidTrigger(k) {
			t.Errorf("unknown trigger %q", k)
		}
		if seen[k] {
			t.Errorf("duplicate trigger %q", k)
		}
		seen[k] = true
	}
	for _, g := range c.CIConfigGlobs {
		if strings.TrimSpace(g) == "" {
			t.Error("CIConfigGlobs must not contain empty patterns")
		}
	}
	// A webhook the provider must manage itself implies it can manage it.
	if c.WebhookScope != gp.WebhookApp && !c.WebhookManagement {
		t.Log("note: repo/group-scoped webhooks without WebhookManagement mean users must register webhooks by hand")
	}

	// Capability honesty, both directions.
	impl := gp.Implemented(p)
	check := func(name string, declared, implemented bool) {
		switch {
		case declared && !implemented:
			t.Errorf("capability %s is declared but the optional interface is not implemented", name)
		case !declared && implemented:
			t.Errorf("optional interface for %s is implemented but the capability is not declared (core would never call it)", name)
		}
	}
	check("resolve_threads", c.ResolveThreads, impl.ResolveThreads)
	check("file_read", c.FileRead, impl.FileRead)
	check("code_search", c.CodeSearch, impl.CodeSearch)
	check("labels", c.Labels, impl.Labels)
	check("reviewer_requests", c.ReviewerRequests, impl.ReviewerRequests)
	check("assignees", c.Assignees, impl.Assignees)
	check("merge", c.Merge, impl.Merge)
	check("rerun_checks", c.RerunChecks, impl.RerunChecks)
	check("reactions", c.Reactions, impl.Reactions)
	check("webhook_management", c.WebhookManagement, impl.WebhookManagement)
	check("identity_deletion", c.IdentityDeletion, impl.IdentityDeletion)
	check("branches", c.Branches, impl.Branches)
	check("access_grants", c.AccessGrants, impl.AccessGrants)
}

// ── wire ─────────────────────────────────────────────────────────────────────

func testWire(t *testing.T, h Harness) {
	p := h.New(t)

	status, body := gp.Dispatch(ctx(), p, "describe", nil)
	if status != 200 {
		t.Fatalf("describe status %d: %s", status, body)
	}
	var env gp.Envelope
	if err := json.Unmarshal(body, &env); err != nil || !env.OK {
		t.Fatalf("describe envelope: %v %s", err, body)
	}
	want, _ := json.Marshal(p.Describe())
	if string(env.Result) != string(want) {
		t.Errorf("describe over the wire differs from Describe():\n got %s\nwant %s", env.Result, want)
	}

	status, body = gp.Dispatch(ctx(), p, "no_such_operation", nil)
	if status != gp.CodeUnsupported.HTTPStatus() || !strings.Contains(string(body), string(gp.CodeUnsupported)) {
		t.Errorf("unknown operation: status %d body %s, want unsupported", status, body)
	}

	// An empty body is a validation error, not a call with a zero-value request.
	for _, op := range gp.Operations() {
		if op.Name == "describe" {
			continue
		}
		_, body := gp.Dispatch(ctx(), p, op.Name, nil)
		var e gp.Envelope
		if err := json.Unmarshal(body, &e); err != nil || e.OK || e.Error == nil ||
			(e.Error.Code != gp.CodeValidation && e.Error.Code != gp.CodeUnsupported) {
			t.Errorf("%s with an empty body: %s, want validation or unsupported", op.Name, body)
		}
	}

	// Every operation survives a malformed body without panicking and says so
	// with a validation (or unsupported) error.
	for _, op := range gp.Operations() {
		if op.Name == "describe" {
			continue
		}
		status, body := gp.Dispatch(ctx(), p, op.Name, []byte(`{not json`))
		var e gp.Envelope
		if err := json.Unmarshal(body, &e); err != nil || e.OK || e.Error == nil {
			t.Errorf("%s with malformed body: status %d body %s", op.Name, status, body)
			continue
		}
		if e.Error.Code != gp.CodeValidation && e.Error.Code != gp.CodeUnsupported {
			t.Errorf("%s with malformed body: code %q, want validation or unsupported", op.Name, e.Error.Code)
		}
	}
}

// ── setup ────────────────────────────────────────────────────────────────────

func testSetup(t *testing.T, h Harness) {
	f := h.Fixtures
	if f.SetupHost == "" {
		t.Skip("Fixtures.SetupHost not set")
	}
	p := h.New(t)
	step, err := p.BeginIdentitySetup(ctx(), gp.BeginSetupRequest{Scope: gp.SetupShared, Host: f.SetupHost,
		ReturnURL: "https://paca.test/return", CallbackURL: "https://paca.test/cb", WebhookURL: "https://paca.test/hook"})
	if err != nil {
		t.Fatalf("BeginIdentitySetup: %v", err)
	}
	switch step.Kind {
	case gp.StepRedirect, gp.StepFormPost:
		if step.URL == "" {
			t.Error("redirect/form_post steps need a URL")
		}
	case gp.StepTokenForm:
		if step.Prompt == "" {
			t.Error("token_form steps need a Prompt saying which scopes the token needs")
		}
		if len(step.Form) == 0 {
			t.Error("token_form steps must list their inputs in Form")
		}
		for _, ff := range step.Form {
			if ff.Name == "" || ff.Label == "" {
				t.Errorf("form field needs a name and a label: %+v", ff)
			}
		}
	case gp.StepDone:
	default:
		t.Errorf("unknown step kind %q", step.Kind)
	}
	if _, err := p.BeginIdentitySetup(ctx(), gp.BeginSetupRequest{Scope: gp.SetupShared, Host: "unknown.invalid"}); err == nil {
		t.Error("an unknown host must be rejected")
	} else {
		mustCode(t, f, err, gp.CodeValidation)
	}
	if !p.Describe().Capabilities.MultipleIdentities {
		_, err := p.BeginIdentitySetup(ctx(), gp.BeginSetupRequest{Scope: gp.SetupAgent, Host: f.SetupHost, AgentID: "a1", AgentName: "dev"})
		mustCode(t, f, err, gp.CodeUnsupported)
	}
	if len(f.SetupCompleteParams) > 0 {
		id, err := p.CompleteIdentitySetup(ctx(), gp.CompleteSetupRequest{Scope: gp.SetupShared, Host: f.SetupHost, State: step.State, Params: f.SetupCompleteParams})
		if err != nil {
			t.Fatalf("CompleteIdentitySetup: %v", err)
		}
		if id.Ref.ID == "" || id.Host == "" || id.DisplayName == "" || id.Kind == "" {
			t.Errorf("incomplete identity: %+v", id)
		}
		noLeak(t, f, mustJSON(id))
	}
	// Missing required inputs must be rejected, not stored as an empty identity.
	_, err = p.CompleteIdentitySetup(ctx(), gp.CompleteSetupRequest{Scope: gp.SetupShared, Host: f.SetupHost})
	mustCode(t, f, err, gp.CodeValidation, gp.CodeInvalidSignature, gp.CodeForbidden)
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// ── repositories ─────────────────────────────────────────────────────────────

func testRepositories(t *testing.T, h Harness) {
	f := h.Fixtures
	p := h.New(t)

	all, err := p.ListRepositories(ctx(), gp.ListReposRequest{Identity: f.Identity})
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	var paths []string
	for _, r := range all.Items {
		paths = append(paths, r.Ref.FullPath)
		if r.Ref.Host == "" || r.Ref.FullPath == "" || r.Ref.CloneURL == "" {
			t.Errorf("repository %+v needs host, full_path and clone_url", r.Ref)
		}
		if u, err := url.Parse(r.Ref.CloneURL); err == nil && u.User != nil {
			t.Errorf("clone_url %q must be credential-free", r.Ref.CloneURL)
		}
	}
	sort.Strings(paths)
	if len(f.ExpectedRepoPaths) > 0 {
		want := append([]string(nil), f.ExpectedRepoPaths...)
		sort.Strings(want)
		if strings.Join(paths, ",") != strings.Join(want, ",") {
			t.Errorf("repositories = %v, want %v", paths, want)
		}
	}

	// Paging with page size 1 must visit exactly the same repositories.
	var paged []string
	cursor := ""
	for i := 0; i < 100; i++ {
		page, err := p.ListRepositories(ctx(), gp.ListReposRequest{Identity: f.Identity, PageSize: 1, Cursor: cursor})
		if err != nil {
			t.Fatalf("paged ListRepositories: %v", err)
		}
		if len(page.Items) > 1 {
			t.Fatalf("PageSize 1 returned %d items", len(page.Items))
		}
		for _, r := range page.Items {
			paged = append(paged, r.Ref.FullPath)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	sort.Strings(paged)
	if strings.Join(paged, ",") != strings.Join(paths, ",") {
		t.Errorf("paging visited %v, a single page returned %v", paged, paths)
	}

	_, err = p.ListRepositories(ctx(), gp.ListReposRequest{Identity: gp.IdentityRef{ID: "no-such-identity"}})
	mustCode(t, f, err, gp.CodeForbidden, gp.CodeNotFound)

	if f.RepoURL != "" {
		res, err := p.ResolveRepository(ctx(), gp.ResolveRepoRequest{PathOrURL: f.RepoURL, Candidates: f.Candidates})
		if err != nil {
			t.Fatalf("ResolveRepository: %v", err)
		}
		if res.Repository.Ref.FullPath != f.Repo.FullPath {
			t.Errorf("resolved %q, want %q", res.Repository.Ref.FullPath, f.Repo.FullPath)
		}
		if res.Identity.ID == "" {
			t.Error("ResolveRepository must say which identity can see the repository")
		}
		_, err = p.ResolveRepository(ctx(), gp.ResolveRepoRequest{PathOrURL: f.RepoURL})
		mustCode(t, f, err, gp.CodeForbidden, gp.CodeNotFound)
	}
	_, err = p.ResolveRepository(ctx(), gp.ResolveRepoRequest{PathOrURL: "definitely/not-a-repo-xyz", Candidates: f.Candidates})
	mustCode(t, f, err, gp.CodeNotFound, gp.CodeForbidden)
}

// ── webhooks ─────────────────────────────────────────────────────────────────

func testWebhooks(t *testing.T, h Harness) {
	f := h.Fixtures
	p := h.New(t)
	caps := p.Describe().Capabilities

	if f.BadSignature.Body != nil || len(f.BadSignature.Headers) > 0 {
		evs, err := p.VerifyAndParseWebhook(ctx(), f.BadSignature)
		mustCode(t, f, err, gp.CodeInvalidSignature)
		if len(evs) != 0 {
			t.Error("a delivery that fails verification must produce no events")
		}
	} else {
		t.Error("Fixtures.BadSignature is required: every provider must verify deliveries")
	}
	if len(f.Webhooks) == 0 {
		t.Fatal("Fixtures.Webhooks is required")
	}

	for _, wc := range f.Webhooks {
		t.Run(wc.Name, func(t *testing.T) {
			evs, err := p.VerifyAndParseWebhook(ctx(), wc.Request)
			if err != nil {
				t.Fatalf("VerifyAndParseWebhook: %v", err)
			}
			if len(evs) != len(wc.Want) {
				t.Fatalf("got %d events, want %d: %s", len(evs), len(wc.Want), mustJSON(evs))
			}
			ids := map[string]bool{}
			for i, ev := range evs {
				checkEventInvariants(t, f, caps, ev)
				if ids[ev.ID] {
					t.Errorf("duplicate event id %q within one delivery", ev.ID)
				}
				ids[ev.ID] = true
				checkWant(t, ev, wc.Want[i], caps)
			}
			// Deterministic: the same delivery yields the same event ids (core's
			// idempotency depends on it).
			again, err := p.VerifyAndParseWebhook(ctx(), wc.Request)
			if err != nil || len(again) != len(evs) {
				t.Fatalf("second parse: %v (%d events)", err, len(again))
			}
			for i := range evs {
				if again[i].ID != evs[i].ID {
					t.Errorf("event id is not deterministic: %q vs %q", again[i].ID, evs[i].ID)
				}
			}
		})
	}
}

func checkEventInvariants(t *testing.T, f Fixtures, caps gp.Capabilities, ev gp.Event) {
	t.Helper()
	if ev.ID == "" {
		t.Error("event id is required")
	}
	if !validEventKind(ev.Kind) {
		t.Errorf("unknown event kind %q", ev.Kind)
	}
	if ev.Action == "" {
		t.Error("event action is required")
	}
	if ev.Repo.FullPath == "" || ev.Repo.Host == "" {
		t.Errorf("event repo needs host and full_path: %+v", ev.Repo)
	}
	if ev.Actor.Login == "" {
		t.Error("event actor login is required")
	}
	if ev.Actor.Kind != gp.ActorUser && ev.Actor.Kind != gp.ActorBot {
		t.Errorf("actor kind %q is invalid", ev.Actor.Kind)
	}
	if ev.Actor.Permission != "" && !ev.Actor.Permission.Valid() {
		t.Errorf("actor permission %q is invalid", ev.Actor.Permission)
	}
	if ev.Subject != nil {
		if ev.Subject.Kind != gp.SubjectIssue && ev.Subject.Kind != gp.SubjectChangeRequest {
			t.Errorf("subject kind %q is invalid", ev.Subject.Kind)
		}
		if ev.Subject.Number == "" {
			t.Error("subject number is required")
		}
	}
	if ev.Subject != nil && ev.Subject.URL != "" {
		if u, err := url.Parse(ev.Subject.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			t.Errorf("subject url %q must be an absolute http(s) URL", ev.Subject.URL)
		}
	}
	if ev.Kind == gp.EventCheck && (ev.Check == nil || ev.Check.Conclusion == "") {
		t.Error("check events need check.conclusion")
	}
	if ev.Kind == gp.EventPush && (ev.Push == nil || ev.Push.Ref == "") {
		t.Error("push events need push.ref (the branch name)")
	}
	for _, a := range ev.Addressed {
		if a.IdentityID != f.Identity.ID {
			t.Errorf("addressed identity %q is not the fixture identity %q", a.IdentityID, f.Identity.ID)
		}
		switch a.Via {
		case gp.ViaMention, gp.ViaReviewRequest, gp.ViaAssignment, gp.ViaLabel:
		default:
			t.Errorf("unknown addressed.via %q", a.Via)
			continue
		}
		if need := triggerFor(a.Via); !caps.SupportsTrigger(need) {
			t.Errorf("event reports being addressed via %q but the %q trigger is not declared in capabilities", a.Via, need)
		}
	}
	noLeak(t, f, mustJSON(ev))
}

func triggerFor(v gp.AddressVia) gp.TriggerKind {
	switch v {
	case gp.ViaMention:
		return gp.TriggerMention
	case gp.ViaReviewRequest:
		return gp.TriggerReviewRequest
	case gp.ViaAssignment:
		return gp.TriggerAssignment
	}
	return gp.TriggerLabel
}

func checkWant(t *testing.T, ev gp.Event, w Want, caps gp.Capabilities) {
	t.Helper()
	if ev.Kind != w.Kind {
		t.Errorf("kind = %q, want %q", ev.Kind, w.Kind)
	}
	if w.Action != "" && ev.Action != w.Action {
		t.Errorf("action = %q, want %q", ev.Action, w.Action)
	}
	if w.RepoPath != "" && ev.Repo.FullPath != w.RepoPath {
		t.Errorf("repo = %q, want %q", ev.Repo.FullPath, w.RepoPath)
	}
	if w.SubjectKind != "" && (ev.Subject == nil || ev.Subject.Kind != w.SubjectKind) {
		t.Errorf("subject kind mismatch: %+v want %q", ev.Subject, w.SubjectKind)
	}
	if w.SubjectNumber != "" && (ev.Subject == nil || ev.Subject.Number != w.SubjectNumber) {
		t.Errorf("subject number mismatch: %+v want %q", ev.Subject, w.SubjectNumber)
	}
	if w.ActorLogin != "" && ev.Actor.Login != w.ActorLogin {
		t.Errorf("actor = %q, want %q", ev.Actor.Login, w.ActorLogin)
	}
	if w.ActorKind != "" && ev.Actor.Kind != w.ActorKind {
		t.Errorf("actor kind = %q, want %q", ev.Actor.Kind, w.ActorKind)
	}
	var got, want []string
	for _, a := range ev.Addressed {
		got = append(got, string(a.Via))
	}
	for _, v := range w.Addressed {
		// A way of being addressed is only expected from providers that declare
		// the matching trigger; the same fixture serves rich and basic platforms.
		if caps.SupportsTrigger(triggerFor(v)) {
			want = append(want, string(v))
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("addressed = %v, want %v", got, want)
	}
	if w.CheckConclusion != "" && (ev.Check == nil || ev.Check.Conclusion != w.CheckConclusion) {
		t.Errorf("check conclusion mismatch: %+v want %q", ev.Check, w.CheckConclusion)
	}
}

// ── credentials ──────────────────────────────────────────────────────────────

func testCredentials(t *testing.T, h Harness) {
	f := h.Fixtures
	p := h.New(t)
	caps := p.Describe().Capabilities

	for _, access := range []gp.Access{gp.AccessRead, gp.AccessWrite} {
		c, err := p.MintCredential(ctx(), gp.MintRequest{Target: target(f), Access: access, TTLSeconds: 300})
		if err != nil {
			t.Fatalf("MintCredential(%s): %v", access, err)
		}
		if c.Secret == "" || c.Username == "" {
			t.Errorf("credential needs username and secret: username=%q secret-empty=%v", c.Username, c.Secret == "")
		}
		if caps.CredentialModel == gp.CredentialEphemeral {
			if c.ExpiresAt.IsZero() {
				t.Error("ephemeral credentials must carry ExpiresAt")
			} else if d := time.Until(c.ExpiresAt); d <= 0 || d > 65*time.Minute {
				t.Errorf("ephemeral credential lifetime %v is outside (0, 65m]", d)
			} else if d > 10*time.Minute {
				t.Errorf("ephemeral credential lifetime %v exceeds the requested TTL of 5m (10m allowed for clock skew)", d)
			}
		}
	}
	_, err := p.MintCredential(ctx(), gp.MintRequest{Target: target(f), Access: "admin"})
	mustCode(t, f, err, gp.CodeValidation)

	if f.InvisibleRepo.FullPath != "" {
		_, err = p.MintCredential(ctx(), gp.MintRequest{Target: gp.Target{Identity: f.Identity, Repo: f.InvisibleRepo}, Access: gp.AccessRead})
		mustCode(t, f, err, gp.CodeNotFound, gp.CodeForbidden)
	}
}

// ── policy inputs ────────────────────────────────────────────────────────────

func testPolicyInputs(t *testing.T, h Harness) {
	f := h.Fixtures
	p := h.New(t)
	for login, want := range f.Actors {
		got, err := p.GetActorPermission(ctx(), gp.ActorPermissionRequest{Target: target(f), Login: login})
		if err != nil {
			t.Fatalf("GetActorPermission(%s): %v", login, err)
		}
		if !got.Valid() {
			t.Errorf("GetActorPermission(%s) = %q, not a defined permission", login, got)
		}
		if got != want {
			t.Errorf("GetActorPermission(%s) = %q, want %q", login, got, want)
		}
	}
	// An unknown login has no access — not an error.
	got, err := p.GetActorPermission(ctx(), gp.ActorPermissionRequest{Target: target(f), Login: "nobody-at-all-xyz"})
	if err != nil || got != gp.PermissionNone {
		t.Errorf("unknown login: %q, %v; want none and no error", got, err)
	}
	if _, err := p.GetBranchProtection(ctx(), gp.BranchProtectionRequest{Target: target(f), Branch: f.Repo.DefaultBranch}); err != nil {
		t.Errorf("GetBranchProtection: %v", err)
	}
}

// ── reads ────────────────────────────────────────────────────────────────────

func testReads(t *testing.T, h Harness) {
	f := h.Fixtures
	p := h.New(t)
	tg := target(f)

	cr, err := p.GetChangeRequest(ctx(), gp.CRRequest{Target: tg, Number: f.ChangeRequest})
	if err != nil {
		t.Fatalf("GetChangeRequest: %v", err)
	}
	switch cr.State {
	case gp.CROpen, gp.CRClosed, gp.CRMerged:
	default:
		t.Errorf("state %q is invalid", cr.State)
	}
	if cr.Number != f.ChangeRequest || cr.Title == "" || cr.Head.Ref == "" || cr.Base.Ref == "" || cr.URL == "" || cr.Author == "" {
		t.Errorf("incomplete change request: %+v", cr)
	}
	_, err = p.GetChangeRequest(ctx(), gp.CRRequest{Target: tg, Number: f.MissingCR})
	mustCode(t, f, err, gp.CodeNotFound)

	// Changed files: all pages, no duplicates, exactly the expected set.
	var files []string
	seen := map[string]bool{}
	cursor := ""
	for i := 0; i < 100; i++ {
		page, err := p.ListChangedFiles(ctx(), gp.PagedCRRequest{Target: tg, Number: f.ChangeRequest, Cursor: cursor})
		if err != nil {
			t.Fatalf("ListChangedFiles: %v", err)
		}
		for _, cf := range page.Items {
			if seen[cf.Path] {
				t.Errorf("file %q listed twice across pages", cf.Path)
			}
			seen[cf.Path] = true
			files = append(files, cf.Path)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	sort.Strings(files)
	want := append([]string(nil), f.ChangedFiles...)
	sort.Strings(want)
	if len(want) > 0 && strings.Join(files, ",") != strings.Join(want, ",") {
		t.Errorf("changed files = %v, want %v", files, want)
	}

	full, err := p.GetDiff(ctx(), gp.DiffRequest{Target: tg, Number: f.ChangeRequest})
	if err != nil || full.Text == "" {
		t.Fatalf("GetDiff: %v (empty=%v)", err, full.Text == "")
	}
	if len(files) > 0 {
		one, err := p.GetDiff(ctx(), gp.DiffRequest{Target: tg, Number: f.ChangeRequest, Path: files[0]})
		if err != nil || one.Text == "" || len(one.Text) > len(full.Text) {
			t.Errorf("per-file GetDiff: %v len=%d (whole=%d)", err, len(one.Text), len(full.Text))
		}
		_, err = p.GetDiff(ctx(), gp.DiffRequest{Target: tg, Number: f.ChangeRequest, Path: "no/such/file.xyz"})
		mustCode(t, f, err, gp.CodeNotFound)
	}
	if len(full.Text) > 20 {
		cut, err := p.GetDiff(ctx(), gp.DiffRequest{Target: tg, Number: f.ChangeRequest, MaxBytes: 10})
		if err != nil {
			t.Fatalf("GetDiff with MaxBytes: %v", err)
		}
		if len(cut.Text) > 10 || !cut.Truncated || !utf8.ValidString(cut.Text) {
			t.Errorf("MaxBytes=10 returned %d bytes truncated=%v; want <=10 and truncated (silent truncation hides content)", len(cut.Text), cut.Truncated)
		}
	}

	threads, err := p.ListThreads(ctx(), gp.PagedCRRequest{Target: tg, Number: f.ChangeRequest})
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	for _, th := range threads.Items {
		if th.ID == "" || len(th.Comments) == 0 {
			t.Errorf("thread needs an id and at least one comment: %+v", th)
		}
		for _, c := range th.Comments {
			if c.ID == "" || c.Author == "" || (c.AuthorKind != gp.ActorUser && c.AuthorKind != gp.ActorBot) {
				t.Errorf("incomplete thread comment: %+v", c)
			}
		}
	}
	if f.IssueNumber != "" {
		is, err := p.GetIssue(ctx(), gp.IssueRequest{Target: tg, Number: f.IssueNumber})
		if err != nil || is.Title == "" || is.URL == "" {
			t.Errorf("GetIssue: %+v %v", is, err)
		}
		_, err = p.GetIssue(ctx(), gp.IssueRequest{Target: tg, Number: f.MissingCR})
		mustCode(t, f, err, gp.CodeNotFound)
	}
	if _, err := p.ListComments(ctx(), gp.ListCommentsRequest{Target: tg, Subject: gp.SubjectRef{Kind: gp.SubjectChangeRequest, Number: f.ChangeRequest}}); err != nil {
		t.Errorf("ListComments: %v", err)
	}

	if f.HeadSHA != "" {
		checks, err := p.GetChecks(ctx(), gp.ChecksRequest{Target: tg, Ref: f.HeadSHA})
		if err != nil {
			t.Fatalf("GetChecks: %v", err)
		}
		for _, c := range checks {
			if c.ID == "" || c.Name == "" || !c.State.Valid() {
				t.Errorf("invalid check: %+v", c)
			}
		}
	}
	if f.FailingCheckID != "" {
		logs, err := p.GetCheckLogs(ctx(), gp.CheckLogsRequest{Target: tg, CheckID: f.FailingCheckID})
		if err != nil || logs.Text == "" {
			t.Errorf("GetCheckLogs: %v empty=%v", err, logs.Text == "")
		}
		tail, err := p.GetCheckLogs(ctx(), gp.CheckLogsRequest{Target: tg, CheckID: f.FailingCheckID, TailBytes: 5})
		if err != nil || len(tail.Text) > 5 || !utf8.ValidString(tail.Text) || (len(logs.Text) > 5 && !tail.Truncated) {
			t.Errorf("tail-limited logs: %q truncated=%v err=%v", tail.Text, tail.Truncated, err)
		}
		_, err = p.GetCheckLogs(ctx(), gp.CheckLogsRequest{Target: tg, CheckID: "no-such-check"})
		mustCode(t, f, err, gp.CodeNotFound)
	}
	// Reads on a repository the identity cannot see never succeed.
	if f.InvisibleRepo.FullPath != "" {
		_, err = p.GetChangeRequest(ctx(), gp.CRRequest{Target: gp.Target{Identity: f.Identity, Repo: f.InvisibleRepo}, Number: f.ChangeRequest})
		mustCode(t, f, err, gp.CodeNotFound, gp.CodeForbidden)
	}

	// Optional reads, only when declared.
	caps := p.Describe().Capabilities
	if fr, ok := p.(gp.FileReader); ok && caps.FileRead && f.File.Path != "" {
		file, err := fr.GetFile(ctx(), gp.FileRequest{Target: tg, Ref: f.File.Ref, Path: f.File.Path})
		if err != nil || len(file.Content) == 0 {
			t.Errorf("GetFile: %v", err)
		}
		_, err = fr.GetFile(ctx(), gp.FileRequest{Target: tg, Ref: f.File.Ref, Path: "no/such/file.xyz"})
		mustCode(t, f, err, gp.CodeNotFound)
		if len(file.Content) > 4 {
			cut, err := fr.GetFile(ctx(), gp.FileRequest{Target: tg, Ref: f.File.Ref, Path: f.File.Path, MaxBytes: 4})
			if err != nil || len(cut.Content) > 4 || !cut.Truncated {
				t.Errorf("GetFile MaxBytes=4: len=%d truncated=%v err=%v", len(cut.Content), cut.Truncated, err)
			}
		}
	}
	if cs, ok := p.(gp.CodeSearcher); ok && caps.CodeSearch && f.SearchQuery != "" {
		hits, err := cs.SearchCode(ctx(), gp.CodeSearchRequest{Target: tg, Query: f.SearchQuery})
		if err != nil || len(hits.Items) == 0 {
			t.Errorf("SearchCode: %v hits=%d", err, len(hits.Items))
		}
	}
}

// ── unsupported ──────────────────────────────────────────────────────────────

func testUnsupported(t *testing.T, h Harness) {
	p := h.New(t)
	caps := mustJSONMap(p.Describe().Capabilities)
	for _, op := range gp.Operations() {
		if op.Capability == "" {
			continue
		}
		declared, _ := caps[op.Capability].(bool)
		if declared {
			continue
		}
		status, body := gp.Dispatch(ctx(), p, op.Name, []byte(`{}`))
		var e gp.Envelope
		if err := json.Unmarshal(body, &e); err != nil || e.OK || e.Error == nil || e.Error.Code != gp.CodeUnsupported || status != gp.CodeUnsupported.HTTPStatus() {
			t.Errorf("%s is not declared (%s=false) but answered status %d: %s; want unsupported", op.Name, op.Capability, status, body)
		}
	}
}

func mustJSONMap(v any) map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(mustJSON(v)), &m)
	return m
}

// ── access grants ────────────────────────────────────────────────────────────

func testAccessGrants(t *testing.T, h Harness) {
	f := h.Fixtures
	p := h.New(t)
	ag, ok := p.(gp.AccessGranter)
	if !p.Describe().Capabilities.AccessGrants || !ok {
		t.Skip("provider does not declare access_grants")
	}
	begin := gp.BeginAccessRequest{Identity: f.Identity, Owner: f.AccessOwner, ReturnURL: "https://paca.test/return", CallbackURL: "https://paca.test/cb"}
	step, err := ag.BeginAccessGrant(ctx(), begin)
	if err != nil {
		t.Fatalf("BeginAccessGrant: %v", err)
	}
	switch step.Kind {
	case gp.StepRedirect, gp.StepFormPost:
		if step.URL == "" {
			t.Error("redirect/form_post steps need a URL")
		}
	default:
		t.Errorf("an access grant is a trip to the platform: step kind %q", step.Kind)
	}
	if step.State == "" {
		t.Error("the step needs a State: it is how the callback is matched to this grant")
	}
	noLeak(t, f, mustJSON(step))

	bad := begin
	bad.Identity = gp.IdentityRef{ID: "no-such-identity"}
	_, err = ag.BeginAccessGrant(ctx(), bad)
	mustCode(t, f, err, gp.CodeNotFound, gp.CodeForbidden, gp.CodeValidation)

	// A callback with no state, or a state this provider never issued, grants nothing.
	_, err = ag.CompleteAccessGrant(ctx(), gp.CompleteAccessRequest{Identity: f.Identity, Params: f.AccessGrantParams})
	mustCode(t, f, err, gp.CodeValidation, gp.CodeForbidden)
	_, err = ag.CompleteAccessGrant(ctx(), gp.CompleteAccessRequest{Identity: f.Identity, State: "never-issued", Params: f.AccessGrantParams})
	mustCode(t, f, err, gp.CodeForbidden, gp.CodeNotFound, gp.CodeValidation)

	if len(f.AccessGrantParams) == 0 {
		return
	}
	grant, err := ag.CompleteAccessGrant(ctx(), gp.CompleteAccessRequest{Identity: f.Identity, State: step.State, Params: f.AccessGrantParams})
	if err != nil {
		t.Fatalf("CompleteAccessGrant: %v", err)
	}
	noLeak(t, f, mustJSON(grant))
	// A state works once: replaying a callback must not grant again.
	_, err = ag.CompleteAccessGrant(ctx(), gp.CompleteAccessRequest{Identity: f.Identity, State: step.State, Params: f.AccessGrantParams})
	mustCode(t, f, err, gp.CodeForbidden, gp.CodeNotFound, gp.CodeValidation)
}

// ── writes ───────────────────────────────────────────────────────────────────

func testWrites(t *testing.T, h Harness) {
	f := h.Fixtures
	tg := target(f)

	t.Run("open-change-request", func(t *testing.T) {
		p := h.New(t)
		caps := p.Describe().Capabilities
		if f.NewBranchHead == "" {
			t.Skip("Fixtures.NewBranchHead not set")
		}
		cr, err := p.OpenChangeRequest(ctx(), gp.OpenCRRequest{Target: tg, Title: "Conformance", Body: "body", Head: f.NewBranchHead, Base: f.BaseBranch})
		if err != nil {
			t.Fatalf("OpenChangeRequest: %v", err)
		}
		if cr.Number == "" || cr.State != gp.CROpen || cr.Head.Ref != f.NewBranchHead || cr.URL == "" {
			t.Errorf("unexpected change request: %+v", cr)
		}
		// A second open for the same branch must not silently create another.
		_, err = p.OpenChangeRequest(ctx(), gp.OpenCRRequest{Target: tg, Title: "Again", Head: f.NewBranchHead, Base: f.BaseBranch})
		mustCode(t, f, err, gp.CodeConflict, gp.CodeValidation)
		_, err = p.OpenChangeRequest(ctx(), gp.OpenCRRequest{Target: tg, Head: f.NewBranchHead, Base: f.BaseBranch})
		mustCode(t, f, err, gp.CodeValidation)
		if caps.DraftChangeRequests {
			d, err := p.OpenChangeRequest(ctx(), gp.OpenCRRequest{Target: tg, Title: "Draft", Head: orDefault(f.DraftBranchHead, "draft-branch"), Base: f.BaseBranch, Draft: true})
			if err != nil || !d.Draft {
				t.Errorf("draft change request: %+v %v", d, err)
			}
		}
		if caps.IdempotentWrites {
			a, errA := p.OpenChangeRequest(ctx(), gp.OpenCRRequest{Target: tg, Title: "Idem", Head: orDefault(f.IdempotentBranchHead, "idem-branch"), Base: f.BaseBranch, IdempotencyKey: "k1"})
			b, errB := p.OpenChangeRequest(ctx(), gp.OpenCRRequest{Target: tg, Title: "Idem", Head: orDefault(f.IdempotentBranchHead, "idem-branch"), Base: f.BaseBranch, IdempotencyKey: "k1"})
			if errA != nil || errB != nil || a.Number != b.Number {
				t.Errorf("idempotent open: %v %v numbers %q/%q", errA, errB, a.Number, b.Number)
			}
		}
		title := "Renamed"
		up, err := p.UpdateChangeRequest(ctx(), gp.UpdateCRRequest{Target: tg, Number: cr.Number, Title: &title})
		if err != nil || up.Title != title {
			t.Errorf("UpdateChangeRequest: %+v %v", up, err)
		}
		bad := gp.CRMerged
		_, err = p.UpdateChangeRequest(ctx(), gp.UpdateCRRequest{Target: tg, Number: cr.Number, State: &bad})
		mustCode(t, f, err, gp.CodeValidation)
	})

	t.Run("comments", func(t *testing.T) {
		p := h.New(t)
		caps := p.Describe().Capabilities
		subj := gp.SubjectRef{Kind: gp.SubjectChangeRequest, Number: f.ChangeRequest}
		r, err := p.Comment(ctx(), gp.CommentRequest{Target: tg, Subject: subj, Body: "hello"})
		if err != nil || r.ID == "" {
			t.Fatalf("Comment: %+v %v (core indexes comments by id, so an id is mandatory)", r, err)
		}
		_, err = p.Comment(ctx(), gp.CommentRequest{Target: tg, Subject: subj, Body: "  "})
		mustCode(t, f, err, gp.CodeValidation)
		_, err = p.Comment(ctx(), gp.CommentRequest{Target: tg, Subject: gp.SubjectRef{Kind: gp.SubjectChangeRequest, Number: f.MissingCR}, Body: "x"})
		mustCode(t, f, err, gp.CodeNotFound)
		if caps.IdempotentWrites {
			a, _ := p.Comment(ctx(), gp.CommentRequest{Target: tg, Subject: subj, Body: "same", IdempotencyKey: "c1"})
			b, _ := p.Comment(ctx(), gp.CommentRequest{Target: tg, Subject: subj, Body: "same", IdempotencyKey: "c1"})
			if a.ID == "" || a.ID != b.ID {
				t.Errorf("idempotent comment returned %q then %q", a.ID, b.ID)
			}
		}
		if f.ThreadID != "" {
			rr, err := p.ReplyThread(ctx(), gp.ReplyThreadRequest{Target: tg, Number: f.ChangeRequest, ThreadID: f.ThreadID, Body: "done"})
			if err != nil || rr.ID == "" {
				t.Errorf("ReplyThread: %+v %v", rr, err)
			}
			_, err = p.ReplyThread(ctx(), gp.ReplyThreadRequest{Target: tg, Number: f.ChangeRequest, ThreadID: "no-such-thread", Body: "x"})
			mustCode(t, f, err, gp.CodeNotFound)
		}
	})

	t.Run("review", func(t *testing.T) {
		p := h.New(t)
		caps := p.Describe().Capabilities
		inline := []gp.InlineComment{}
		if len(f.ChangedFiles) > 0 {
			inline = append(inline, gp.InlineComment{Path: f.ChangedFiles[0], Line: 1, Side: "new", Body: "nit"})
		}
		res, err := p.SubmitReview(ctx(), gp.SubmitReviewRequest{Target: tg, Number: f.ChangeRequest, Verdict: gp.VerdictComment, Summary: "looks fine", Comments: inline})
		if err != nil {
			t.Fatalf("SubmitReview: %v", err)
		}
		if res.Posted != len(inline) {
			t.Errorf("Posted = %d, want %d", res.Posted, len(inline))
		}
		if res.Emulated == caps.BatchReview {
			t.Errorf("Emulated=%v contradicts BatchReview=%v: a provider without batch review must emulate and report it", res.Emulated, caps.BatchReview)
		}
		_, err = p.SubmitReview(ctx(), gp.SubmitReviewRequest{Target: tg, Number: f.ChangeRequest, Verdict: "shrug"})
		mustCode(t, f, err, gp.CodeValidation)
		_, err = p.SubmitReview(ctx(), gp.SubmitReviewRequest{Target: tg, Number: f.ChangeRequest, Verdict: gp.VerdictComment,
			Comments: []gp.InlineComment{{Path: "not/in/the/diff.go", Line: 1, Body: "x"}}})
		mustCode(t, f, err, gp.CodeValidation)
		_, err = p.SubmitReview(ctx(), gp.SubmitReviewRequest{Target: tg, Number: f.MissingCR, Verdict: gp.VerdictComment, Summary: "x"})
		mustCode(t, f, err, gp.CodeNotFound)
	})

	t.Run("status", func(t *testing.T) {
		p := h.New(t)
		subj := gp.SubjectRef{Kind: gp.SubjectChangeRequest, Number: f.ChangeRequest}
		st, err := p.SetStatus(ctx(), gp.SetStatusRequest{Target: tg, Subject: subj, SHA: f.HeadSHA, State: gp.RunQueued, Title: "Paca / dev"})
		if err != nil || st.StatusRef == "" {
			t.Fatalf("SetStatus: %+v %v", st, err)
		}
		upd, err := p.SetStatus(ctx(), gp.SetStatusRequest{Target: tg, Subject: subj, SHA: f.HeadSHA, State: gp.RunSuccess, Title: "Paca / dev", Summary: "done", StatusRef: st.StatusRef})
		if err != nil || upd.StatusRef == "" {
			t.Errorf("updating a status by its reference: %+v %v", upd, err)
		}
	})
}

func testOptionalWrites(t *testing.T, h Harness) {
	f := h.Fixtures
	tg := target(f)
	probe := h.New(t)
	caps := probe.Describe().Capabilities
	subj := gp.SubjectRef{Kind: gp.SubjectChangeRequest, Number: f.ChangeRequest}

	if caps.ResolveThreads && f.ThreadID != "" {
		t.Run("resolve-thread", func(t *testing.T) {
			p := h.New(t)
			tr := p.(gp.ThreadResolver)
			if _, err := tr.ResolveThread(ctx(), gp.ResolveThreadRequest{Target: tg, Number: f.ChangeRequest, ThreadID: f.ThreadID, Resolved: true}); err != nil {
				t.Fatalf("ResolveThread: %v", err)
			}
			page, err := p.ListThreads(ctx(), gp.PagedCRRequest{Target: tg, Number: f.ChangeRequest})
			if err != nil {
				t.Fatal(err)
			}
			resolved := false
			for _, th := range page.Items {
				if th.ID == f.ThreadID {
					resolved = th.Resolved
				}
			}
			if !resolved {
				t.Error("thread is not reported resolved after ResolveThread")
			}
			_, err = tr.ResolveThread(ctx(), gp.ResolveThreadRequest{Target: tg, Number: f.ChangeRequest, ThreadID: "no-such-thread", Resolved: true})
			mustCode(t, f, err, gp.CodeNotFound)
		})
	}
	if caps.Branches {
		t.Run("branches", func(t *testing.T) {
			p := h.New(t)
			bm := p.(gp.BranchManager)
			def := orDefault(tg.Repo.DefaultBranch, f.BaseBranch)
			name := orDefault(f.NewBranchName, "paca/conformance")
			base, err := bm.GetBranch(ctx(), gp.GetBranchRequest{Target: tg, Name: def})
			if err != nil || base.Name != def || base.SHA == "" {
				t.Fatalf("GetBranch(default): %+v %v", base, err)
			}
			_, err = bm.GetBranch(ctx(), gp.GetBranchRequest{Target: tg, Name: "no-such-branch"})
			mustCode(t, f, err, gp.CodeNotFound)
			_, err = bm.GetBranch(ctx(), gp.GetBranchRequest{Target: tg})
			mustCode(t, f, err, gp.CodeValidation)

			made, err := bm.CreateBranch(ctx(), gp.CreateBranchRequest{Target: tg, Name: name})
			if err != nil || made.Name != name || made.SHA != base.SHA {
				t.Fatalf("CreateBranch from the default branch: %+v %v (want sha %s)", made, err, base.SHA)
			}
			got, err := bm.GetBranch(ctx(), gp.GetBranchRequest{Target: tg, Name: name})
			if err != nil || got.SHA != made.SHA {
				t.Errorf("created branch not visible: %+v %v", got, err)
			}
			_, err = bm.CreateBranch(ctx(), gp.CreateBranchRequest{Target: tg, Name: name})
			mustCode(t, f, err, gp.CodeConflict)
			_, err = bm.CreateBranch(ctx(), gp.CreateBranchRequest{Target: tg, Name: name + "-from-missing", From: "no-such-branch"})
			mustCode(t, f, err, gp.CodeNotFound)
			for _, bad := range []string{"bad name~^", "-leading-dash", "a..b", "trailing."} {
				_, err = bm.CreateBranch(ctx(), gp.CreateBranchRequest{Target: tg, Name: bad})
				mustCode(t, f, err, gp.CodeValidation)
			}
			if caps.IdempotentWrites {
				a, errA := bm.CreateBranch(ctx(), gp.CreateBranchRequest{Target: tg, Name: name + "-idem", IdempotencyKey: "b1"})
				b, errB := bm.CreateBranch(ctx(), gp.CreateBranchRequest{Target: tg, Name: name + "-idem", IdempotencyKey: "b1"})
				if errA != nil || errB != nil || a != b {
					t.Errorf("idempotent branch create: %+v %v / %+v %v", a, errA, b, errB)
				}
			}
		})
	}
	if caps.Labels {
		t.Run("labels", func(t *testing.T) {
			p := h.New(t)
			if _, err := p.(gp.LabelManager).AddLabels(ctx(), gp.LabelsRequest{Target: tg, Subject: subj, Labels: []string{"paca"}}); err != nil {
				t.Errorf("AddLabels: %v", err)
			}
			cr, err := p.GetChangeRequest(ctx(), gp.CRRequest{Target: tg, Number: f.ChangeRequest})
			if err != nil || !contains(cr.Labels, "paca") {
				t.Errorf("label not visible afterwards: %v %v", cr.Labels, err)
			}
		})
	}
	if caps.ReviewerRequests && f.Reviewer != "" {
		t.Run("reviewers", func(t *testing.T) {
			p := h.New(t)
			if _, err := p.(gp.ReviewerRequester).RequestReviewers(ctx(), gp.PeopleRequest{Target: tg, Subject: subj, Logins: []string{f.Reviewer}}); err != nil {
				t.Errorf("RequestReviewers: %v", err)
			}
		})
	}
	if caps.Assignees && f.IssueNumber != "" {
		t.Run("assignees", func(t *testing.T) {
			p := h.New(t)
			if _, err := p.(gp.AssigneeSetter).SetAssignees(ctx(), gp.PeopleRequest{Target: tg, Subject: gp.SubjectRef{Kind: gp.SubjectIssue, Number: f.IssueNumber}, Logins: []string{f.Identity.Handle}}); err != nil {
				t.Errorf("SetAssignees: %v", err)
			}
		})
	}
	if caps.Merge {
		t.Run("merge", func(t *testing.T) {
			p := h.New(t)
			m := p.(gp.Merger)
			cr, err := p.GetChangeRequest(ctx(), gp.CRRequest{Target: tg, Number: f.ChangeRequest})
			if err != nil {
				t.Fatal(err)
			}
			// The expected head is mandatory and a mismatch must refuse.
			_, err = m.MergeChangeRequest(ctx(), gp.MergeRequest{Target: tg, Number: f.ChangeRequest})
			mustCode(t, f, err, gp.CodeValidation)
			_, err = m.MergeChangeRequest(ctx(), gp.MergeRequest{Target: tg, Number: f.ChangeRequest, ExpectedHeadSHA: "stale-sha"})
			mustCode(t, f, err, gp.CodeConflict)
			if _, err := m.MergeChangeRequest(ctx(), gp.MergeRequest{Target: tg, Number: f.ChangeRequest, ExpectedHeadSHA: cr.Head.SHA, Method: "squash"}); err != nil {
				t.Fatalf("merge with the right head: %v", err)
			}
			after, _ := p.GetChangeRequest(ctx(), gp.CRRequest{Target: tg, Number: f.ChangeRequest})
			if after.State != gp.CRMerged {
				t.Errorf("state after merge = %q", after.State)
			}
		})
	}
	if caps.RerunChecks && f.HeadSHA != "" {
		t.Run("rerun-checks", func(t *testing.T) {
			p := h.New(t)
			if _, err := p.(gp.CheckRerunner).RerunChecks(ctx(), gp.RerunRequest{Target: tg, Ref: f.HeadSHA}); err != nil {
				t.Errorf("RerunChecks: %v", err)
			}
		})
	}
	if caps.Reactions {
		t.Run("react", func(t *testing.T) {
			p := h.New(t)
			r := p.(gp.Reactor)
			if _, err := r.React(ctx(), gp.ReactRequest{Target: tg, CommentID: "c-1", Emoji: "eyes"}); err != nil {
				t.Errorf("React: %v", err)
			}
			_, err := r.React(ctx(), gp.ReactRequest{Target: tg, CommentID: "c-1", Emoji: "not-an-emoji"})
			mustCode(t, f, err, gp.CodeValidation)
		})
	}
	if caps.WebhookManagement {
		t.Run("webhook-management", func(t *testing.T) {
			p := h.New(t)
			w := p.(gp.WebhookManager)
			req := gp.EnsureWebhookRequest{Target: tg, URL: "https://paca.test/hook"}
			a, err := w.EnsureWebhook(ctx(), req)
			if err != nil || a.ID == "" {
				t.Fatalf("EnsureWebhook: %+v %v", a, err)
			}
			b, err := w.EnsureWebhook(ctx(), req)
			if err != nil || b.ID != a.ID {
				t.Errorf("EnsureWebhook must be idempotent for the same URL: %q then %q (%v)", a.ID, b.ID, err)
			}
			if _, err := w.RemoveWebhook(ctx(), gp.RemoveWebhookRequest{Target: tg, Webhook: a}); err != nil {
				t.Errorf("RemoveWebhook: %v", err)
			}
			if _, err := w.RemoveWebhook(ctx(), gp.RemoveWebhookRequest{Target: tg, Webhook: a}); err != nil {
				t.Errorf("removing an already-removed webhook must not fail: %v", err)
			}
		})
	}
	if caps.IdentityDeletion {
		t.Run("delete-identity", func(t *testing.T) {
			p := h.New(t)
			if _, err := p.(gp.IdentityDeleter).DeleteIdentity(ctx(), gp.DeleteIdentityRequest{Identity: f.Identity}); err != nil {
				t.Errorf("DeleteIdentity: %v", err)
			}
		})
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
