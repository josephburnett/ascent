//go:build !unix

// The no-PTY half: Windows and anything else without a unix PTY. Start
// refuses, so this build links no creack/pty and no syscall.Kill. The
// package doc and the shared Config live in shelldriver.go.
package shelldriver

import (
	"fmt"
	"runtime"
)

// Session exists here only so the two halves present the same type to
// shellsvc.Session. Start never returns one — it always fails — so these
// methods are the shape, not a code path: every channel is already closed
// and every write is refused, so a caller that ignored Start's error still
// terminates instead of hanging on a nil channel.
type Session struct{}

var (
	closedOutput = func() chan []byte {
		c := make(chan []byte)
		close(c)
		return c
	}()
	closedDone = func() chan struct{} {
		c := make(chan struct{})
		close(c)
		return c
	}()
)

// Start refuses on a platform with no PTY. The error is the whole behavior:
// it travels the ordinary shell-open path — shellsvc.Manager.Acquire, the
// home namespace's OpenShell, the shell door's exit frame — and lands on the
// client as the reason a shell would not attach, the same route a dead tmux
// session takes. Shells unavailable is a state, not a crash.
func Start(_ Config) (*Session, error) {
	return nil, fmt.Errorf("shelldriver: %w (%s)", ErrShellsUnavailable, runtime.GOOS)
}

func (s *Session) Output() <-chan []byte       { return closedOutput }
func (s *Session) Write(_ []byte) (int, error) { return 0, ErrShellsUnavailable }
func (s *Session) Resize(_, _ uint16) error    { return ErrShellsUnavailable }
func (s *Session) Done() <-chan struct{}       { return closedDone }
func (s *Session) Close() error                { return nil }
