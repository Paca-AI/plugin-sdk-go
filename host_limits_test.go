package plugin

import (
	"errors"
	"strings"
	"testing"
)

func TestHostResultError_ResponseTooLarge(t *testing.T) {
	err := hostResultError("fetch", "fetch: response of 1238310 bytes does not fit", hostCodeResponseTooLarge)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("errors.Is(err, ErrResponseTooLarge) = false for %v", err)
	}
	for _, want := range []string{"plugin: fetch:", "response too large", "1238310"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestHostResultError_PlainError(t *testing.T) {
	err := hostResultError("fetch", "fetch: domain not permitted by plugin manifest", "")
	if errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("plain host error must not match ErrResponseTooLarge: %v", err)
	}
	if got, want := err.Error(), "plugin: fetch: fetch: domain not permitted by plugin manifest"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestErrEmptyHostResponse_NotMisleading(t *testing.T) {
	msg := errEmptyHostResponse("fetch").Error()
	if strings.Contains(msg, "empty response from host") {
		t.Errorf("message should not use the old ambiguous wording: %q", msg)
	}
	if !strings.Contains(msg, "does not necessarily mean the network is unreachable") {
		t.Errorf("message should disclaim a network failure: %q", msg)
	}
}
