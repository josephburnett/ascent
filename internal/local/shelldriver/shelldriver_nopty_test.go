//go:build !unix

package shelldriver

import (
	"errors"
	"strings"
	"testing"
)

// The no-PTY Start refuses, with a reason a person can read, and refuses
// through ErrShellsUnavailable so callers can classify it. This is the whole
// contract of the half: it never half-opens a session.
func TestStartRefusesWithoutAPTY(t *testing.T) {
	s, err := Start(Config{Cols: 80, Rows: 24})
	if err == nil {
		t.Fatal("Start succeeded on a platform with no PTY")
	}
	if s != nil {
		t.Fatalf("Start returned a session with its error: %#v", s)
	}
	if !errors.Is(err, ErrShellsUnavailable) {
		t.Errorf("Start error = %v; want it to wrap ErrShellsUnavailable", err)
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("Start error = %q; the user-facing reason must read as unavailability", err)
	}
}

// A zero size is not the interesting failure here: with no PTY at all, the
// refusal must not depend on the caller's arguments.
func TestStartRefusesRegardlessOfConfig(t *testing.T) {
	for _, cfg := range []Config{{}, {Cols: 1, Rows: 1}, {Cols: 200, Rows: 60, BashPath: "bash"}} {
		if _, err := Start(cfg); !errors.Is(err, ErrShellsUnavailable) {
			t.Errorf("Start(%+v) error = %v; want ErrShellsUnavailable", cfg, err)
		}
	}
}
