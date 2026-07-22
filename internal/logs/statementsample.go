package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	sqltags "github.com/querysheriff/sqltags/go"
)

const paramValuePattern = `(?:(NULL)|'((?:[^']|'')*)')`

type statementSampleExtractor struct {
	autoExplainWithParams *regexp.Regexp
	autoExplainWithCosts  *regexp.Regexp
	autoExplainValues     *regexp.Regexp
	durationParam         *regexp.Regexp
}

func newStatementSampleExtractor() *statementSampleExtractor {
	return &statementSampleExtractor{
		autoExplainWithParams: regexp.MustCompile(
			`^Query Text: ([\s\S]+)\r?\n\s*Query Parameters: (.+)\r?\n\s*([\s\S]+)`,
		),
		autoExplainWithCosts: regexp.MustCompile(
			`^Query Text: ([\s\S]+?)\r?\n\s*([\S ]+  \(cost=\d+\.\d+\.\.\d+\.\d+ rows=\d+ width=\d+\)[\s\S]+)`,
		),
		autoExplainValues: regexp.MustCompile(paramValuePattern),
		durationParam:     regexp.MustCompile(`(?:[Pp]arameters: |, )\$\d+ = ` + paramValuePattern),
	}
}

func unquoteParam(null, inner string) string {
	if null != "" {
		return ""
	}

	return strings.ReplaceAll(inner, "''", "'")
}

type explainJSONHeader struct {
	QueryText       string `json:"Query Text"`
	QueryParameters string `json:"Query Parameters"`
}

// fromAutoExplain converts auto_explain output into a sample, preserving the raw plan verbatim.
func (x *statementSampleExtractor) fromAutoExplain(explainText, runtime string) (StatementSample, error) {
	durationMs, err := strconv.ParseFloat(runtime, 64)
	if err != nil {
		return StatementSample{}, fmt.Errorf("parse auto_explain duration %q: %w", runtime, err)
	}

	var sample StatementSample
	switch {
	case strings.HasPrefix(explainText, "{"):
		sample, err = x.fromExplainJSON(explainText, durationMs)
	case strings.HasPrefix(explainText, "Query Text:"):
		sample, err = x.fromExplainText(explainText, durationMs)
	default:
		return StatementSample{}, errors.New("unsupported auto_explain format")
	}
	if err != nil {
		return StatementSample{}, err
	}

	return attachTags(sample), nil
}

func (x *statementSampleExtractor) fromExplainJSON(explainText string, durationMs float64) (StatementSample, error) {
	var header explainJSONHeader
	if err := json.Unmarshal([]byte(explainText), &header); err != nil {
		return StatementSample{}, err
	}

	return StatementSample{
		Query:           strings.TrimSpace(header.QueryText),
		Parameters:      x.values(header.QueryParameters),
		DurationMs:      durationMs,
		ExplainPlanJSON: explainText,
	}, nil
}

func (x *statementSampleExtractor) fromExplainText(explainText string, durationMs float64) (StatementSample, error) {
	if parts := x.autoExplainWithParams.FindStringSubmatch(explainText); parts != nil {
		return StatementSample{
			Query:           parts[1],
			Parameters:      x.values(parts[2]),
			DurationMs:      durationMs,
			ExplainPlanJSON: parts[3],
		}, nil
	}

	parts := x.autoExplainWithCosts.FindStringSubmatch(explainText)
	if parts == nil {
		return StatementSample{}, errors.New("auto_explain output does not match the expected format")
	}

	return StatementSample{
		Query:           parts[1],
		DurationMs:      durationMs,
		ExplainPlanJSON: parts[2],
	}, nil
}

// fromLogMinDuration converts a log_min_duration_statement line into a sample.
func (x *statementSampleExtractor) fromLogMinDuration(queryText, runtime, step, detail string) (StatementSample, bool) {
	if step == "bind" || step == "parse" {
		return StatementSample{}, false
	}

	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return StatementSample{}, false
	}

	durationMs, err := strconv.ParseFloat(runtime, 64)
	if err != nil {
		return StatementSample{}, false
	}

	sample := StatementSample{Query: queryText, DurationMs: durationMs}

	if strings.HasPrefix(detail, "Parameters: ") || strings.HasPrefix(detail, "parameters: ") {
		for _, m := range x.durationParam.FindAllStringSubmatch(detail, -1) {
			sample.Parameters = append(sample.Parameters, unquoteParam(m[1], m[2]))
		}
	}

	return attachTags(sample), true
}

func attachTags(sample StatementSample) StatementSample {
	sample.Query, sample.Tags = sqltags.Untag(sample.Query)

	return sample
}

func (x *statementSampleExtractor) values(text string) []string {
	var values []string
	for _, m := range x.autoExplainValues.FindAllStringSubmatch(text, -1) {
		values = append(values, unquoteParam(m[1], m[2]))
	}

	return values
}
