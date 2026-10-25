// Package fakeprovider is the reference implementation of the Git Provider SPI:
// a complete, in-memory "platform" with deterministic seed data. It exists to
// (1) show provider authors what correct behaviour looks like, (2) give core and
// plugin tests a provider that needs no network, and (3) prove that the
// conformance suite can be satisfied by both a full and a minimal provider.
//
// Two types are provided: [Full] implements every optional interface and
// declares every capability; [Minimal] implements only the required surface and
// declares an honest, smaller capability set (static credentials, repo-scoped
// webhooks, comment-only status, emulated batch review).
//
// The fake's webhook format is deliberately simple: a JSON body plus
// `x-fake-signature: sha256=<hex hmac of the body>`.
package fakeprovider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
)

// Seed data, exported so fixtures and tests can refer to it.
const (
	Host           = "fake.test"
	IdentityID     = "ident-1"
	IdentityHandle = "paca-bot"
	WebhookSecret  = "whsec_fake_secret_value"
	StaticSecret   = "static_fake_secret_value"
	RepoPath       = "acme/web"
	SeedCRNumber   = "1"
	SeedHeadSHA    = "abc123"
)

var allRepoPaths = []string{"acme/web", "acme/api", "tools/docs"}

type cr struct {
	gp.ChangeRequest
	files   []gp.ChangedFile
	threads []*gp.Thread
	merged  bool
}

type repoState struct {
	ref       gp.RepoRef
	private   bool
	crs       map[string]*cr
	issues    map[string]*gp.Issue
	comments  map[string][]gp.Comment // subject key → comments
	perms     map[string]gp.Permission
	files     map[string][]byte // path → content (any ref)
	protected map[string]bool
	checks    map[string][]gp.Check // sha → checks
	reactions []string
	hooks     map[string]string
	nextID    int
	statuses  map[string]gp.SetStatusRequest
}

// Core implements the required [gp.Provider] surface against an in-memory world.
type Core struct {
	mu     sync.Mutex
	caps   gp.Capabilities
	repos  map[string]*repoState
	idem   map[string]json.RawMessage
	seq    int
	grants map[string]bool // access-grant states → used (Full only)
	// Deleted records identities removed through DeleteIdentity (Full only).
	Deleted []string
	// Now is the clock; tests may override.
	Now func() time.Time
}

func newCore(caps gp.Capabilities) *Core {
	c := &Core{caps: caps, repos: map[string]*repoState{}, idem: map[string]json.RawMessage{}, Now: time.Now}
	for i, path := range allRepoPaths {
		c.repos[path] = c.seedRepo(path, i == 2)
	}
	return c
}

func (c *Core) seedRepo(path string, private bool) *repoState {
	r := &repoState{
		ref: gp.RepoRef{Host: Host, FullPath: path, ExternalID: "ext-" + strings.ReplaceAll(path, "/", "-"),
			DefaultBranch: "main", CloneURL: "https://" + Host + "/" + path + ".git"},
		private:   private,
		crs:       map[string]*cr{},
		issues:    map[string]*gp.Issue{},
		comments:  map[string][]gp.Comment{},
		perms:     map[string]gp.Permission{"octocat": gp.PermissionWrite, "reader": gp.PermissionRead, "owner": gp.PermissionAdmin, IdentityHandle: gp.PermissionWrite},
		files:     map[string][]byte{"README.md": []byte("# " + path + "\n"), "src/main.go": []byte("package main\n\nfunc main() {}\n")},
		protected: map[string]bool{"main": true},
		checks:    map[string][]gp.Check{SeedHeadSHA: {{ID: "chk-1", Name: "ci/test", State: gp.CheckFailure, Required: true, URL: "https://" + Host + "/chk-1"}, {ID: "chk-2", Name: "ci/lint", State: gp.CheckSuccess}}},
		hooks:     map[string]string{},
		statuses:  map[string]gp.SetStatusRequest{},
	}
	r.crs[SeedCRNumber] = &cr{
		ChangeRequest: gp.ChangeRequest{Number: SeedCRNumber, Title: "Add feature", Body: "Implements the feature.", State: gp.CROpen,
			Head: gp.Branch{Ref: "feat/x", SHA: SeedHeadSHA, Repo: path}, Base: gp.Branch{Ref: "main", SHA: "base000", Repo: path},
			Author: "octocat", URL: "https://" + Host + "/" + path + "/pulls/1", Labels: []string{"enhancement"}},
		files: []gp.ChangedFile{
			{Path: "src/main.go", Status: "modified", Additions: 3, Deletions: 1},
			{Path: "src/util.go", Status: "added", Additions: 10},
			{Path: "docs/guide.md", Status: "added", Additions: 5},
		},
		threads: []*gp.Thread{{ID: "thr-1", Path: "src/main.go", Line: 3, Comments: []gp.Comment{
			{ID: "c-1", Author: "reader", AuthorKind: gp.ActorUser, Body: "Why not use a constant here?"}}}},
	}
	r.issues["7"] = &gp.Issue{Number: "7", Title: "A bug", Body: "It crashes.", State: "open", Author: "octocat", URL: "https://" + Host + "/" + path + "/issues/7"}
	return r
}

