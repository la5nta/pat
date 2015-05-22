// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package transport

// A BusyChannelChecker is a generic busy detector for a
// physical transmission medium.
type BusyChannelChecker interface {
	// Returns true if the channel is clear
	Busy() bool
}
