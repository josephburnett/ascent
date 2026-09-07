//go:build windows

package cli

// Windows stub: no flock, so a windows build serves without the per-home
// guard. The status probe refuses honestly rather than answering "not
// serving", because a false negative would let a second server start.

import "errors"

type serveLock struct{}

func acquireServeLock(home string) (*serveLock, error) { return &serveLock{}, nil }
func (l *serveLock) WriteBanner(banner string)         {}
func (l *serveLock) Release()                          {}

func probeServeLock(home string) (string, bool, error) {
	return "", false, errors.New("serve-lock probe is not supported on this platform")
}
