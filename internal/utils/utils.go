package utils

import (
	"strings"
	"unicode"
)

func StripLeadingComments(sql string) string {
	s := strings.TrimLeftFunc(sql, unicode.IsSpace)
	for {
		switch {
		case strings.HasPrefix(s, "/*"):
			end := strings.Index(s[2:], "*/")
			if end < 0 {
				return s
			}
			s = strings.TrimLeftFunc(s[2+end+2:], unicode.IsSpace)
		case strings.HasPrefix(s, "--"):
			nl := strings.IndexByte(s, '\n')
			if nl < 0 {
				return s
			}
			s = strings.TrimLeftFunc(s[nl+1:], unicode.IsSpace)
		default:
			return s
		}
	}
}
