//go:build !go1.18
// +build !go1.18

package buildinfo

// GitRev is the git commit hash that the binary was built at.
var GitRev = "unknown origin" // Set by make.bash