func (c *Core) repo(t gp.Target) (*repoState, error) {
	r, ok := c.repos[t.Repo.FullPath]
	if !ok || t.Identity.ID != IdentityID {
		return nil, gp.Errorf(gp.CodeNotFound, "repository not found")
	}
	return r, nil
}

func (c *Core) next(prefix string) string { c.seq++; return fmt.Sprintf("%s-%d", prefix, c.seq) }

// idemKey namespaces a caller-supplied idempotency key; no key, no memoisation.
func idemKey(op, key string) string {
	if key == "" {
		return ""
	}
	return op + ":" + key
}

// once runs fn at most once per idempotency key when the provider declares
// IdempotentWrites; otherwise it always runs.
func once[T any](c *Core, key string, fn func() (T, error)) (T, error) {
	if key == "" || !c.caps.IdempotentWrites {
		return fn()
	}
	if raw, ok := c.idem[key]; ok {
		var v T
		_ = json.Unmarshal(raw, &v)
		return v, nil
	}
	v, err := fn()
	if err == nil {
		raw, _ := json.Marshal(v)
		c.idem[key] = raw
	}
	return v, err
}

// Describe implements gp.Provider.
func (c *Core) Describe() gp.Info {
	return gp.Info{SPIVersion: gp.SPIVersion, ID: "com.paca.fake", DisplayName: "Fake Git", Icon: "git",
		Hosts: []string{Host}, SelfHostedHosts: true,
		Terms:        gp.Terms{ChangeRequest: "Change request", ChangeRequestPlural: "Change requests", Repository: "Repository"},
		Capabilities: c.caps}
}

// BeginIdentitySetup implements gp.Provider.
func (c *Core) BeginIdentitySetup(_ context.Context, req gp.BeginSetupRequest) (gp.SetupStep, error) {
	if req.Host != Host {
		return gp.SetupStep{}, gp.Errorf(gp.CodeValidation, "unknown host %q", req.Host)
	}
	if req.Scope == gp.SetupAgent && !c.caps.MultipleIdentities {
		return gp.SetupStep{}, gp.Errorf(gp.CodeUnsupported, "per-agent identities are not supported")
	}
	return gp.SetupStep{Kind: gp.StepTokenForm, Prompt: "Paste a token with repo access", State: "setup-" + string(req.Scope),
		Form: []gp.FormField{{Name: "token", Label: "Access token", Help: "Needs repository read/write access", Secret: true, Required: true}}}, nil
}

// CompleteIdentitySetup implements gp.Provider.
func (c *Core) CompleteIdentitySetup(_ context.Context, req gp.CompleteSetupRequest) (gp.Identity, error) {
	if req.State == "" || req.Params["token"] == "" {
		return gp.Identity{}, gp.Errorf(gp.CodeValidation, "state and token are required")
	}
	return gp.Identity{Ref: gp.IdentityRef{ID: IdentityID, Handle: IdentityHandle}, Host: Host, Kind: gp.IdentityBotUser, DisplayName: "Paca Bot"}, nil
}

