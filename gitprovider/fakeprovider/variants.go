package fakeprovider

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
)

// Minimal implements only the required surface and declares the smaller,
// honest capability set of a basic platform.
type Minimal struct{ *Core }

// NewMinimal returns a provider with static credentials, repo-scoped webhooks,
// comment-only status, emulated batch review, and only mention triggers.
func NewMinimal() *Minimal {
	return &Minimal{newCore(gp.Capabilities{
		IdentityKinds: []gp.IdentityKind{gp.IdentityToken}, CredentialModel: gp.CredentialStatic,
		WebhookScope: gp.WebhookRepo, StatusKind: gp.StatusComment,
		Triggers:    []gp.TriggerKind{gp.TriggerMention},
		NestedPaths: true,
	})}
}

// Full implements every optional interface and declares every capability.
type Full struct{ *Core }

// New returns the full-featured fake.
func New() *Full {
	return &Full{newCore(gp.Capabilities{
		IdentityKinds: []gp.IdentityKind{gp.IdentityApp, gp.IdentityBotUser}, CredentialModel: gp.CredentialEphemeral,
		WebhookScope: gp.WebhookApp, StatusKind: gp.StatusCheckRun,
		Triggers:           gp.AllTriggerKinds,
		MultipleIdentities: true,
		ResolveThreads:     true, FileRead: true, CodeSearch: true, Labels: true, ReviewerRequests: true, Assignees: true,
		Merge: true, RerunChecks: true, Reactions: true, Branches: true, WebhookManagement: true, IdentityDeletion: true, AccessGrants: true,
		BatchReview: true, IdempotentWrites: true, HiddenMarkers: true, ForkChangeRequests: true, DraftChangeRequests: true,
		CIConfigGlobs: []string{".fake-ci/**"},
	})}
}

// validBranchName reports whether name is acceptable as a branch name (the
// fake's rule is git's core one: no spaces or ref-special characters).
func validBranchName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "..") || strings.HasSuffix(name, ".lock") {
		return false
	}
	return !strings.ContainsAny(name, " ~^:?*[\\")
}

// GetBranch implements gp.BranchManager.
func (f *Full) GetBranch(_ context.Context, req gp.GetBranchRequest) (gp.BranchInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.repo(req.Target)
	if err != nil {
		return gp.BranchInfo{}, err
	}
	sha, ok := r.branches[req.Name]
	if !ok {
		return gp.BranchInfo{}, gp.Errorf(gp.CodeNotFound, "branch not found")
	}
	return gp.BranchInfo{Name: req.Name, SHA: sha, URL: "https://" + Host + "/" + r.ref.FullPath + "/tree/" + req.Name}, nil
}

// CreateBranch implements gp.BranchManager.
func (f *Full) CreateBranch(_ context.Context, req gp.CreateBranchRequest) (gp.BranchInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.repo(req.Target)
	if err != nil {
		return gp.BranchInfo{}, err
	}
	if !validBranchName(req.Name) {
		return gp.BranchInfo{}, gp.Errorf(gp.CodeValidation, "invalid branch name")
	}
	from := req.From
	if from == "" {
		from = r.ref.DefaultBranch
	}
	sha, ok := r.branches[from]
	if !ok {
		return gp.BranchInfo{}, gp.Errorf(gp.CodeNotFound, "source branch not found")
	}
	if _, taken := r.branches[req.Name]; taken {
		return gp.BranchInfo{}, gp.Errorf(gp.CodeConflict, "branch already exists")
	}
	r.branches[req.Name] = sha
	return gp.BranchInfo{Name: req.Name, SHA: sha, URL: "https://" + Host + "/" + r.ref.FullPath + "/tree/" + req.Name}, nil
}

// ResolveThread implements gp.ThreadResolver.
func (f *Full) ResolveThread(_ context.Context, req gp.ResolveThreadRequest) (gp.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, x, err := f.cr(req.Target, req.Number)
	if err != nil {
		return gp.Empty{}, err
	}
	for _, t := range x.threads {
		if t.ID == req.ThreadID {
			t.Resolved = req.Resolved
			return gp.Empty{}, nil
		}
	}
	return gp.Empty{}, gp.Errorf(gp.CodeNotFound, "thread not found")
}

