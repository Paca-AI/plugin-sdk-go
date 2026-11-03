package plugin

import (
	"strings"
	"testing"
)

func TestBoundResult_PassesResultsUpToTheLimit(t *testing.T) {
	small := []byte("ok")
	if got := boundResult(small, func(string) []byte { t.Fatal("must not build a fallback"); return nil }); string(got) != "ok" {
		t.Fatalf("got %q", got)
	}
	atLimit := make([]byte, maxResultBytes)
	if got := boundResult(atLimit, func(string) []byte { t.Fatal("limit is inclusive"); return nil }); len(got) != maxResultBytes {
		t.Fatalf("len = %d", len(got))
	}
}

func TestBoundResult_ReplacesAnOversizedResultWithAnExplicitError(t *testing.T) {
	var msg string
	got := boundResult(make([]byte, maxResultBytes+1), func(m string) []byte { msg = m; return []byte("err") })
	if string(got) != "err" {
		t.Fatalf("got %q", got)
	}
	for _, want := range []string{"exceeds the limit", "16777217"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}
