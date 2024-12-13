package editor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/la5nta/pat/internal/buildinfo"
)

func Executable() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	} else if e := os.Getenv("VISUAL"); e != "" {
		return e
	}

	switch runtime.GOOS {
	case "windows":
		return "notepad"
	case "linux":
		if path, err := exec.LookPath("editor"); err == nil {
			return path
		}
	}

	return "vi"
}

func Open(path string) error {
	cmd := exec.Command(Executable(), path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func EditText(template string) (string, error) {
	f, err := os.CreateTemp("", strings.ToLower(buildinfo.AppName)+"_edit_*.txt")
	if err != nil {
		return template, fmt.Errorf("Unable to prepare temporary file for body: %w", err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	f.Write([]byte(template))
	f.Sync()

	// Windows fix: Avoid 'cannot access the file because it is being used by another process' error.
	// Close the file before opening the editor.
	f.Close()

	// Fire up the editor
	if err := Open(f.Name()); err != nil {
		return template, fmt.Errorf("Unable to start text editor: %w", err)
	}

	// Read back the edited file
	f, err = os.OpenFile(f.Name(), os.O_RDWR, 0o666)
	if err != nil {
		return template, fmt.Errorf("Unable to read temporary file from editor: %w", err)
	}
	defer f.Close()
	defer os.Remove(f.Name())
	body, err := io.ReadAll(f)
	return string(body), err
}