// GetFile implements gp.FileReader.
func (f *Full) GetFile(_ context.Context, req gp.FileRequest) (gp.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.repo(req.Target)
	if err != nil {
		return gp.File{}, err
	}
	b, ok := r.files[req.Path]
	if !ok {
		return gp.File{}, gp.Errorf(gp.CodeNotFound, "file not found")
	}
	if req.MaxBytes > 0 && len(b) > req.MaxBytes {
		return gp.File{Path: req.Path, Content: b[:req.MaxBytes], Truncated: true}, nil
	}
	return gp.File{Path: req.Path, Content: b, SHA: "blob-" + req.Path}, nil
}

// SearchCode implements gp.CodeSearcher.
func (f *Full) SearchCode(_ context.Context, req gp.CodeSearchRequest) (gp.CodeHits, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.repo(req.Target)
	if err != nil {
		return gp.CodeHits{}, err
	}
	if strings.TrimSpace(req.Query) == "" {
		return gp.CodeHits{}, gp.Errorf(gp.CodeValidation, "query is required")
	}
	var hits gp.CodeHits
	for p, b := range r.files {
		if strings.Contains(string(b), req.Query) {
			hits.Items = append(hits.Items, gp.CodeHit{Path: p, Line: 1, Snippet: req.Query})
		}
	}
	return hits, nil
}

func (f *Full) subject(t gp.Target, s gp.SubjectRef) (*repoState, error) {
	r, err := f.repo(t)
	if err != nil {
		return nil, err
	}
	if s.Kind == gp.SubjectChangeRequest {
		if _, ok := r.crs[s.Number]; !ok {
			return nil, gp.Errorf(gp.CodeNotFound, "change request not found")
		}
	} else if _, ok := r.issues[s.Number]; !ok {
		return nil, gp.Errorf(gp.CodeNotFound, "issue not found")
	}
	return r, nil
}

// AddLabels implements gp.LabelManager.
func (f *Full) AddLabels(_ context.Context, req gp.LabelsRequest) (gp.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.subject(req.Target, req.Subject)
	if err != nil {
		return gp.Empty{}, err
	}
	if req.Subject.Kind == gp.SubjectChangeRequest {
		x := r.crs[req.Subject.Number]
		x.Labels = append(x.Labels, req.Labels...)
	} else {
		i := r.issues[req.Subject.Number]
		i.Labels = append(i.Labels, req.Labels...)
	}
	return gp.Empty{}, nil
}

// RequestReviewers implements gp.ReviewerRequester.
func (f *Full) RequestReviewers(_ context.Context, req gp.PeopleRequest) (gp.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.subject(req.Target, req.Subject)
	if err != nil {
		return gp.Empty{}, err
	}
	if req.Subject.Kind != gp.SubjectChangeRequest {
		return gp.Empty{}, gp.Errorf(gp.CodeValidation, "reviewers apply to change requests")
	}
	x := r.crs[req.Subject.Number]
	x.Reviewers = append(x.Reviewers, req.Logins...)
	return gp.Empty{}, nil
}

// SetAssignees implements gp.AssigneeSetter.
func (f *Full) SetAssignees(_ context.Context, req gp.PeopleRequest) (gp.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.subject(req.Target, req.Subject)
	if err != nil {
		return gp.Empty{}, err
	}
	if req.Subject.Kind == gp.SubjectIssue {
		r.issues[req.Subject.Number].Assignees = req.Logins
	}
	return gp.Empty{}, nil
}

// MergeChangeRequest implements gp.Merger.
func (f *Full) MergeChangeRequest(_ context.Context, req gp.MergeRequest) (gp.MergeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, x, err := f.cr(req.Target, req.Number)
	if err != nil {
		return gp.MergeResult{}, err
	}
	if req.ExpectedHeadSHA == "" {
		return gp.MergeResult{}, gp.Errorf(gp.CodeValidation, "expected_head_sha is required")
	}
	if x.Head.SHA != req.ExpectedHeadSHA {
		return gp.MergeResult{}, gp.Errorf(gp.CodeConflict, "head moved since it was reviewed")
	}
	if x.State != gp.CROpen {
		return gp.MergeResult{}, gp.Errorf(gp.CodeConflict, "change request is not open")
	}
	x.State = gp.CRMerged
	return gp.MergeResult{SHA: "merge-" + x.Head.SHA}, nil
}

