package forms

import (
	"regexp"
	"strings"
)

type Select struct {
	Prompt  string
	Options []Option
}

type Ask struct {
	Prompt    string
	Multiline bool
}

type Option struct {
	Item  string
	Value string
}

func promptAsks(str string, promptFn func(Ask) string) string {
	re := regexp.MustCompile(`(?i)<Ask\s+([^,]+)(,[^>]+)?>`)
	for {
		tokens := re.FindAllStringSubmatch(str, -1)
		if len(tokens) == 0 {
			return str
		}
		replace, prompt, options := tokens[0][0], tokens[0][1], strings.TrimPrefix(tokens[0][2], ",")
		a := Ask{Prompt: prompt, Multiline: strings.EqualFold(options, "MU")}
		ans := promptFn(a)
		str = strings.Replace(str, replace, ans, 1)
	}
}

func promptSelects(str string, promptFn func(Select) Option) string {
	re := regexp.MustCompile(`(?i)<Select\s+([^,]+)(,[^>]+)?>`)
	for {
		tokens := re.FindAllStringSubmatch(str, -1)
		if len(tokens) == 0 {
			return str
		}
		replace, prompt, options := tokens[0][0], tokens[0][1], strings.Split(strings.TrimPrefix(tokens[0][2], ","), ",")
		s := Select{Prompt: prompt}
		for _, opt := range options {
			item, value, ok := strings.Cut(opt, "=")
			if !ok {
				value = item
			}
			s.Options = append(s.Options, Option{Item: item, Value: value})
		}
		ans := promptFn(s)
		str = strings.Replace(str, replace, ans.Value, 1)
	}
}

func promptVars(str string, promptFn func(string) string) string {
	re := regexp.MustCompile(`(?i)<Var\s+(\w+)\s*>`)
	for {
		tokens := re.FindAllStringSubmatch(str, -1)
		if len(tokens) == 0 {
			return str
		}
		replace, key := tokens[0][0], tokens[0][1]
		ans := promptFn(key)
		if ans == "" {
			ans = "blank"
		}
		str = strings.Replace(str, replace, ans, 1)
	}
}
