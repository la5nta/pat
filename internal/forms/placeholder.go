package forms

import (
	"regexp"

	"github.com/la5nta/pat/internal/debug"
)

// placeholderReplacer returns a function that performs a case-insensitive search and replace.
// The placeholders are expected to be encapsulated with prefix and suffix.
// Any whitespace between prefix/suffix and key is ignored.
func placeholderReplacer(prefix, suffix string, fields map[string]string) func(string) string {
	const (
		space               = `\s*`
		caseInsensitiveFlag = `(?i)`
	)
	// compileRegexp compiles a case insensitive regular expression matching the given key.
	prefix, suffix = regexp.QuoteMeta(prefix)+space, space+regexp.QuoteMeta(suffix)
	compileRegexp := func(key string) *regexp.Regexp {
		return regexp.MustCompile(caseInsensitiveFlag + prefix + regexp.QuoteMeta(key) + suffix)
	}
	// Build a map from regexp to replacement values for all tags.
	regexps := make(map[*regexp.Regexp]string, len(fields))
	for key, newValue := range fields {
		regexps[compileRegexp(key)] = newValue
	}
	// Return a function for applying the replacements.
	return func(str string) string {
		for re, newValue := range regexps {
			str = re.ReplaceAllLiteralString(str, newValue)
		}
		if debug.Enabled() {
			// Log remaining insertion tags
			re := caseInsensitiveFlag + prefix + `[\w_-]+` + suffix
			if matches := regexp.MustCompile(re).FindAllString(str, -1); len(matches) > 0 {
				debug.Printf("Unhandled placeholder: %v", matches)
			}
		}
		return str
	}
}