// RerunChecks implements gp.CheckRerunner.
func (f *Full) RerunChecks(_ context.Context, req gp.RerunRequest) (gp.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.repo(req.Target)
	if err != nil {
		return gp.Empty{}, err
	}
	list, ok := r.checks[req.Ref]
	if !ok {
		return gp.Empty{}, gp.Errorf(gp.CodeNotFound, "no checks for this ref")
	}
	for i := range list {
		if list[i].State == gp.CheckFailure && (req.CheckID == "" || list[i].ID == req.CheckID) {
			list[i].State = gp.CheckPending
		}
	}
	return gp.Empty{}, nil
}

// React implements gp.Reactor.
func (f *Full) React(_ context.Context, req gp.ReactRequest) (gp.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.repo(req.Target)
	if err != nil {
		return gp.Empty{}, err
	}
	switch req.Emoji {
	case "eyes", "rocket", "confused", "+1":
	default:
		return gp.Empty{}, gp.Errorf(gp.CodeValidation, "unsupported emoji")
	}
	r.reactions = append(r.reactions, req.CommentID+":"+req.Emoji)
	return gp.Empty{}, nil
}

// EnsureWebhook implements gp.WebhookManager.
func (f *Full) EnsureWebhook(_ context.Context, req gp.EnsureWebhookRequest) (gp.WebhookRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.repo(req.Target)
	if err != nil {
		return gp.WebhookRef{}, err
	}
	if req.URL == "" {
		return gp.WebhookRef{}, gp.Errorf(gp.CodeValidation, "url is required")
	}
	for id, u := range r.hooks {
		if u == req.URL {
			return gp.WebhookRef{ID: id}, nil // idempotent
		}
	}
	id := f.next("hook")
	r.hooks[id] = req.URL
	return gp.WebhookRef{ID: id}, nil
}

// RemoveWebhook implements gp.WebhookManager.
func (f *Full) RemoveWebhook(_ context.Context, req gp.RemoveWebhookRequest) (gp.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.repo(req.Target)
	if err != nil {
		return gp.Empty{}, err
	}
	delete(r.hooks, req.Webhook.ID) // removing a missing hook is not an error
	return gp.Empty{}, nil
}

// DeleteIdentity implements gp.IdentityDeleter.
func (f *Full) DeleteIdentity(_ context.Context, req gp.DeleteIdentityRequest) (gp.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Deleted = append(f.Deleted, req.Identity.ID)
	return gp.Empty{}, nil
}

// BeginAccessGrant implements gp.AccessGranter.
func (f *Full) BeginAccessGrant(_ context.Context, req gp.BeginAccessRequest) (gp.SetupStep, error) {
	if req.Identity.ID != IdentityID {
		return gp.SetupStep{}, gp.Errorf(gp.CodeNotFound, "identity not found")
	}
	if req.CallbackURL == "" {
		return gp.SetupStep{}, gp.Errorf(gp.CodeValidation, "callback_url is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	state := "grant-" + strconv.Itoa(f.seq)
	if f.grants == nil {
		f.grants = map[string]bool{}
	}
	f.grants[state] = false
	return gp.SetupStep{Kind: gp.StepRedirect, URL: "https://fake.example/grant?owner=" + url.QueryEscape(req.Owner), State: state}, nil
}

// CompleteAccessGrant implements gp.AccessGranter. A state works once.
func (f *Full) CompleteAccessGrant(_ context.Context, req gp.CompleteAccessRequest) (gp.AccessGrant, error) {
	if req.Identity.ID != IdentityID {
		return gp.AccessGrant{}, gp.Errorf(gp.CodeNotFound, "identity not found")
	}
	if req.State == "" {
		return gp.AccessGrant{}, gp.Errorf(gp.CodeValidation, "state is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	used, known := f.grants[req.State]
	if !known || used {
		return gp.AccessGrant{}, gp.Errorf(gp.CodeForbidden, "unknown or already used state")
	}
	f.grants[req.State] = true
	return gp.AccessGrant{Owner: req.Params["owner"]}, nil
}
