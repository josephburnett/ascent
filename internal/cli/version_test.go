package cli

import "testing"

// An unstamped binary is honestly "dev", never empty: a support question
// about which build someone is running always gets an answer, and a blank
// line would read as a broken command.
func TestVersionStringIsNeverEmpty(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })

	Version = ""
	if got := VersionString(); got != "dev" {
		t.Errorf("unstamped VersionString() = %q, want \"dev\"", got)
	}
	Version = "0.1.0"
	if got := VersionString(); got != "0.1.0" {
		t.Errorf("stamped VersionString() = %q, want the stamp verbatim", got)
	}
}
