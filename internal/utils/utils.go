package utils

import (
	"regexp"
	"strings"
	"unicode"
)

var sqlTagFieldRe = regexp.MustCompile(`^([a-z][a-z0-9_]*)=([A-Za-z0-9._:@/+-]+)$`)

// ParseSQLTags strips a leading SQL block comment from sql and parses any
// key=value tags it holds. A leading /* ... */ comment is always removed from
// the returned SQL, even when it carries no valid tags (then the map is nil).
// SQL with no leading block comment, or an unterminated one, is returned
// unchanged with a nil map.
func ParseSQLTags(sql string) (map[string]string, string) {
	rest := strings.TrimLeftFunc(sql, unicode.IsSpace)
	if !strings.HasPrefix(rest, "/*") {
		return nil, sql
	}

	end := strings.Index(rest[2:], "*/")
	if end < 0 {
		return nil, sql
	}

	stripped := strings.TrimLeftFunc(rest[2+end+2:], unicode.IsSpace)

	return parseTagFields(rest[2 : 2+end]), stripped
}

// parseTagFields returns the key=value tags in a comment body, or nil if the
// body is empty, a field is malformed, or a key repeats.
func parseTagFields(body string) map[string]string {
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return nil
	}

	tags := make(map[string]string, len(fields))
	for _, field := range fields {
		m := sqlTagFieldRe.FindStringSubmatch(field)
		if m == nil {
			return nil
		}
		if _, dup := tags[m[1]]; dup {
			return nil
		}
		tags[m[1]] = m[2]
	}

	return tags
}
