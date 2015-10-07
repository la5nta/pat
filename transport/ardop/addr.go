// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

const network = "ardop"

type Addr struct{ string }

func (a Addr) Network() string { return network }
func (a Addr) String() string {
	return a.string
}
