package lint

import (
	"errors"
	"strconv"
	"strings"

	"github.com/valkdb/postgresparser"
)

// finalize sorts findings deterministically and derives the severity counts
// so the totals can never drift away from the listed findings.
func finalize(findings []Finding, statementIndex int) *Report {
	sortFindings(findings)

	report := &Report{Findings: findings}
	for _, finding := range findings {
		switch finding.Severity {
		case SeverityHigh:
			report.HighCount++
		case SeverityMedium:
			report.MediumCount++
		case SeverityInfo:
			report.InfoCount++
		}
	}
	return report
}

// statementParseError turns a failed batch entry back into an error. ParseSQLAll
// retains syntax problems as warnings rather than Go errors, so they are
// surfaced here instead of being hidden behind an empty report.
func statementParseError(stmt postgresparser.StatementParseResult) error {
	messages := make([]string, 0, len(stmt.Warnings))
	for _, warning := range stmt.Warnings {
		messages = append(messages, warning.Message)
	}
	if len(messages) == 0 {
		messages = append(messages, "statement failed to parse")
	}
	return errors.New("lint: statement " + strconv.Itoa(stmt.Index) + " failed to parse: " + strings.Join(messages, "; "))
}
