package gitprovider_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/fakeprovider"
)

func TestListIssuesContract(t *testing.T) {
	if !gp.Implemented(fakeprovider.New()).IssueList || gp.Implemented(fakeprovider.NewMinimal()).IssueList {
		t.Fatal("IssueList implementation detection is wrong")
	}
	operations := gp.Operations()
	found := false
	for _, op := range operations {
		if op.Name == "list_issues" && op.Capability == "issue_list" {
			found = true
		}
	}
	if !found {
		t.Fatal("list_issues must be gated by issue_list")
	}
	for name, typ := range map[string]reflect.Type{"ListIssuesRequest": reflect.TypeOf(gp.ListIssuesRequest{}), "IssuePage": reflect.TypeOf(gp.IssuePage{})} {
		if gp.WireTypes()[name] != typ {
			t.Errorf("WireTypes missing %s", name)
		}
	}
	for _, tc := range []struct {
		value any
		want  string
	}{
		{gp.ListIssuesRequest{}, `{"target":{"identity":{"id":""},"repo":{"host":"","full_path":""}}}`},
		{gp.ListIssuesRequest{State: "closed", Cursor: "next"}, `{"target":{"identity":{"id":""},"repo":{"host":"","full_path":""}},"state":"closed","cursor":"next"}`},
		{gp.IssuePage{Items: []gp.Issue{}}, `{"items":[]}`},
		{gp.IssuePage{Items: []gp.Issue{}, NextCursor: "next"}, `{"items":[],"next_cursor":"next"}`},
	} {
		got, err := json.Marshal(tc.value)
		if err != nil || string(got) != tc.want {
			t.Errorf("marshal: %s, %v; want %s", got, err, tc.want)
		}
	}
}

type missingIssueLister struct{ gp.Provider }

func (p missingIssueLister) Describe() gp.Info {
	i := p.Provider.Describe()
	i.Capabilities.IssueList = true
	return i
}

type hiddenIssueLister struct{ *fakeprovider.Full }

func (p hiddenIssueLister) Describe() gp.Info {
	i := p.Full.Describe()
	i.Capabilities.IssueList = false
	return i
}

func TestListIssuesWireGuards(t *testing.T) {
	for _, p := range []gp.Provider{fakeprovider.NewMinimal(), missingIssueLister{fakeprovider.NewMinimal()}, hiddenIssueLister{fakeprovider.New()}} {
		status, raw := gp.Dispatch(context.Background(), p, "list_issues", []byte(`{}`))
		var env gp.Envelope
		if err := json.Unmarshal(raw, &env); err != nil || status != 501 || env.Error == nil || !errors.Is(env.Error, gp.ErrUnsupported) {
			t.Errorf("unsupported: status=%d body=%s err=%v", status, raw, err)
		}
	}
	for _, body := range [][]byte{nil, []byte(`{`)} {
		status, raw := gp.Dispatch(context.Background(), fakeprovider.New(), "list_issues", body)
		var env gp.Envelope
		if err := json.Unmarshal(raw, &env); err != nil || status != 400 || env.Error == nil || env.Error.Code != gp.CodeValidation {
			t.Errorf("invalid body: status=%d body=%s err=%v", status, raw, err)
		}
	}
}

func TestListIssuesDispatchFiltersAndValidates(t *testing.T) {
	f := fakeprovider.Fixtures()
	target := gp.Target{Identity: f.Identity, Repo: f.Repo}
	for _, tc := range []struct {
		state, cursor string
		status        int
		code          gp.ErrorCode
		items         int
	}{
		{"closed", "", 200, "", 0},
		{"all", "", 200, "", 1},
		{"all", "2", 200, "", 0},
		{"invalid", "", 400, gp.CodeValidation, 0},
		{"all", "bad", 400, gp.CodeValidation, 0},
	} {
		body, _ := json.Marshal(gp.ListIssuesRequest{Target: target, State: tc.state, Cursor: tc.cursor})
		status, raw := gp.Dispatch(context.Background(), fakeprovider.New(), "list_issues", body)
		var env gp.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatal(err)
		}
		if status != tc.status {
			t.Fatalf("%+v: status=%d body=%s", tc, status, raw)
		}
		if tc.code != "" {
			if env.Error == nil || env.Error.Code != tc.code {
				t.Fatalf("%+v: body=%s", tc, raw)
			}
			continue
		}
		var page gp.IssuePage
		if err := json.Unmarshal(env.Result, &page); err != nil || !env.OK || len(page.Items) != tc.items {
			t.Fatalf("%+v: body=%s err=%v", tc, raw, err)
		}
	}
}

func TestListIssuesDispatch(t *testing.T) {
	p := fakeprovider.New()
	f := fakeprovider.Harness(false).Fixtures
	body, err := json.Marshal(map[string]any{"target": gp.Target{Identity: f.Identity, Repo: f.Repo}})
	if err != nil {
		t.Fatal(err)
	}
	status, raw := gp.Dispatch(context.Background(), p, "list_issues", body)
	var env gp.Envelope
	if err := json.Unmarshal(raw, &env); err != nil || status != 200 || !env.OK {
		t.Fatalf("list_issues: status=%d body=%s err=%v", status, raw, err)
	}
	var page struct {
		Items []gp.Issue `json:"items"`
	}
	if err := json.Unmarshal(env.Result, &page); err != nil || len(page.Items) != 1 || page.Items[0].Number != f.IssueNumber {
		t.Fatalf("issue page: %+v err=%v", page, err)
	}
}
