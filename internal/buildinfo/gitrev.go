//go:build go1.18
// +build go1.18

package buildinfo

import "runtime/debug"

// GitRev is the git commit hash that the binary was built at.
var GitRev = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) > 7 {
				return setting.Value[:7]
			}
		}
	}
	return "unknown origin"
}()
