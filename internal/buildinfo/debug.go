package buildinfo

import (
	"runtime/debug"
)

// Modules is a list describing all modules that is part of this build.
var Modules = func() []*debug.Module {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return append([]*debug.Module{&info.Main}, info.Deps...)
}()

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
