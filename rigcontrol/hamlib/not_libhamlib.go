// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// +build !cgo, !libhamlib

package hamlib

import "errors"

var ErrNotAvailable = errors.New("Not available in this build")

func OpenSerialURI(uri string) (Rig, error) { return nil, ErrNotAvailable }
func Rigs() map[RigModel]string             { return map[RigModel]string{} }
