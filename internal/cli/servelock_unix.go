//go:build unix

package cli

// The flock half of the serve lock. servelock.go holds the contract and the
// error type both halves present.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// serveLock is the held exclusive lock; the zero value is never valid.
type serveLock struct {
	f *os.File
}

// acquireServeLock takes the exclusive per-home lock, or returns
// *errServeLockHeld carrying the running holder's banner.
func acquireServeLock(home string) (*serveLock, error) {
	path := filepath.Join(home, "serve.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("serve lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		banner, _ := os.ReadFile(path)
		f.Close()
		return nil, &errServeLockHeld{banner: strings.TrimSpace(string(banner))}
	}
	// Won: any content is a crashed holder's leftover, since a clean Release
	// removes the file. Empty it until our banner is known.
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, fmt.Errorf("serve lock: %w", err)
	}
	return &serveLock{f: f}, nil
}

// WriteBanner records the holder's serve banner: the line a conflicting
// serve re-emits so the desktop app connects to this one instead.
func (l *serveLock) WriteBanner(banner string) {
	_, _ = l.f.WriteAt([]byte(banner+"\n"), 0)
	_ = l.f.Sync()
}

// probeServeLock answers "is anyone serving this home?" without acquiring:
// a shared, non-blocking LOCK_SH flock, which coexists with other probes
// and never truncates or unlinks. Taking the exclusive lock for the test
// would let a read-only question beat a starting serve to the flock and
// manufacture a failure.
func probeServeLock(home string) (banner string, held bool, err error) {
	path := filepath.Join(home, "serve.lock")
	f, oerr := os.Open(path)
	if oerr != nil {
		if os.IsNotExist(oerr) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("serve lock: %w", oerr)
	}
	defer f.Close()
	if flerr := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); flerr != nil {
		// Exclusively held: a serve is running, or mid-start.
		b, _ := os.ReadFile(path)
		return strings.TrimSpace(string(b)), true, nil
	}
	// A shared lock means nobody holds the exclusive one. Closing drops it;
	// the file stays, as the crashed-holder breadcrumb.
	return "", false, nil
}

// Release drops the lock and removes the file, so a leftover serve.lock
// means the holder crashed. That is informational only: the flock is what
// gates, and a dead holder's flock is already gone.
func (l *serveLock) Release() {
	_ = os.Remove(l.f.Name())
	_ = l.f.Close() // closing drops the flock
}
