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
	Name            string `json:"name"`
	TxtFileURI      string `json:"txt_file_uri"`
	InitialURI      string `json:"initial_uri"`
	ViewerURI       string `json:"viewer_uri"`
	ReplyTxtFileURI string `json:"reply_txt_file_uri"`
	ReplyInitialURI string `json:"reply_initial_uri"`
	ReplyViewerURI  string `json:"reply_viewer_uri"`
}

func readTemplate(path string, filesMap formFilesMap) (Template, error) {
	f, err := os.Open(path)
	if err != nil {
		return Template{}, err
	}
	defer f.Close()

	template := Template{
		Name:       strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		TxtFileURI: path,
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
				if ext := filepath.Ext(name); ext == "" {
					name += htmlFileExt
				}
				files[i] = resolveFileReference(filesMap, filepath.Dir(path), name)
				if files[i] == "" {
					debug.Printf("%s: failed to resolve referenced file %q", template.TxtFileURI, name)
				}
			}
			template.InitialURI = files[0]
			if len(files) > 1 {
				template.ViewerURI = files[1]
			}
		case "ReplyTemplate":
			name := strings.TrimSpace(value)
			// Some are missing file extension (default to .txt)
			if filepath.Ext(name) == "" {
				name += txtFileExt
			}
			path := resolveFileReference(filesMap, filepath.Dir(path), name)
			if path == "" {
				debug.Printf("%s: failed to resolve referenced reply template file %q", template.TxtFileURI, name)
				continue
			}
			replyTemplate, err := readTemplate(path, filesMap)
			if err != nil {
				debug.Printf("%s: failed to load referenced reply template: %v", template.TxtFileURI, err)
			}
			template.ReplyTxtFileURI = path
			template.ReplyInitialURI = replyTemplate.InitialURI
			template.ReplyViewerURI = replyTemplate.ViewerURI
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
	debug.Printf("fallback to map based lookup of %q", filepath.Join(filepath.Base(basePath), referencePath))
	return filesMap.get(referencePath)
}

func (t Template) matchesName(nameToMatch string) bool {
	return t.InitialURI == nameToMatch ||
		strings.EqualFold(t.InitialURI, nameToMatch+htmlFileExt) ||
		t.ViewerURI == nameToMatch ||
		strings.EqualFold(t.ViewerURI, nameToMatch+htmlFileExt) ||
		t.ReplyInitialURI == nameToMatch ||
		t.ReplyInitialURI == nameToMatch+".0" ||
		t.ReplyViewerURI == nameToMatch ||
		t.ReplyViewerURI == nameToMatch+".0" ||
		t.TxtFileURI == nameToMatch ||
		strings.EqualFold(t.TxtFileURI, nameToMatch+txtFileExt)
}

func (t Template) containsName(partialName string) bool {
	return strings.Contains(t.InitialURI, partialName) ||
		strings.Contains(t.ViewerURI, partialName) ||
		strings.Contains(t.ReplyInitialURI, partialName) ||
		strings.Contains(t.ReplyViewerURI, partialName) ||
		strings.Contains(t.ReplyTxtFileURI, partialName) ||
		strings.Contains(t.TxtFileURI, partialName)
}

type formFilesMap map[string]string

func (m formFilesMap) get(name string) string {
	name = strings.ToLower(name)
	if path, ok := m[name]; ok {
		return path
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
		return strings.EqualFold(filepath.Ext(name), ".0")
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
