package logs

import (
	"regexp"
	"strings"
)

func rotatedLogMatcher(logFilename string) (*regexp.Regexp, error) {
	stem := strings.TrimSuffix(logFilename, ".log")

	var b strings.Builder
	b.WriteString(`^`)
	for i := 0; i < len(stem); i++ {
		if stem[i] == '%' && i+1 < len(stem) {
			i++
			b.WriteString(strftimeVerbRegex(stem[i]))

			continue
		}
		b.WriteString(regexp.QuoteMeta(stem[i : i+1]))
	}
	b.WriteString(`\.(?:json|log)$`)

	return regexp.Compile(b.String())
}

func strftimeVerbRegex(verb byte) string {
	switch verb {
	case '%':
		return `%`
	case 'a', 'A', 'b', 'B', 'h', 'p', 'P', 'Z':
		return `[A-Za-z]+`
	default:
		return `\d+`
	}
}
