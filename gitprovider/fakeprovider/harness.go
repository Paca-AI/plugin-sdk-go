package fakeprovider

import (
	"encoding/json"
	"testing"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/conformance"
)

func delivery(h fakeHook) gp.WebhookRequest {
	body, _ := json.Marshal(h)
	return gp.WebhookRequest{
		Identity: gp.IdentityRef{ID: IdentityID},
		Method:   "POST",
		Headers:  map[string]string{"x-fake-signature": Sign(body)},
		Body:     body,
	}
}

// Fixtures returns the conformance fixtures for the fake's seed data. It is also
// the template for a real provider's fixtures: canned deliveries recorded from
// the platform, with the expectations a correct adapter must produce.
func Fixtures() conformance.Fixtures {
	identity := gp.IdentityRef{ID: IdentityID, Handle: IdentityHandle}
	repo := gp.RepoRef{Host: Host, FullPath: RepoPath, DefaultBranch: "main"}
	bad := delivery(fakeHook{Delivery: "d-bad", Type: "comment", Repo: RepoPath, Number: "1", Actor: "octocat", Text: "@paca-bot hi"})
	bad.Headers["x-fake-signature"] = "sha256=deadbeef"

	return conformance.Fixtures{
		Identity: identity, Repo: repo,
		InvisibleRepo:     gp.RepoRef{Host: Host, FullPath: "secret/hidden"},
		RepoURL:           "https://" + Host + "/" + RepoPath + ".git",
		Candidates:        []gp.IdentityRef{identity},
		ExpectedRepoPaths: allRepoPaths,

		ChangeRequest: SeedCRNumber, MissingCR: "999",
		ChangedFiles: []string{"src/main.go", "src/util.go", "docs/guide.md"},
		HeadSHA:      SeedHeadSHA, FailingCheckID: "chk-1",
		IssueNumber: "7", ThreadID: "thr-1",
		Actors: map[string]gp.Permission{"octocat": gp.PermissionWrite, "reader": gp.PermissionRead, "owner": gp.PermissionAdmin},

		BadSignature: bad,
		Secrets:      []string{WebhookSecret, StaticSecret},
		Webhooks: []conformance.WebhookCase{
			{Name: "mention-in-comment",
				Request: delivery(fakeHook{Delivery: "d1", Type: "comment", Action: "created", Repo: RepoPath, Subject: "change_request", Number: "1", Actor: "octocat", Text: "@paca-bot please fix the test", Path: "src/main.go", Line: 3, Thread: "thr-1", Comment: "c-9"}),
				Want:    []conformance.Want{{Kind: gp.EventComment, Action: "created", RepoPath: RepoPath, SubjectKind: gp.SubjectChangeRequest, SubjectNumber: "1", ActorLogin: "octocat", ActorKind: gp.ActorUser, Addressed: []gp.AddressVia{gp.ViaMention}}}},
			{Name: "comment-without-mention",
				Request: delivery(fakeHook{Delivery: "d2", Type: "comment", Action: "created", Repo: RepoPath, Subject: "issue", Number: "7", Actor: "reader", Text: "thanks!"}),
				Want:    []conformance.Want{{Kind: gp.EventComment, SubjectKind: gp.SubjectIssue, SubjectNumber: "7", ActorLogin: "reader", ActorKind: gp.ActorUser}}},
			{Name: "email-address-is-not-a-mention",
				Request: delivery(fakeHook{Delivery: "d3", Type: "comment", Action: "created", Repo: RepoPath, Number: "1", Actor: "octocat", Text: "mail me at dev@paca-bot.example"}),
				Want:    []conformance.Want{{Kind: gp.EventComment, ActorLogin: "octocat"}}},
			{Name: "bot-actor",
				Request: delivery(fakeHook{Delivery: "d4", Type: "comment", Action: "created", Repo: RepoPath, Number: "1", Actor: "dependabot", ActorKind: "bot", Text: "bump"}),
				Want:    []conformance.Want{{Kind: gp.EventComment, ActorLogin: "dependabot", ActorKind: gp.ActorBot}}},
			{Name: "review-requested-from-bot",
				Request: delivery(fakeHook{Delivery: "d5", Type: "review_request", Action: "requested", Repo: RepoPath, Number: "1", Actor: "octocat", Requested: IdentityHandle}),
				Want:    []conformance.Want{{Kind: gp.EventReviewRequest, Action: "requested", ActorLogin: "octocat", Addressed: []gp.AddressVia{gp.ViaReviewRequest}}}},
			{Name: "assigned-to-bot",
				Request: delivery(fakeHook{Delivery: "d6", Type: "assignment", Action: "assigned", Repo: RepoPath, Subject: "issue", Number: "7", Actor: "octocat", Assignee: IdentityHandle}),
				Want:    []conformance.Want{{Kind: gp.EventAssignment, SubjectKind: gp.SubjectIssue, Addressed: []gp.AddressVia{gp.ViaAssignment}}}},
			{Name: "ci-failed",
				Request: delivery(fakeHook{Delivery: "d7", Type: "check", Action: "completed", Repo: RepoPath, Number: "1", Actor: "ci", ActorKind: "bot", HeadSHA: SeedHeadSHA, Check: "ci/test", Result: "failure"}),
				Want:    []conformance.Want{{Kind: gp.EventCheck, Action: "completed", ActorKind: gp.ActorBot, CheckConclusion: "failure"}}},
			{Name: "branch-created-by-push",
				Request: delivery(fakeHook{Delivery: "d10", Type: "push", Action: "created", Repo: RepoPath, Actor: "octocat", HeadRef: "feat/PACA-12-x", HeadSHA: "def456", Created: true}),
				Want:    []conformance.Want{{Kind: gp.EventPush, Action: "created", ActorLogin: "octocat"}}},
			{Name: "unmapped-event-is-dropped",
				Request: delivery(fakeHook{Delivery: "d8", Type: "star", Repo: RepoPath, Actor: "octocat"}),
				Want:    nil},
			{Name: "unknown-repository-is-dropped",
				Request: delivery(fakeHook{Delivery: "d9", Type: "comment", Action: "created", Repo: "nobody/knows", Number: "1", Actor: "octocat", Text: "@paca-bot"}),
				Want:    nil},
		},

		SetupHost:           Host,
		SetupCompleteParams: map[string]string{"token": "pasted-token"},

		MutationsAllowed: true,
		NewBranchHead:    "feat/new", BaseBranch: "main",
		File:        struct{ Ref, Path string }{Ref: "main", Path: "README.md"},
		SearchQuery: "package main",
		Reviewer:    "reader",

		AccessGrantParams: map[string]string{"owner": "acme"}, AccessOwner: "acme",
	}
}

// Harness returns a conformance harness for the full or minimal fake.
func Harness(minimal bool) conformance.Harness {
	return conformance.Harness{
		New: func(t *testing.T) gp.Provider {
			if minimal {
				return NewMinimal()
			}
			return New()
		},
		Fixtures: Fixtures(),
	}
}
