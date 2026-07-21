package plugin

import (
	"testing"
	"time"
)

// ttlToSeconds feeds the WASM host's cache_set import, which only accepts
// whole seconds and treats 0 as "store without expiry". These cases guard
// against a sub-second ttl silently truncating to 0 and turning a
// short-lived cache entry into a permanent one.
func TestTTLToSeconds(t *testing.T) {
	cases := []struct {
		name string
		ttl  time.Duration
		want int32
	}{
		{"zero", 0, 0},
		{"negative", -time.Second, 0},
		{"sub-second rounds up to one second, not zero", 500 * time.Millisecond, 1},
		{"just under a second rounds up", 999 * time.Millisecond, 1},
		{"exact second stays exact", time.Second, 1},
		{"partial second past a whole second rounds up", 1500 * time.Millisecond, 2},
		{"minutes", 5 * time.Minute, 300},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ttlToSeconds(tc.ttl); got != tc.want {
				t.Fatalf("ttlToSeconds(%v) = %d, want %d", tc.ttl, got, tc.want)
			}
		})
	}
}
