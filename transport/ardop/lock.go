// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import "sync"

// Lock is like a sync.Mutex, except:
//   * Lock of locked is noop
//   * Unlock of unlocked is noop
//   * Wait() is used to block until Lock is unlocked.
// The zero-value is an unlocked lock.
type lock struct {
	mu   sync.Mutex
	wait chan struct{}
}

// Locks if unlocked, noop otherwise.
func (l *lock) Lock() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.wait != nil {
		return // Already locked
	}

	l.wait = make(chan struct{})
}

// Unlocks if locked, noop otherwise.
func (l *lock) Unlock() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.wait == nil {
		return // Already unlocked
	}

	close(l.wait)
	l.wait = nil
}

// Blocks until lock is released. Returns immediately if it's unlocked.
func (l *lock) Wait() {
	l.mu.Lock()

	if l.wait == nil {
		return
	}
	wait := l.wait

	l.mu.Unlock()
	<-wait
}
