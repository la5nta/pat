package porta_paths

import (
	"github.com/la5nta/pat/internal/directories"
	"os"
	"path/filepath"
	"runtime"
)

var rootPath = ""

func SetRootPath(path string) {
	rootPath = path
}

func GetRootPath() string {
	if len(rootPath) < 1 {
		return "/data/data/aq.metallists.portaparty/files"
	}
	return rootPath
}

type pathKind uint8

const (
	PATH_KIND_CONFIG = pathKind(iota + 1)
	PATH_KIND_MAILBOX
	PATH_KIND_FORMS
	PATH_KIND_PREHOOKS
	PATH_KIND_LOG
	PATH_KIND_EVENTLOG
	PATH_KIND_DATAROOT
	PATH_KIND_STATEDIR
)

func GetPortaPath(kind pathKind) string {
	path := GetPortaPathReal(kind)

	switch kind {
	case PATH_KIND_CONFIG, PATH_KIND_LOG, PATH_KIND_EVENTLOG:
		return path
	default:
		if _, err := os.Stat(path); os.IsNotExist(err) {
			os.MkdirAll(path, 0700)
		}
	}

	return path
}

func GetPortaPathReal(kind pathKind) string {
	switch kind {
	case PATH_KIND_CONFIG:
		return filepath.Join(GetRootPath(), "config.json")
	case PATH_KIND_MAILBOX:
		return filepath.Join(GetRootPath(), "mailbox")
	case PATH_KIND_FORMS:
		return filepath.Join(GetRootPath(), "Standard_Forms")
	case PATH_KIND_PREHOOKS:
		return filepath.Join(GetRootPath(), "prehooks")
	case PATH_KIND_LOG:
		return filepath.Join(GetRootPath(), "applog.log")
	case PATH_KIND_EVENTLOG:
		return filepath.Join(GetRootPath(), "eventlog.json")
	case PATH_KIND_STATEDIR:
		return filepath.Join(GetRootPath(), "state")
	default:
		return GetRootPath()
	}
}

func GetDataDirFor(subpath string) string {
	if runtime.GOOS == "android" {
		return filepath.Join(GetPortaPath(PATH_KIND_DATAROOT), subpath)
	} else {
		return filepath.Join(directories.DataDir(), subpath)
	}
}

func GetStateDirFor(subpath string) string {
	if runtime.GOOS == "android" {
		return filepath.Join(GetPortaPath(PATH_KIND_STATEDIR), subpath)
	} else {
		return filepath.Join(directories.StateDir(), subpath)
	}
}
