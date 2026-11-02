package fakeprovider

import (
	"context"
	"errors"
	"testing"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
)

func TestListIssuesStatesAndPages(t *testing.T) {
	p := New()
	target := gp.Target{Identity: gp.IdentityRef{ID: IdentityID}, Repo: gp.RepoRef{FullPath: RepoPath}}
	r := p.repos[RepoPath]
	r.issues["8"] = &gp.Issue{Number: "8", State: "closed"}
	r.issues["9"] = &gp.Issue{Number: "9", State: "open"}
	r.issues["10"] = &gp.Issue{Number: "10", State: "open"}
	for _, tc := range []struct {
		state string
		want  map[string]bool
	}{
		{"", map[string]bool{"7": true, "9": true, "10": true}},
		{"open", map[string]bool{"7": true, "9": true, "10": true}},
		{"closed", map[string]bool{"8": true}},
		{"all", map[string]bool{"7": true, "8": true, "9": true, "10": true}},
	} {
		t.Run(tc.state, func(t *testing.T) {
			seen := map[string]bool{}
			cursor := ""
			for i := 0; i < 10; i++ {
				page, err := p.ListIssues(context.Background(), gp.ListIssuesRequest{Target: target, State: tc.state, Cursor: cursor})
				if err != nil {
					t.Fatal(err)
				}
				for _, issue := range page.Items {
					if !tc.want[issue.Number] || seen[issue.Number] {
						t.Fatalf("unexpected or duplicate issue %q", issue.Number)
					}
					seen[issue.Number] = true
				}
				if page.NextCursor == "" {
					break
				}
				if page.NextCursor == cursor {
					t.Fatal("cursor did not advance")
				}
				cursor = page.NextCursor
				if i == 9 {
					t.Fatal("pagination did not terminate")
				}
			}
			if len(seen) != len(tc.want) {
				t.Fatalf("got %v, want %v", seen, tc.want)
			}
		})
	}
}

func TestListIssuesRejectsInvalidInput(t *testing.T) {
	p := New()
	target := gp.Target{Identity: gp.IdentityRef{ID: IdentityID}, Repo: gp.RepoRef{FullPath: RepoPath}}
	for _, req := range []gp.ListIssuesRequest{
		{Target: target, State: "merged"},
		{Target: target, Cursor: "-1"},
		{Target: target, Cursor: "garbage"},
	} {
		_, err := p.ListIssues(context.Background(), req)
		if !errors.Is(err, gp.Errorf(gp.CodeValidation, "")) {
			t.Errorf("request %+v: got %v, want validation", req, err)
		}
	}
	_, err := p.ListIssues(context.Background(), gp.ListIssuesRequest{Target: gp.Target{}})
	if !errors.Is(err, gp.Errorf(gp.CodeNotFound, "")) {
		t.Errorf("bad target: got %v, want not_found", err)
	}
}
