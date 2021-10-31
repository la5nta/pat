// Copyright 2017 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package buildinfo

const (
	// AppName is the friendly name of the app.
	AppName = "Pat"
	// Version is the app's SemVer.
	Version = "0.12.0"
)

// GitRev is the git commit hash that the binary was built at.
var GitRev = "unknown origin" // Set by make.bash
