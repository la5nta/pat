package cli

import (
	"bufio"
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

func SplitFunc(c rune) bool {
	return unicode.IsSpace(c) || c == ',' || c == ';'
}
