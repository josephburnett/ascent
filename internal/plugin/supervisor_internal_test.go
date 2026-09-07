package plugin

import (
	"testing"
	"time"
)

// The respawn policy is pure, so it is pinned directly here.
// respawn_e2e_test.go crashes a real subprocess and proves the watch loop uses
// it.
func TestRespawnPauseBacksOffTheYoungAndForgivesTheOld(t *testing.T) {
	for _, tc := range []struct {
		name   string
		last   time.Duration
		ranFor time.Duration
		want   time.Duration
	}{
		{"a process that proved it can stay up comes back at once",
			10 * time.Second, stableFor + time.Second, firstBackoff},
		{"a young death doubles the pause",
			firstBackoff, time.Second, 2 * firstBackoff},
		{"and keeps doubling",
			2 * firstBackoff, 0, 4 * firstBackoff},
		{"a spawn that will not start is a death at age zero",
			time.Second, 0, 2 * time.Second},
		{"the pause is capped",
			maxBackoff, time.Second, maxBackoff},
		{"and never exceeds the cap",
			maxBackoff - time.Second, 0, maxBackoff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := respawnPause(tc.last, tc.ranFor); got != tc.want {
				t.Errorf("respawnPause(%v, %v) = %v, want %v", tc.last, tc.ranFor, got, tc.want)
			}
		})
	}
}
