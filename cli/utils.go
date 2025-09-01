package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
)

var stdin *bufio.Reader

func readLine() string {
	if stdin == nil {
		stdin = bufio.NewReader(os.Stdin)
	}

	str, _ := stdin.ReadString('\n')
	return strings.TrimSpace(str)
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return true // Fail-safe
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func prompt2(w io.Writer, question, defaultValue string, options ...string) string {
	var suffix string
	if len(options) > 0 {
		// Ensure default is included in options if not already present
		allOptions := options
		defaultFound := false
		for _, opt := range options {
			if strings.EqualFold(opt, defaultValue) {
				defaultFound = true
				break
			}
		}
		if !defaultFound && defaultValue != "" {
			allOptions = append([]string{defaultValue}, options...)
		}

		// Use standard (Y/n) format where uppercase indicates default
		formatted := make([]string, len(allOptions))
		for i, opt := range allOptions {
			if strings.EqualFold(opt, defaultValue) {
				formatted[i] = strings.ToUpper(opt)
			} else {
				formatted[i] = strings.ToLower(opt)
			}
		}
		suffix = fmt.Sprintf(" (%s)", strings.Join(formatted, "/"))
	} else if defaultValue != "" {
		// Free-text field with default value
		suffix = fmt.Sprintf(" [%s]", defaultValue)
	}

	fmt.Fprintf(w, "%s%s: ", question, suffix)
	response := readLine()
	if response == "" {
		return defaultValue
	}
	return response
}

func prompt(question, defaultValue string, options ...string) string {
	return prompt2(os.Stdout, question, defaultValue, options...)
}

func SplitFunc(c rune) bool {
	return unicode.IsSpace(c) || c == ',' || c == ';'
}

func exitOnContextCancellation(ctx context.Context) (cancel func()) {
	done := make(chan struct{}, 1)
	go func() {
		select {
		case <-ctx.Done():
			fmt.Println()
			os.Exit(1)
		case <-done:
		}
	}()
	return func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}
}

func openMessage(a *app.App, path string) (*fbb.Message, error) {
	// Search if only MID is specified.
	if filepath.Dir(path) == "." && filepath.Ext(path) == "" {
		debug.Printf("openMessage(%q): Searching...", path)
		path += mailbox.Ext
		fs.WalkDir(os.DirFS(a.Mailbox().MBoxPath), ".", func(p string, d fs.DirEntry, err error) error {
			if d.Name() != path {
				return nil
			}
			debug.Printf("openMessage(%q): Found %q", d.Name(), p)
			path = filepath.Join(a.Mailbox().MBoxPath, p)
			return io.EOF
		})
	}
	return mailbox.OpenMessage(path)
}
