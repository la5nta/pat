package forms

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
)

// Template holds information about a Winlink template.
type Template struct {
	// The name of this template.
	Name string `json:"name"`

	// Absolute path to the template file represented by this struct.
	//
	// Note: The web gui uses relative paths, and for these instances the
	// value is set accordingly.
	Path string `json:"template_path"`

	// Absolute path to the optional HTML Form composer (aka "input form").
	InputFormPath string `json:"-"`

	// Absolute path to the optional HTML Form viewer (aka "display form").
	DisplayFormPath string `json:"-"`

	// Absolute path to the optional reply template.
	ReplyTemplatePath string `json:"-"`
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

	resolveFileReference := func(kind string, ref string) string {
		if ref == "" {
			return ""
		}
		resolved := resolveFileReference(filesMap, filepath.Dir(template.Path), strings.TrimSpace(ref))
		if resolved == "" {
			debugName, _ := filepath.Rel(filepath.Join(template.Path, "..", ".."), template.Path)
			debug.Printf("%s: failed to resolve referenced %s %q", debugName, kind, ref)
		}
		return resolved
	}
	scanner := bufio.NewScanner(newTrimBomReader(f))
	var isTemplate bool
	for scanner.Scan() {
		switch key, value, _ := strings.Cut(scanner.Text(), ":"); textproto.CanonicalMIMEHeaderKey(key) {
		case "Msg":
			isTemplate = true
		case "Form": // Form: <input form>[,<display form>]
			isTemplate = true
			inputForm, displayForm, _ := strings.Cut(value, ",")
			template.InputFormPath = resolveFileReference("input from", inputForm)
			template.DisplayFormPath = resolveFileReference("display form", displayForm)
		case "Replytemplate": // ReplyTemplate: <template>
			template.ReplyTemplatePath = resolveFileReference("reply template", value)
		}
	}
	if !isTemplate {
		return template, fmt.Errorf("not a template")
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
