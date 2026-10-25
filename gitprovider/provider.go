package gitprovider

import "context"

// Provider is the required surface of a Git platform adapter. Everything here
// exists on every platform Paca targets; platform-specific extras are the
// optional interfaces below.
type Provider interface {
	// Describe returns static information and capabilities.
	Describe() Info

	// ── identity setup ──
	BeginIdentitySetup(ctx context.Context, req BeginSetupRequest) (SetupStep, error)
	CompleteIdentitySetup(ctx context.Context, req CompleteSetupRequest) (Identity, error)

	// ── repositories ──
	ListRepositories(ctx context.Context, req ListReposRequest) (RepositoryPage, error)
	ResolveRepository(ctx context.Context, req ResolveRepoRequest) (ResolvedRepository, error)

	// ── webhooks ──

	// VerifyAndParseWebhook checks the delivery's signature/secret and returns
	// normalized events (possibly none). It is the only place a platform's
	// webhook format lives. A failed verification returns [CodeInvalidSignature].
	VerifyAndParseWebhook(ctx context.Context, req WebhookRequest) ([]Event, error)

	// ── credentials and policy inputs ──
	MintCredential(ctx context.Context, req MintRequest) (Credential, error)
	GetActorPermission(ctx context.Context, req ActorPermissionRequest) (Permission, error)
	GetBranchProtection(ctx context.Context, req BranchProtectionRequest) (BranchProtection, error)

	// ── reads ──
	GetChangeRequest(ctx context.Context, req CRRequest) (ChangeRequest, error)
	ListChangedFiles(ctx context.Context, req PagedCRRequest) (ChangedFilePage, error)
	GetDiff(ctx context.Context, req DiffRequest) (Diff, error)
	ListThreads(ctx context.Context, req PagedCRRequest) (ThreadPage, error)
	ListComments(ctx context.Context, req ListCommentsRequest) (CommentPage, error)
	GetIssue(ctx context.Context, req IssueRequest) (Issue, error)
	GetChecks(ctx context.Context, req ChecksRequest) ([]Check, error)
	GetCheckLogs(ctx context.Context, req CheckLogsRequest) (Logs, error)

	// ── writes ──
	OpenChangeRequest(ctx context.Context, req OpenCRRequest) (ChangeRequest, error)
	UpdateChangeRequest(ctx context.Context, req UpdateCRRequest) (ChangeRequest, error)
	Comment(ctx context.Context, req CommentRequest) (CommentResult, error)
	ReplyThread(ctx context.Context, req ReplyThreadRequest) (CommentResult, error)
	SubmitReview(ctx context.Context, req SubmitReviewRequest) (ReviewResult, error)
	SetStatus(ctx context.Context, req SetStatusRequest) (StatusResult, error)
}

// Optional interfaces. Implement one only when the platform supports it, and
// set the matching [Capabilities] flag; the conformance suite checks that the
// two agree.

// ThreadResolver resolves and reopens review threads (capability ResolveThreads).
type ThreadResolver interface {
	ResolveThread(ctx context.Context, req ResolveThreadRequest) (Empty, error)
}

// FileReader reads files at a ref without a checkout (capability FileRead).
type FileReader interface {
	GetFile(ctx context.Context, req FileRequest) (File, error)
}

// CodeSearcher searches repository code (capability CodeSearch).
type CodeSearcher interface {
	SearchCode(ctx context.Context, req CodeSearchRequest) (CodeHits, error)
}

// LabelManager adds labels (capability Labels).
type LabelManager interface {
	AddLabels(ctx context.Context, req LabelsRequest) (Empty, error)
}

// ReviewerRequester requests reviewers (capability ReviewerRequests).
type ReviewerRequester interface {
	RequestReviewers(ctx context.Context, req PeopleRequest) (Empty, error)
}

// AssigneeSetter sets assignees (capability Assignees).
type AssigneeSetter interface {
	SetAssignees(ctx context.Context, req PeopleRequest) (Empty, error)
}

// Merger merges change requests (capability Merge).
type Merger interface {
	MergeChangeRequest(ctx context.Context, req MergeRequest) (MergeResult, error)
}

// CheckRerunner re-runs failed checks (capability RerunChecks).
type CheckRerunner interface {
	RerunChecks(ctx context.Context, req RerunRequest) (Empty, error)
}

// Reactor adds emoji reactions (capability Reactions).
type Reactor interface {
	React(ctx context.Context, req ReactRequest) (Empty, error)
}

// WebhookManager registers webhooks on repo/group-scoped platforms (capability
// WebhookManagement).
type WebhookManager interface {
	EnsureWebhook(ctx context.Context, req EnsureWebhookRequest) (WebhookRef, error)
	RemoveWebhook(ctx context.Context, req RemoveWebhookRequest) (Empty, error)
}

// IdentityDeleter cleans up an identity remotely (capability IdentityDeletion).
type IdentityDeleter interface {
	DeleteIdentity(ctx context.Context, req DeleteIdentityRequest) (Empty, error)
}

// AccessGranter lets a user grant an identity access to one more account or
// organization (capability AccessGrants).
type AccessGranter interface {
	BeginAccessGrant(ctx context.Context, req BeginAccessRequest) (SetupStep, error)
	CompleteAccessGrant(ctx context.Context, req CompleteAccessRequest) (AccessGrant, error)
}

// BranchManager looks up and creates branches (capability Branches). GetBranch
// returns not_found for a missing branch; CreateBranch returns conflict when the
// name is taken, not_found when From does not exist and validation for an
// invalid name.
type BranchManager interface {
	GetBranch(ctx context.Context, req GetBranchRequest) (BranchInfo, error)
	CreateBranch(ctx context.Context, req CreateBranchRequest) (BranchInfo, error)
}

// Implemented reports which optional capabilities p actually implements, so
// tooling can compare them with what [Info] declares.
func Implemented(p Provider) Capabilities {
	var c Capabilities
	_, c.ResolveThreads = p.(ThreadResolver)
	_, c.FileRead = p.(FileReader)
	_, c.CodeSearch = p.(CodeSearcher)
	_, c.Labels = p.(LabelManager)
	_, c.ReviewerRequests = p.(ReviewerRequester)
	_, c.Assignees = p.(AssigneeSetter)
	_, c.Merge = p.(Merger)
	_, c.RerunChecks = p.(CheckRerunner)
	_, c.Reactions = p.(Reactor)
	_, c.Branches = p.(BranchManager)
	_, c.WebhookManagement = p.(WebhookManager)
	_, c.IdentityDeletion = p.(IdentityDeleter)
	_, c.AccessGrants = p.(AccessGranter)
	return c
}
