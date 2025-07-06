package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"
)

var stdin *bufio.Reader

func readLine() string {
	if stdin == nil {
		stdin = bufio.NewReader(os.Stdin)
	}

	str, _ := stdin.ReadString('\n')
	return strings.TrimSpace(str)
}

func prompt(question, defaultValue string, options ...string) string {
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

	fmt.Printf("%s%s: ", question, suffix)
	response := readLine()
	if response == "" {
		return defaultValue
	}
	return response
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
