package plugin

import (
	"errors"
	"fmt"
	"strings"
)

// ErrResponseTooLarge is returned (wrapped) when the host produced a result
// but it could not be delivered because it does not fit in the plugin's
// fixed host-allocation buffer. It is a size limit, not a network failure:
// retry with less data (e.g. a smaller page size), not with backoff.
//
//	if errors.Is(err, plugin.ErrResponseTooLarge) { ... }
var ErrResponseTooLarge = errors.New("response too large")

// hostCodeResponseTooLarge is the "code" value hosts put in an error result
// that maps to ErrResponseTooLarge. Keep in sync with the host's
// errCodeResponseTooLarge (services/api/internal/platform/plugin/runtime.go).
const hostCodeResponseTooLarge = "response_too_large"

// hostResultError builds the error for a host result envelope that carried
// an error message (and optionally a machine-readable code).
func hostResultError(op, msg, code string) error {
	// Host messages already carry the operation prefix ("fetch: ..."); drop it
	// rather than printing "plugin: fetch: fetch: ...".
	msg = strings.TrimPrefix(msg, op+": ")
	if code == hostCodeResponseTooLarge {
		return fmt.Errorf("plugin: %s: %w: %s", op, ErrResponseTooLarge, msg)
	}
	return fmt.Errorf("plugin: %s: %s", op, msg)
}

// errHostNoData is wrapped by [errEmptyHostResponse] so internal callers can
// tell "no data" apart from other failures.
var errHostNoData = errors.New("host returned no data")

// errEmptyHostResponse is returned when a host call delivered no data at all.
// Newer hosts always send an explicit error envelope instead, so reaching
// this means an older host, an exhausted buffer, or a host-side failure --
// not necessarily an unreachable network.
func errEmptyHostResponse(op string) error {
	return fmt.Errorf("plugin: %s: %w (the result may not have fit in the plugin's host buffer, or the host call failed; this does not necessarily mean the network is unreachable)", op, errHostNoData)
}
