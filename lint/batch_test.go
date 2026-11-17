package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckAll_Attribution verifies each finding is attributed to the correct
// one-based statement index and uses that statement's own raw text.
func TestCheckAll_Attribution(t *testing.T) {
	sql := "SELECT * FROM users; INSERT INTO users (id) VALUES (1); SELECT id FROM orders WHERE id > 5"
	batch, err := CheckAll(sql)
	require.NoError(t, err)
	require.Len(t, batch.Reports, 3)

	// Statement 1: star + full table scan.
	first := batch.Reports[0]
	require.NotEmpty(t, first.Findings)
	for _, finding := range first.Findings {
		assert.Equal(t, 1, finding.Statement)
		assert.Contains(t, "SELECT * FROM users", finding.Fragment)
	}
	requireRule(t, first, RuleStarProjection)
	requireRule(t, first, RuleFullTableScan)

	// Statement 2: INSERT triggers no query rules.
	assert.Empty(t, batch.Reports[1].Findings)

	// Statement 3: bounded result with WHERE but no LIMIT.
	third := batch.Reports[2]
	finding := requireRule(t, third, RuleUnboundedResult)
	assert.Equal(t, 3, finding.Statement)
	assert.Contains(t, "SELECT id FROM orders WHERE id > 5", finding.Fragment)
}

// TestCheckAll_Totals verifies aggregate counts equal the sum of per-report
// counts and the number of listed findings.
func TestCheckAll_Totals(t *testing.T) {
	sql := "SELECT * FROM users; SELECT id FROM orders WHERE id > 5; DELETE FROM users WHERE id = 1"
	batch, err := CheckAll(sql)
	require.NoError(t, err)
	require.Len(t, batch.Reports, 3)

	var high, medium, info int
	listed := 0
	for _, report := range batch.Reports {
		high += report.HighCount
		medium += report.MediumCount
		info += report.InfoCount
		listed += len(report.Findings)
		assert.Len(t, report.Findings, report.HighCount+report.MediumCount+report.InfoCount)
	}
	assert.Equal(t, high, batch.HighCount)
	assert.Equal(t, medium, batch.MediumCount)
	assert.Equal(t, info, batch.InfoCount)
	assert.Equal(t, listed, batch.HighCount+batch.MediumCount+batch.InfoCount)
}

// TestCheckAll_StableOrder verifies findings within a report stay ordered by
// severity then fragment start, and statements stay in source order.
func TestCheckAll_StableOrder(t *testing.T) {
	sql := "SELECT a.id FROM a CROSS JOIN b ORDER BY a.id; SELECT * FROM t WHERE x = 1"
	batch, err := CheckAll(sql)
	require.NoError(t, err)
	require.Len(t, batch.Reports, 2)

	first := batch.Reports[0]
	require.GreaterOrEqual(t, len(first.Findings), 2)
	for i := 1; i < len(first.Findings); i++ {
		prev, cur := first.Findings[i-1], first.Findings[i]
		prevRank, curRank := severityRank(prev.Severity), severityRank(cur.Severity)
		assert.LessOrEqual(t, prevRank, curRank)
		if prevRank == curRank {
			assert.LessOrEqual(t, prev.Start, cur.Start)
		}
		assert.Equal(t, 1, prev.Statement)
		assert.Equal(t, 1, cur.Statement)
	}
	for _, finding := range batch.Reports[1].Findings {
		assert.Equal(t, 2, finding.Statement)
	}
}

// TestCheckAll_ParseFailure verifies a batch whose statements carry parser
// warnings is returned as an error rather than hidden behind empty reports.
func TestCheckAll_ParseFailure(t *testing.T) {
	batch, err := CheckAll("SELECT * FROM ((")
	require.Error(t, err)
	assert.Nil(t, batch)
}

// TestCheckAll_StatementWarning verifies a recovered statement with a parser
// warning is surfaced as an error instead of a fabricated empty report.
func TestCheckAll_StatementWarning(t *testing.T) {
	batch, err := CheckAll("SELECT id FROM a; INSERT INTO; SELECT id FROM b")
	require.Error(t, err)
	assert.Nil(t, batch)
	assert.Contains(t, err.Error(), "failed to parse")
}

// TestCheckAll_EmptyInput verifies empty input propagates the parser error.
func TestCheckAll_EmptyInput(t *testing.T) {
	batch, err := CheckAll("   ")
	require.Error(t, err)
	assert.Nil(t, batch)
}

// TestCheck_SingleStatementIndex verifies Check always uses index one.
func TestCheck_SingleStatementIndex(t *testing.T) {
	report, err := Check("SELECT * FROM users")
	require.NoError(t, err)
	for _, finding := range report.Findings {
		assert.Equal(t, 1, finding.Statement)
	}
}
