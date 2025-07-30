package propagation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// findExecutable finds the given executable name and returns an absolute path.
// The lookup order is:
// 1. If 'name' is an absolute path, it is used directly.
// 2. If 'name' contains a path separator, it's treated as a relative path to the current working directory.
// 3. If 'name' does not contain a separator, it is looked up in PATH (using exec.LookPath).
func findExecutable(name string) (string, error) {
	name = os.ExpandEnv(name)

	// 1. If 'name' is an absolute path, check if it exists and return it.
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err != nil {
			return "", err
		}
		return name, nil
	}

	// 2. If 'name' contains a path separator, treat as a relative path.
	if strings.ContainsRune(name, os.PathSeparator) {
		absPath, err := filepath.Abs(name)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path for '%s': %w", name, err)
		}
		if _, err := os.Stat(absPath); err != nil {
			return "", err
		}
		return absPath, nil
	}

	// 3. Try to find the command in PATH.
	return exec.LookPath(name) // LookPath returns an absolute path or an error
}
