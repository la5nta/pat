package directories

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsInPath(t *testing.T) {
	t.Run("absolute paths", func(t *testing.T) {
		runIsInPathCases(t, t.TempDir(), false)
	})
	t.Run("relative paths", func(t *testing.T) {
		runIsInPathCases(t, t.TempDir(), true)
	})
	t.Run("parent does not exist", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "does_not_exist_parent")
		sub := filepath.Join(parent, "subdir")
		if IsInPath(parent, sub) {
			t.Errorf("should return false when parent does not exist")
		}
	})
	t.Run("mix abs/rel (should panic)", func(t *testing.T) {
		dir := t.TempDir()
		rel := "subdir"
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic when mixing abs/rel paths")
			}
		}()
		_ = IsInPath(dir, rel)
	})
}

func runIsInPathCases(t *testing.T, base string, makeRelative bool) {
	sub := filepath.Join(base, "subdir")
	file := filepath.Join(sub, "file.txt")
	os.MkdirAll(sub, 0755)
	os.WriteFile(file, []byte("test"), 0644)

	otherDir := filepath.Join(base, "..", "otherdir")
	os.MkdirAll(otherDir, 0755)
	otherFile := filepath.Join(otherDir, "other.txt")
	os.WriteFile(otherFile, []byte("other"), 0644)

	parent := filepath.Dir(base)
	nonExistent := filepath.Join(sub, "does_not_exist.txt")

	if makeRelative {
		// Change working directory
		origCwd, _ := os.Getwd()
		defer os.Chdir(origCwd)
		cwd := filepath.Join(base, "..")
		os.Chdir(cwd)

		// Convert all paths to be relative to cwd
		rel := func(p string) string {
			rel, _ := filepath.Rel(cwd, p)
			return rel
		}
		sub = rel(sub)
		file = rel(file)
		otherFile = rel(otherFile)
		parent = rel(parent)
		nonExistent = rel(nonExistent)
		base = rel(base)
	}

	if !IsInPath(base, sub) {
		t.Errorf("subdir should be in base")
	}
	if !IsInPath(base, file) {
		t.Errorf("file should be in base")
	}
	if IsInPath(base, otherFile) {
		t.Errorf("file in otherdir should not be in base")
	}
	if IsInPath(base, parent) {
		t.Errorf("parent should not be in base")
	}
	if !IsInPath(base, nonExistent) {
		t.Errorf("non-existent file within base should return true")
	}
}
