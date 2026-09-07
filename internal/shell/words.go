package shell

import (
	"fmt"
	"strings"
	"unicode"
)

// Words handles quotes and backslash escapes, but never expansion or execution.
func Words(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, c := range line {
		if escaped {
			word.WriteRune(c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			continue
		}
		switch {
		case c == '\'' || c == '"':
			quote = c
			started = true
		case unicode.IsSpace(c):
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		case c == '#' && !started:
			return words, nil
		case strings.ContainsRune("|;&<>", c):
			return nil, fmt.Errorf("shell operators are not supported; quote literal characters")
		default:
			word.WriteRune(c)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unfinished quote or escape")
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}