// ListRepositories implements gp.Provider.
func (c *Core) ListRepositories(_ context.Context, req gp.ListReposRequest) (gp.RepositoryPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if req.Identity.ID != IdentityID {
		return gp.RepositoryPage{}, gp.Errorf(gp.CodeForbidden, "unknown identity")
	}
	var paths []string
	for p := range c.repos {
		if req.Query == "" || strings.Contains(p, req.Query) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	start, _ := strconv.Atoi(req.Cursor)
	size := req.PageSize
	if size <= 0 {
		size = 50
	}
	end := start + size
	next := ""
	if end < len(paths) {
		next = strconv.Itoa(end)
	} else {
		end = len(paths)
	}
	var page gp.RepositoryPage
	if start < len(paths) {
		for _, p := range paths[start:end] {
			r := c.repos[p]
			page.Items = append(page.Items, gp.Repository{Ref: r.ref, Private: r.private})
		}
	}
	page.NextCursor = next
	return page, nil
}

var urlPath = regexp.MustCompile(`^(?:https?://[^/]+/)?([^?#]+?)(?:\.git)?/?$`)

// ResolveRepository implements gp.Provider.
func (c *Core) ResolveRepository(_ context.Context, req gp.ResolveRepoRequest) (gp.ResolvedRepository, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := urlPath.FindStringSubmatch(strings.TrimSpace(req.PathOrURL))
	if m == nil {
		return gp.ResolvedRepository{}, gp.Errorf(gp.CodeValidation, "not a repository path or URL")
	}
	r, ok := c.repos[m[1]]
	if !ok {
		return gp.ResolvedRepository{}, gp.Errorf(gp.CodeNotFound, "repository not found")
	}
	for _, cand := range req.Candidates {
		if cand.ID == IdentityID {
			return gp.ResolvedRepository{Repository: gp.Repository{Ref: r.ref, Private: r.private}, Identity: cand}, nil
		}
	}
	return gp.ResolvedRepository{}, gp.Errorf(gp.CodeForbidden, "no candidate identity can see this repository")
}

// ── webhooks ─────────────────────────────────────────────────────────────────

type fakeHook struct {
	Delivery  string `json:"delivery"`
	Type      string `json:"type"`
	Action    string `json:"action"`
	Repo      string `json:"repo"`
	Subject   string `json:"subject"`
	Number    string `json:"number"`
	Actor     string `json:"actor"`
	ActorKind string `json:"actor_kind"`
	Text      string `json:"text"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Thread    string `json:"thread"`
	Comment   string `json:"comment"`
	HeadRef   string `json:"head_ref"`
	HeadSHA   string `json:"head_sha"`
	Base      string `json:"base"`
	Draft     bool   `json:"draft"`
	Label     string `json:"label"`
	Check     string `json:"check"`
	Created   bool   `json:"created"`
	Deleted   bool   `json:"deleted"`
	Result    string `json:"conclusion"`
	Requested string `json:"requested"`
	Assignee  string `json:"assignee"`
}

// Sign returns the signature header value for a webhook body.
func Sign(body []byte) string {
	m := hmac.New(sha256.New, []byte(WebhookSecret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func mentions(text, handle string) bool {
	re := regexp.MustCompile(`(?i)(^|[^\w@])@` + regexp.QuoteMeta(handle) + `\b`)
	return re.MatchString(text)
}

// VerifyAndParseWebhook implements gp.Provider.
func (c *Core) VerifyAndParseWebhook(_ context.Context, req gp.WebhookRequest) ([]gp.Event, error) {
	sig := req.Headers["x-fake-signature"]
	if !hmac.Equal([]byte(sig), []byte(Sign(req.Body))) {
		return nil, gp.Errorf(gp.CodeInvalidSignature, "signature mismatch")
	}
	var h fakeHook
	if err := json.Unmarshal(req.Body, &h); err != nil {
		return nil, gp.Errorf(gp.CodeValidation, "malformed payload")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.repos[h.Repo]
	if !ok {
		return nil, nil // not a repository this identity can see: nothing to report
	}
	kind := map[string]gp.EventKind{"comment": gp.EventComment, "change_request": gp.EventChangeRequest, "review": gp.EventReview,
		"check": gp.EventCheck, "push": gp.EventPush, "label": gp.EventLabel, "assignment": gp.EventAssignment, "review_request": gp.EventReviewRequest}[h.Type]
	if kind == "" {
		return nil, nil // unmapped event types are dropped, not guessed
	}
	actorKind := gp.ActorUser
	if h.ActorKind == "bot" {
		actorKind = gp.ActorBot
	}
	ev := gp.Event{ID: h.Delivery + ":" + h.Type, Kind: kind, Action: h.Action, Repo: r.ref,
		Actor: gp.Actor{Login: h.Actor, Kind: actorKind, Permission: r.perms[h.Actor]}, Text: h.Text, Label: h.Label, RawRef: h.Delivery}
	if h.Number != "" {
		sk := gp.SubjectKind(h.Subject)
		if sk == "" {
			sk = gp.SubjectChangeRequest
		}
		sub := &gp.EventSubject{Kind: sk, Number: h.Number, Base: h.Base, Draft: h.Draft}
		if h.HeadRef != "" {
			sub.Head = &gp.Branch{Ref: h.HeadRef, SHA: h.HeadSHA, Repo: h.Repo}
		}
		ev.Subject = sub
	}
	if h.Path != "" || h.Thread != "" || h.Comment != "" {
		ev.Location = &gp.Location{Path: h.Path, Line: h.Line, ThreadID: h.Thread, CommentID: h.Comment}
	}
	if kind == gp.EventPush {
		ev.Push = &gp.PushInfo{Ref: h.HeadRef, SHA: h.HeadSHA, Created: h.Created, Deleted: h.Deleted}
	}
	if kind == gp.EventCheck {
		ev.Check = &gp.CheckInfo{Name: h.Check, Conclusion: h.Result, SHA: h.HeadSHA}
	}
	// Only report ways of being addressed that this provider declares: core
	// trusts `addressed` and capabilities to agree.
	if c.caps.SupportsTrigger(gp.TriggerMention) && mentions(h.Text, IdentityHandle) {
		ev.Addressed = append(ev.Addressed, gp.Addressed{IdentityID: IdentityID, Via: gp.ViaMention})
	}
	if c.caps.SupportsTrigger(gp.TriggerReviewRequest) && h.Requested == IdentityHandle {
		ev.Addressed = append(ev.Addressed, gp.Addressed{IdentityID: IdentityID, Via: gp.ViaReviewRequest})
	}
	if c.caps.SupportsTrigger(gp.TriggerAssignment) && h.Assignee == IdentityHandle {
		ev.Addressed = append(ev.Addressed, gp.Addressed{IdentityID: IdentityID, Via: gp.ViaAssignment})
	}
	return []gp.Event{ev}, nil
}

// ── credentials and policy ───────────────────────────────────────────────────

// MintCredential implements gp.Provider.
func (c *Core) MintCredential(_ context.Context, req gp.MintRequest) (gp.Credential, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.repo(req.Target); err != nil {
		return gp.Credential{}, err
	}
	if req.Access != gp.AccessRead && req.Access != gp.AccessWrite {
		return gp.Credential{}, gp.Errorf(gp.CodeValidation, "access must be read or write")
	}
	if c.caps.CredentialModel == gp.CredentialStatic {
		return gp.Credential{Username: "oauth2", Secret: StaticSecret}, nil
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 || ttl > time.Hour {
		ttl = 10 * time.Minute
	}
	return gp.Credential{Username: "x-access-token", Secret: c.next("tok-" + string(req.Access)), ExpiresAt: c.Now().Add(ttl)}, nil
}

// GetActorPermission implements gp.Provider.
func (c *Core) GetActorPermission(_ context.Context, req gp.ActorPermissionRequest) (gp.Permission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return gp.PermissionNone, err
	}
	if p, ok := r.perms[req.Login]; ok {
		return p, nil
	}
	return gp.PermissionNone, nil
}

// GetBranchProtection implements gp.Provider.
func (c *Core) GetBranchProtection(_ context.Context, req gp.BranchProtectionRequest) (gp.BranchProtection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return gp.BranchProtection{}, err
	}
	return gp.BranchProtection{Protected: r.protected[req.Branch], RequiresReview: r.protected[req.Branch]}, nil
}

// ── reads ────────────────────────────────────────────────────────────────────

func (c *Core) cr(t gp.Target, n string) (*repoState, *cr, error) {
	r, err := c.repo(t)
	if err != nil {
		return nil, nil, err
	}
	x, ok := r.crs[n]
	if !ok {
		return nil, nil, gp.Errorf(gp.CodeNotFound, "change request not found")
	}
	return r, x, nil
}

// GetChangeRequest implements gp.Provider.
func (c *Core) GetChangeRequest(_ context.Context, req gp.CRRequest) (gp.ChangeRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, x, err := c.cr(req.Target, req.Number)
	if err != nil {
		return gp.ChangeRequest{}, err
	}
	return x.ChangeRequest, nil
}

// ListChangedFiles implements gp.Provider.
func (c *Core) ListChangedFiles(_ context.Context, req gp.PagedCRRequest) (gp.ChangedFilePage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, x, err := c.cr(req.Target, req.Number)
	if err != nil {
		return gp.ChangedFilePage{}, err
	}
	start, _ := strconv.Atoi(req.Cursor)
	const size = 2
	end := start + size
	page := gp.ChangedFilePage{}
	if end < len(x.files) {
		page.NextCursor = strconv.Itoa(end)
	} else {
		end = len(x.files)
	}
	if start < len(x.files) {
		page.Items = append(page.Items, x.files[start:end]...)
	}
	return page, nil
}

// GetDiff implements gp.Provider.
func (c *Core) GetDiff(_ context.Context, req gp.DiffRequest) (gp.Diff, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, x, err := c.cr(req.Target, req.Number)
	if err != nil {
		return gp.Diff{}, err
	}
	var b strings.Builder
	found := req.Path == ""
	for _, f := range x.files {
		if req.Path != "" && f.Path != req.Path {
			continue
		}
		found = true
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1,1 +1,%d @@\n+changed %s\n", f.Path, f.Path, f.Path, f.Path, f.Additions, f.Path)
	}
	if !found {
		return gp.Diff{}, gp.Errorf(gp.CodeNotFound, "file is not part of this change request")
	}
	out := b.String()
	if req.MaxBytes > 0 && len(out) > req.MaxBytes {
		return gp.Diff{Text: out[:req.MaxBytes], Truncated: true}, nil
	}
	return gp.Diff{Text: out}, nil
}

// ListThreads implements gp.Provider.
func (c *Core) ListThreads(_ context.Context, req gp.PagedCRRequest) (gp.ThreadPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, x, err := c.cr(req.Target, req.Number)
	if err != nil {
		return gp.ThreadPage{}, err
	}
	page := gp.ThreadPage{}
	for _, t := range x.threads {
		page.Items = append(page.Items, *t)
	}
	return page, nil
}

func subjectKey(s gp.SubjectRef) string { return string(s.Kind) + "#" + s.Number }

// ListComments implements gp.Provider.
func (c *Core) ListComments(_ context.Context, req gp.ListCommentsRequest) (gp.CommentPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return gp.CommentPage{}, err
	}
	return gp.CommentPage{Items: append([]gp.Comment(nil), r.comments[subjectKey(req.Subject)]...)}, nil
}

// GetIssue implements gp.Provider.
func (c *Core) GetIssue(_ context.Context, req gp.IssueRequest) (gp.Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return gp.Issue{}, err
	}
	i, ok := r.issues[req.Number]
	if !ok {
		return gp.Issue{}, gp.Errorf(gp.CodeNotFound, "issue not found")
	}
	return *i, nil
}

// GetChecks implements gp.Provider.
func (c *Core) GetChecks(_ context.Context, req gp.ChecksRequest) ([]gp.Check, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return nil, err
	}
	return append([]gp.Check(nil), r.checks[req.Ref]...), nil
}

// GetCheckLogs implements gp.Provider.
func (c *Core) GetCheckLogs(_ context.Context, req gp.CheckLogsRequest) (gp.Logs, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return gp.Logs{}, err
	}
	for _, list := range r.checks {
		for _, ch := range list {
			if ch.ID == req.CheckID {
				log := "step 1 ok\nstep 2 FAILED: expected 1, got 2\n"
				if req.TailBytes > 0 && len(log) > req.TailBytes {
					return gp.Logs{Text: log[len(log)-req.TailBytes:], Truncated: true}, nil
				}
				return gp.Logs{Text: log}, nil
			}
		}
	}
	return gp.Logs{}, gp.Errorf(gp.CodeNotFound, "check not found")
}

// ── writes ───────────────────────────────────────────────────────────────────

// OpenChangeRequest implements gp.Provider.
func (c *Core) OpenChangeRequest(_ context.Context, req gp.OpenCRRequest) (gp.ChangeRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return gp.ChangeRequest{}, err
	}
	if strings.TrimSpace(req.Title) == "" || req.Head == "" || req.Base == "" {
		return gp.ChangeRequest{}, gp.Errorf(gp.CodeValidation, "title, head and base are required")
	}
	if req.Draft && !c.caps.DraftChangeRequests {
		return gp.ChangeRequest{}, gp.Errorf(gp.CodeValidation, "draft change requests are not supported")
	}
	return once(c, idemKey("open", req.IdempotencyKey), func() (gp.ChangeRequest, error) {
		for _, existing := range r.crs {
			if existing.Head.Ref == req.Head && existing.State == gp.CROpen {
				return gp.ChangeRequest{}, gp.Errorf(gp.CodeConflict, "an open change request already exists for this branch")
			}
		}
		r.nextID++
		n := strconv.Itoa(100 + r.nextID)
		x := &cr{ChangeRequest: gp.ChangeRequest{Number: n, Title: req.Title, Body: req.Body, State: gp.CROpen, Draft: req.Draft,
			Head: gp.Branch{Ref: req.Head, SHA: "sha-" + n, Repo: req.Target.Repo.FullPath}, Base: gp.Branch{Ref: req.Base},
			Author: IdentityHandle, URL: "https://" + Host + "/" + req.Target.Repo.FullPath + "/pulls/" + n,
			Labels: req.Labels, Reviewers: req.Reviewers}}
		r.crs[n] = x
		return x.ChangeRequest, nil
	})
}

// UpdateChangeRequest implements gp.Provider.
func (c *Core) UpdateChangeRequest(_ context.Context, req gp.UpdateCRRequest) (gp.ChangeRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, x, err := c.cr(req.Target, req.Number)
	if err != nil {
		return gp.ChangeRequest{}, err
	}
	if req.Title != nil {
		x.Title = *req.Title
	}
	if req.Body != nil {
		x.Body = *req.Body
	}
	if req.Draft != nil {
		x.Draft = *req.Draft
	}
	if req.State != nil {
		if *req.State != gp.CROpen && *req.State != gp.CRClosed {
			return gp.ChangeRequest{}, gp.Errorf(gp.CodeValidation, "state must be open or closed")
		}
		x.State = *req.State
	}
	return x.ChangeRequest, nil
}

// Comment implements gp.Provider.
func (c *Core) Comment(_ context.Context, req gp.CommentRequest) (gp.CommentResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return gp.CommentResult{}, err
	}
	if strings.TrimSpace(req.Body) == "" {
		return gp.CommentResult{}, gp.Errorf(gp.CodeValidation, "comment body is required")
	}
	if req.Subject.Kind == gp.SubjectChangeRequest {
		if _, ok := r.crs[req.Subject.Number]; !ok {
			return gp.CommentResult{}, gp.Errorf(gp.CodeNotFound, "change request not found")
		}
	} else if _, ok := r.issues[req.Subject.Number]; !ok {
		return gp.CommentResult{}, gp.Errorf(gp.CodeNotFound, "issue not found")
	}
	return once(c, idemKey("comment", req.IdempotencyKey), func() (gp.CommentResult, error) {
		id := c.next("cm")
		r.comments[subjectKey(req.Subject)] = append(r.comments[subjectKey(req.Subject)],
			gp.Comment{ID: id, Author: IdentityHandle, AuthorKind: gp.ActorBot, Body: req.Body})
		return gp.CommentResult{ID: id, URL: "https://" + Host + "/c/" + id}, nil
	})
}

// ReplyThread implements gp.Provider.
func (c *Core) ReplyThread(_ context.Context, req gp.ReplyThreadRequest) (gp.CommentResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, x, err := c.cr(req.Target, req.Number)
	if err != nil {
		return gp.CommentResult{}, err
	}
	for _, t := range x.threads {
		if t.ID == req.ThreadID {
			return once(c, idemKey("reply", req.IdempotencyKey), func() (gp.CommentResult, error) {
				id := c.next("cm")
				t.Comments = append(t.Comments, gp.Comment{ID: id, Author: IdentityHandle, AuthorKind: gp.ActorBot, Body: req.Body})
				return gp.CommentResult{ID: id}, nil
			})
		}
	}
	return gp.CommentResult{}, gp.Errorf(gp.CodeNotFound, "thread not found")
}

// SubmitReview implements gp.Provider. With BatchReview=false the review is
// emulated: inline comments are posted one by one, then the summary.
func (c *Core) SubmitReview(_ context.Context, req gp.SubmitReviewRequest) (gp.ReviewResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, x, err := c.cr(req.Target, req.Number)
	if err != nil {
		return gp.ReviewResult{}, err
	}
	switch req.Verdict {
	case gp.VerdictComment, gp.VerdictApprove, gp.VerdictRequestChanges:
	default:
		return gp.ReviewResult{}, gp.Errorf(gp.CodeValidation, "unknown verdict %q", req.Verdict)
	}
	if req.Verdict == gp.VerdictApprove && x.Author == IdentityHandle {
		return gp.ReviewResult{}, gp.Errorf(gp.CodeConflict, "an author cannot approve their own change request")
	}
	if req.Verdict == gp.VerdictRequestChanges && strings.TrimSpace(req.Summary) == "" && len(req.Comments) == 0 {
		return gp.ReviewResult{}, gp.Errorf(gp.CodeValidation, "requesting changes needs a summary or comments")
	}
	known := map[string]bool{}
	for _, f := range x.files {
		known[f.Path] = true
	}
	for _, ic := range req.Comments {
		if !known[ic.Path] || ic.Line <= 0 {
			return gp.ReviewResult{}, gp.Errorf(gp.CodeValidation, "inline comment on %s:%d is not on the diff", ic.Path, ic.Line)
		}
	}
	return once(c, idemKey("review", req.IdempotencyKey), func() (gp.ReviewResult, error) {
		id := c.next("rv")
		return gp.ReviewResult{ID: id, URL: "https://" + Host + "/r/" + id, Posted: len(req.Comments), Emulated: !c.caps.BatchReview}, nil
	})
}

// SetStatus implements gp.Provider.
func (c *Core) SetStatus(_ context.Context, req gp.SetStatusRequest) (gp.StatusResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.repo(req.Target)
	if err != nil {
		return gp.StatusResult{}, err
	}
	ref := req.StatusRef
	if ref == "" {
		ref = c.next("st")
	} else if _, ok := r.statuses[ref]; !ok {
		return gp.StatusResult{}, gp.Errorf(gp.CodeNotFound, "status not found")
	}
	r.statuses[ref] = req
	return gp.StatusResult{StatusRef: ref}, nil
}

// StatusOf lets tests observe the last state set under a reference.
func (c *Core) StatusOf(repo, ref string) (gp.SetStatusRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.repos[repo]
	if !ok {
		return gp.SetStatusRequest{}, false
	}
	s, ok := r.statuses[ref]
	return s, ok
}
