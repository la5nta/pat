package forms

import (
	"bufio"
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

func readTemplate(path string) (Template, error) {
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
	baseURI := filepath.Dir(template.TxtFileURI)
	for scanner.Scan() {
		switch key, value, _ := strings.Cut(scanner.Text(), ":"); key {
		case "Form":
			// Form: <composer>,<viewer>
			files := strings.Split(value, ",")
			// Extend to absolute paths and add missing html extension
			for i, path := range files {
				path = strings.TrimSpace(path)
				if ext := filepath.Ext(path); ext == "" {
					path += htmlFileExt
				}
				files[i] = resolveFileReference(baseURI, path)
				if files[i] == "" {
					debug.Printf("%s: failed to resolve referenced file %q", template.TxtFileURI, path)
				}
			}
			template.InitialURI = files[0]
			if len(files) > 1 {
				template.ViewerURI = files[1]
			}
		case "ReplyTemplate":
			path := strings.TrimSpace(value)
			// Some are missing .txt
			if filepath.Ext(path) == "" {
				path += txtFileExt
			}
			path = resolveFileReference(baseURI, path)
			if path == "" {
				debug.Printf("%s: failed to resolve referenced reply template file %q", template.TxtFileURI, path)
				continue
			}
			replyTemplate, err := readTemplate(path)
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

// resolveFileReference searches for files referenced in .txt files.
//
// If found the returned path is the absolute path, otherwise an empty string is returned.
func resolveFileReference(basePath string, referencePath string) string {
	path := filepath.Join(basePath, referencePath)
	if !directories.IsInPath(basePath, path) {
		debug.Printf("%q escapes template's base path (%q)", referencePath, basePath)
		return ""
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	// Fallback to case-insenstive search.
	// Some HTML files references in the .txt files has a different caseness than the actual filename on disk.
	//TODO: Walk basePath tree instead
	absPathTemplateFolder := filepath.Dir(path)
	entries, err := os.ReadDir(absPathTemplateFolder)
	if err != nil {
		return path
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.EqualFold(filepath.Base(path), name) {
			return filepath.Join(absPathTemplateFolder, name)
		}
	}
	return ""
}
