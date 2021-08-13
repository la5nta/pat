package buildinfo

import (
	"fmt"
	"runtime"
)

// VersionString returns a very descriptive version including the app SemVer, git rev plus the
// Golang OS, architecture and version.
func VersionString() string {
	return fmt.Sprintf("%s %s/%s - %s",
		VersionStringShort(), runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// VersionStringShort returns the app SemVer and git rev.
func VersionStringShort() string {
	return fmt.Sprintf("v%s (%s)", Version, GitRev)
}

// UserAgent returns a suitable HTTP user agent string containing app name, SemVer, git rev, plus
// the Golang OS, architecture and version.
func UserAgent() string {
	return fmt.Sprintf("%v/%v (%v) %v (%v; %v)",
		AppName, Version, GitRev, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
