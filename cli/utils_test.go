package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func setupMockStdin(input string) func() {
	originalStdin := stdin
	stdin = bufio.NewReader(strings.NewReader(input))
	return func() { stdin = originalStdin }
}

func TestPrompt(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		question       string
		defaultValue   string
		options        []string
		expected       string
		expectedPrompt string
	}{
		{
			name:           "basic prompt with response",
			input:          "test response\n",
			question:       "Test question",
			defaultValue:   "default",
			expected:       "test response",
			expectedPrompt: "Test question [default]: ",
		},
		{
			name:           "prompt with default value but no input",
			input:          "\n",
			question:       "Test question",
			defaultValue:   "default",
			expected:       "default",
			expectedPrompt: "Test question [default]: ",
		},
		{
			name:           "prompt with options and matching input",
			input:          "YES\n",
			question:       "Test question",
			defaultValue:   "default",
			options:        []string{"yes", "no"},
			expected:       "YES",
			expectedPrompt: "Test question (DEFAULT/yes/no): ",
		},
		{
			name:           "prompt with options and default not in options",
			input:          "\n",
			question:       "Test question",
			defaultValue:   "default",
			options:        []string{"yes", "no"},
			expected:       "default",
			expectedPrompt: "Test question (DEFAULT/yes/no): ",
		},
		{
			name:           "prompt with default value not in options",
			input:          "\n",
			question:       "Test question",
			defaultValue:   "yes",
			options:        []string{"no"},
			expected:       "yes",
			expectedPrompt: "Test question (YES/no): ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := setupMockStdin(tt.input)
			defer restore()

			var buf bytes.Buffer
			result := prompt2(&buf, tt.question, tt.defaultValue, tt.options...)

			if result != tt.expected {
				t.Errorf("prompt(%q, %q, %v) = %q; expected %q", tt.question, tt.defaultValue, tt.options, result, tt.expected)
			}
			promptOutput := buf.String()
			if promptOutput != tt.expectedPrompt {
				t.Errorf("prompt output = %q; expected %q", promptOutput, tt.expectedPrompt)
			}
		})
	}
}

func TestSplitFunc(t *testing.T) {
	tests := []struct {
		input    rune
		expected bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{',', true},
		{';', true},
		{'a', false},
		{'A', false},
		{'1', false},
	}

	for _, test := range tests {
		result := SplitFunc(test.input)
		if result != test.expected {
			t.Errorf("SplitFunc(%c): expected %v, got %v", test.input, test.expected, result)
		}
	}
}

func TestReadLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "read single line",
			input:    "line1\n",
			expected: "line1",
		},
		{
			name:     "read multiple lines",
			input:    "line1\nline2\n",
			expected: "line1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock stdin
			restore := setupMockStdin(tt.input)
			defer restore()

			result := readLine()
			if result != tt.expected {
				t.Errorf("readLine() = %q; expected %q", result, tt.expected)
			}
		})
	}
}
