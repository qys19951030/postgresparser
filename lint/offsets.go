package lint

import "strings"

// resolveOffsets converts findings positioned relative to their producing
// query node into statement-level findings with absolute byte offsets inside
// the statement raw text.
func (c *collector) resolveOffsets() []Finding {
	findings := make([]Finding, 0, len(c.raw))
	for _, raw := range c.raw {
		start := raw.relStart
		if raw.queryText != c.rootText {
			if base := indexWithin(c.rootText, raw.queryText); base >= 0 {
				start = base + raw.relStart
			} else {
				// The nested raw text is not recoverable verbatim (whitespace
				// normalisation etc.); anchor the fragment to the statement.
				start = 0
			}
		}
		findings = append(findings, Finding{
			Rule:      raw.rule,
			Severity:  raw.severity,
			Message:   raw.message,
			Fragment:  raw.fragment,
			Start:     start,
			Statement: c.statementIndex,
		})
	}
	return findings
}

// indexWithin locates scope inside the statement text, tolerating whitespace
// differences introduced by IR text reconstruction.
func indexWithin(statementText, scope string) int {
	if scope == "" {
		return -1
	}
	if offset := strings.Index(statementText, scope); offset >= 0 {
		return offset
	}
	trimmed := strings.TrimSpace(scope)
	if trimmed == "" || trimmed == scope {
		return -1
	}
	return strings.Index(statementText, trimmed)
}
