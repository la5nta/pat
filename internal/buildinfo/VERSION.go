// Copyright 2017 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package buildinfo

const (
	// AppName is the friendly name of the app.
	//
	// Forks should consider using a different name.
	AppName = "Pat"

	// Version is the app's SemVer.
	//
	// Forks should NOT bump this unless they use a unique AppName. The Winlink
	// system uses this to derive the "these users should upgrade" wall of shame
	// from CMS connects.
	Version = "0.14.1"
)
