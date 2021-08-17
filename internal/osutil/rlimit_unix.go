// +build !windows,!freebsd

package osutil

import (
	"fmt"
	"syscall"
)

// RaiseOpenFileLimit tries to maximize the limit of open file descriptors, limited by max or the OS's hard limit
func RaiseOpenFileLimit(max uint64) error {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return fmt.Errorf("could not get current limit: %w", err)
	}
	if limit.Cur >= limit.Max || limit.Cur >= max {
		return nil
	}
	limit.Cur = limit.Max
	if limit.Cur > max {
		limit.Cur = max
	}
	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit)
}
