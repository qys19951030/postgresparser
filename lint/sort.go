package lint

import "sort"

// severityRank orders severities for stable output: high first, then medium,
// then info. Unknown values sort last.
func severityRank(severity Severity) int {
	switch severity {
	case SeverityHigh:
		return 0
	case SeverityMedium:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}

// sortFindings orders findings by severity, then statement index, then
// fragment start offset, and finally rule name so equal offsets never depend
// on map iteration order.
func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if rankA, rankB := severityRank(a.Severity), severityRank(b.Severity); rankA != rankB {
			return rankA < rankB
		}
		if a.Statement != b.Statement {
			return a.Statement < b.Statement
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Rule < b.Rule
	})
}
