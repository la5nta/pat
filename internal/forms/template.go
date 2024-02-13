package forms

import (
	"bufio"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
)

// Template holds information about a Winlink form template
type Template struct {
	Name string `json:"name"`
	Path string `json:"template_path"`

	InitialURI      string `json:"-"`
	ViewerURI       string `json:"-"`
	ReplyTxtFileURI string `json:"-"`
}

func readTemplate(path string, filesMap formFilesMap) (Template, error) {
	f, err := os.Open(path)
	if err != nil {
		return Template{}, err
	}
	defer f.Close()

	template := Template{
		Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Path: path,
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		switch key, value, _ := strings.Cut(scanner.Text(), ":"); key {
		case "Form":
			// Form: <composer>,<viewer>
			files := strings.Split(value, ",")
			// Extend to absolute paths and add missing html extension
			for i, name := range files {
				name = strings.TrimSpace(name)
				files[i] = resolveFileReference(filesMap, filepath.Dir(path), name)
				if files[i] == "" {
					debug.Printf("%s: failed to resolve referenced file %q", template.Path, name)
				}
			}
			template.InitialURI = files[0]
			if len(files) > 1 {
				template.ViewerURI = files[1]
			}
		case "ReplyTemplate":
			name := strings.TrimSpace(value)
			template.ReplyTxtFileURI = resolveFileReference(filesMap, filepath.Dir(path), name)
			if template.ReplyTxtFileURI == "" {
				debug.Printf("%s: failed to resolve referenced reply template file %q", template.Path, name)
				continue
			}
		}
	}
	return template, err
}

// resolveFileReference searches for files referenced in .txt files.
//
// If found the returned path is the absolute path, otherwise an empty string is returned.
func resolveFileReference(filesMap formFilesMap, basePath string, referencePath string) string {
	path := filepath.Join(basePath, referencePath)
	if !directories.IsInPath(basePath, path) {
		debug.Printf("%q escapes template's base path (%q)", referencePath, basePath)
		return ""
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	// Attempt by guessing the file extension.
	debugName := filepath.Join(filepath.Base(basePath), referencePath)
	for _, ext := range []string{htmlFileExt, replyFileExt, txtFileExt} {
		if _, err := os.Stat(path + ext); err == nil {
			debug.Printf("found %q by guessing file extension (%s)", debugName, ext)
			return path + ext
		}
	}
	// Fallback to map based lookup.
	if path := filesMap.get(referencePath); path != "" {
		debug.Printf("found %q by map based lookup", debugName)
		return path
	}
	return ""
}

type formFilesMap map[string]string

func (m formFilesMap) get(name string) string {
	if path, ok := m[strings.ToLower(name)]; ok {
		return path
	}
	if filepath.Ext(name) != "" {
		return ""
	}
	// Attempt by guessing the file extension
	for _, ext := range []string{htmlFileExt, replyFileExt, txtFileExt} {
		if path := m.get(name + ext); path != "" {
			debug.Printf("found %q (in map) by guessing file extension (%s)", name, ext)
			return path
		}
	}
	return ""
}

// formFilesFromPath returns a map from filenames to absolute paths of
// identified HTML Forms and reply templates within the given base path.
func formFilesFromPath(basePath string) formFilesMap {
	m := make(map[string]string)
	isFormFile := func(name string) bool {
		return strings.EqualFold(filepath.Ext(name), htmlFileExt)
	}
	isReplyTemplate := func(name string) bool {
		return strings.EqualFold(filepath.Ext(name), replyFileExt)
	}
	add := func(name, path string) {
		name = strings.ToLower(name)
		if !(isFormFile(name) || isReplyTemplate(name)) {
			return
		}
		if dup, ok := m[name]; ok {
			debug.Printf("duplicate filenames: %q, %q", path, dup)
		}
		m[name] = path
	}
	err := fs.WalkDir(os.DirFS(basePath), ".", func(path string, d fs.DirEntry, err error) error {
		switch {
		case path == ".":
			return nil
		case err != nil:
			return err
		case d.IsDir():
			for k, v := range formFilesFromPath(path) {
				add(k, v)
			}
			return nil
		default:
			add(d.Name(), filepath.Join(basePath, path))
			return nil
		}
	})
	if err != nil {
		log.Printf("failed to walk path %q: %v", basePath, err)
	}
	return m
}
