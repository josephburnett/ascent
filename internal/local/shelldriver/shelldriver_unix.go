//go:build unix

// The real driver: one Session is one PTY, over creack/pty. The package doc
// and the shared Config live in shelldriver.go.
package shelldriver

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// outputBufferFrames is the depth of the internal PTY-output channel, deep
// enough that the detach-to-reattach gap of a takeover drops no output. When
// it is full the pump goroutine blocks on the PTY read, which back-pressures
// the process; dropping bytes could truncate an ANSI escape sequence.
const outputBufferFrames = 64

// Session is one live PTY. All methods are safe to call concurrently, and one
// that needs the PTY after Close has run returns an error rather than
// panicking on a torn-down file descriptor.
type Session struct {
	cmd  *exec.Cmd
	ptmx *os.File
	pid  int

	// outCh is the single drain point for PTY bytes: one internal pump
	// goroutine writes to it and one subscriber at a time reads from it. The
	// takeover protocol needs a cancel-safe select on this channel, which a
	// blocking PTY Read could not satisfy.
	outCh chan []byte

	closeOnce sync.Once
	closed    atomic.Bool
	doneCh    chan struct{}
	exitErr   error
}

// Start launches a bash session under the given Config. It returns as soon as
// exec succeeds and the PTY is ready.
func Start(cfg Config) (*Session, error) {
	if cfg.Cols == 0 || cfg.Rows == 0 {
		return nil, fmt.Errorf("shelldriver: cols and rows must be > 0 (got %dx%d)", cfg.Cols, cfg.Rows)
	}
	cwd := resolveCwd(cfg.Cwd)
	bashPath := cfg.BashPath
	if bashPath == "" {
		bashPath = "bash"
	}
	args := cfg.Args
	if len(args) == 0 {
		args = []string{"-i"}
	}
	cmd := exec.Command(bashPath, args...)
	cmd.Dir = cwd
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	} else {
		// Default TERM to what xterm.js claims on the client, so bash's
		// prompt rendering is not flat.
		env := append([]string{}, os.Environ()...)
		if os.Getenv("TERM") == "" {
			env = append(env, "TERM=xterm-256color")
		}
		cmd.Env = env
	}
	// Detach into a fresh process group so the parent terminal's SIGINT and
	// SIGTSTP are not forwarded. Close does the signalling instead.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: cfg.Cols,
		Rows: cfg.Rows,
	})
	if err != nil {
		return nil, fmt.Errorf("shelldriver: start bash: %w", err)
	}
	s := &Session{
		cmd:    cmd,
		ptmx:   ptmx,
		pid:    cmd.Process.Pid,
		outCh:  make(chan []byte, outputBufferFrames),
		doneCh: make(chan struct{}),
	}
	go s.reap()
	go s.pump()
	return s, nil
}

// Output returns the channel of PTY-output byte chunks. Each chunk is a fresh
// slice owned by the receiver, aliasing no internal buffer. The channel is
// closed once the bash process has exited or Close has run, so a range loop
// over it terminates.
//
// After Close, chunks the PTY produced before the fd closed, a startup prompt
// for instance, may still arrive before the channel closes, because the pump's
// cancellable send races the consumer's receive. Consumers must drain to close
// and never assume the next receive after Close is the close itself.
func (s *Session) Output() <-chan []byte { return s.outCh }

// pump is the single PTY reader. It runs until the master fd reports EOF,
// meaning bash exited or Close closed the fd, then closes the output channel.
func (s *Session) pump() {
	defer close(s.outCh)
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			// Cancellable send. A plain `outCh <- chunk` would wedge this
			// goroutine forever when outCh is full and nobody drains it,
			// during a takeover gap or after a tile is deleted, because
			// closing the PTY unblocks a blocked Read and leaves a blocked
			// channel send alone. doneCh closes when the process exits, which
			// Close guarantees through SIGTERM then SIGKILL, so the final
			// chunk is dropped instead of leaking the goroutine and the fd.
			// While the process lives doneCh is open, so a full channel still
			// back-pressures the PTY read.
			select {
			case s.outCh <- chunk:
			case <-s.doneCh:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// Write forwards bytes to bash's stdin. After Close it returns 0 and
// io.ErrClosedPipe.
func (s *Session) Write(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return s.ptmx.Write(p)
}

// Resize updates the PTY's window size. Both dimensions must be > 0. It is
// safe to call repeatedly as the pane is dragged.
func (s *Session) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return fmt.Errorf("shelldriver: cols and rows must be > 0 (got %dx%d)", cols, rows)
	}
	if s.closed.Load() {
		return io.ErrClosedPipe
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// Done returns a channel closed when the spawned process has fully exited.
func (s *Session) Done() <-chan struct{} { return s.doneCh }

// Close terminates the bash process group with SIGTERM, then SIGKILL after a
// short grace period. Repeat calls are no-ops. It returns bash's own exit
// error, which is the process's exit status and not a teardown failure.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		// Signal the process group so a child the user spawned, vim or
		// htop, goes down with bash.
		if s.cmd != nil && s.cmd.Process != nil {
			pgid, err := syscall.Getpgid(s.pid)
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGTERM)
			} else {
				_ = s.cmd.Process.Signal(syscall.SIGTERM)
			}
		}
		select {
		case <-s.doneCh:
		case <-time.After(500 * time.Millisecond):
			// Escalate, guarding against a subprocess that hangs
			// instead of respecting SIGTERM.
			if s.cmd != nil && s.cmd.Process != nil {
				pgid, err := syscall.Getpgid(s.pid)
				if err == nil {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				} else {
					_ = s.cmd.Process.Kill()
				}
			}
			<-s.doneCh
		}
		// Closing the master after the child has exited unblocks any
		// in-flight Output reads with io.EOF.
		_ = s.ptmx.Close()
	})
	return s.exitErr
}

// reap waits for the bash process to exit and closes doneCh. It runs in its
// own goroutine so Close can race against it on the timeout.
func (s *Session) reap() {
	if s.cmd != nil {
		s.exitErr = s.cmd.Wait()
	}
	close(s.doneCh)
}

// resolveCwd picks the bash starting directory in priority order: the
// caller's choice if it is an existing dir, then $HOME, then the gridwell
// process's own cwd. A path that does not exist is rejected here so bash does
// not die at exec with a misleading "no such file or directory".
func resolveCwd(want string) string {
	if want != "" && dirExists(want) {
		return want
	}
	if h := os.Getenv("HOME"); h != "" && dirExists(h) {
		return h
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "/"
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
