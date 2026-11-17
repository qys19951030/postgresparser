// Package lint provides static pre-deployment review of SQL statements.
//
// The checks operate solely on the intermediate representation produced by
// the postgresparser package: no rule ever matches against the raw SQL text.
// Reports are read-only values. The package keeps no mutable global state and
// performs no caching, so concurrent calls are independent.
package lint

import (
	"github.com/valkdb/postgresparser"
)

// Severity is the risk level attached to a Finding.
type Severity string

const (
	// SeverityHigh flags findings that must be resolved before deployment.
	SeverityHigh Severity = "high"
	// SeverityMedium flags findings that should be reviewed before deployment.
	SeverityMedium Severity = "medium"
	// SeverityInfo flags advisory findings that do not block deployment.
	SeverityInfo Severity = "info"
)

// Rule names emitted by the linter. They are stable identifiers intended for
// filtering and suppression, not display text.
const (
	// RuleFullTableScan fires when a query has neither WHERE nor LIMIT.
	RuleFullTableScan = "full-table-scan"
	// RuleUnboundedResult fires when a query has WHERE but no LIMIT.
	RuleUnboundedResult = "unbounded-result"
	// RuleStarProjection fires when a query selects columns via *.
	RuleStarProjection = "star-projection"
	// RuleCartesianJoin fires when a join has no join condition.
	RuleCartesianJoin = "cartesian-join"
	// RuleOrderWithoutLimit fires when ORDER BY is not bounded by LIMIT.
	RuleOrderWithoutLimit = "order-without-limit"
	// RuleSubqueryDepth fires once when subqueries nest more than three levels.
	RuleSubqueryDepth = "subquery-depth"
)

// Finding is one lint result. All fields are read-only.
type Finding struct {
	// Rule is the stable rule name that produced the finding.
	Rule string
	// Severity is the risk level of the finding.
	Severity Severity
	// Message is the human-readable explanation of the problem.
	Message string
	// Fragment is the piece of the originating statement's raw text that
	// exhibits the problem.
	Fragment string
	// Start is the zero-based byte offset of Fragment inside the statement
	// raw text.
	Start int
	// Statement is the one-based index of the statement inside a batch.
	// Check always reports 1.
	Statement int
}

// Report is the review result for one statement. Findings is sorted by
// severity (high, then medium, then info) and, within one severity, by
// statement index and fragment start offset. The count fields always match
// the number of findings at each severity.
type Report struct {
	Findings    []Finding
	HighCount   int
	MediumCount int
	InfoCount   int
}

// BatchReport is the review result for a multi-statement batch. Reports holds
// one Report per input statement in source order, and the count fields are
// the totals across all of them.
type BatchReport struct {
	Reports     []*Report
	HighCount   int
	MediumCount int
	InfoCount   int
}

// Check reviews one SQL string and returns its report. Parse failures are
// returned as the parser's original error rather than a fabricated empty
// report.
func Check(sql string) (*Report, error) {
	query, err := postgresparser.ParseSQL(sql)
	if err != nil {
		return nil, err
	}
	return buildReport(query, 1), nil
}

// CheckAll reviews every statement in a multi-statement SQL string and
// returns one report per statement plus aggregated totals. A statement that
// fails to parse aborts the call with the parser's error surfaced verbatim;
// no partial, empty report is fabricated for it.
func CheckAll(sql string) (*BatchReport, error) {
	batch, err := postgresparser.ParseSQLAll(sql)
	if err != nil {
		return nil, err
	}

	reports := make([]*Report, len(batch.Statements))
	out := &BatchReport{Reports: reports}
	for i, stmt := range batch.Statements {
		if stmt.Query == nil {
			return nil, statementParseError(stmt)
		}
		report := buildReport(stmt.Query, i+1)
		reports[i] = report
		out.HighCount += report.HighCount
		out.MediumCount += report.MediumCount
		out.InfoCount += report.InfoCount
	}
	return out, nil
}
