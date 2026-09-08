//go:build !unix

// The no-PTY half. Start refuses, so this build links no creack/pty and no
// syscall.Kill.
package shelldriver

import (
	"fmt"
	"runtime"
)

// Session exists so both halves present the same type to shellsvc.Session.
// Start never returns one; its channels are already closed and its writes
// refused, so a caller that ignored Start's error terminates instead of
// hanging.
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

// Start refuses; see ErrShellsUnavailable.
func Start(_ Config) (*Session, error) {
	return nil, fmt.Errorf("shelldriver: %w (%s)", ErrShellsUnavailable, runtime.GOOS)
}

func (s *Session) Output() <-chan []byte       { return closedOutput }
func (s *Session) Write(_ []byte) (int, error) { return 0, ErrShellsUnavailable }
func (s *Session) Resize(_, _ uint16) error    { return ErrShellsUnavailable }
func (s *Session) Done() <-chan struct{}       { return closedDone }
func (s *Session) Close() error                { return nil }
