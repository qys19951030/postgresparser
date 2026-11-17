package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcceptance_BoundsMutualExclusion covers acceptance item 1: no WHERE/no
// LIMIT is high, WHERE-without-LIMIT is medium, and the two never coexist or
// replace each other incorrectly.
func TestAcceptance_BoundsMutualExclusion(t *testing.T) {
	high, err := Check("SELECT id FROM users")
	require.NoError(t, err)
	requireRule(t, high, RuleFullTableScan)
	assert.Equal(t, SeverityHigh, high.Findings[0].Severity)
	assertNoRule(t, high, RuleUnboundedResult)

	medium, err := Check("SELECT id FROM users WHERE id = 1")
	require.NoError(t, err)
	requireRule(t, medium, RuleUnboundedResult)
	assertNoRule(t, medium, RuleFullTableScan)
	for _, finding := range medium.Findings {
		if finding.Rule == RuleUnboundedResult {
			assert.Equal(t, SeverityMedium, finding.Severity)
		}
	}
}

// TestAcceptance_SeveritiesAndMessages covers acceptance item 2: star is
// medium, condition-less join is high, order-without-limit is info, each with
// the expected explanatory text.
func TestAcceptance_SeveritiesAndMessages(t *testing.T) {
	star, err := Check("SELECT * FROM t")
	require.NoError(t, err)
	f := requireRule(t, star, RuleStarProjection)
	assert.Equal(t, SeverityMedium, f.Severity)
	assert.Contains(t, f.Message, "字段没有收敛")

	join, err := Check("SELECT a.id FROM a CROSS JOIN b")
	require.NoError(t, err)
	f = requireRule(t, join, RuleCartesianJoin)
	assert.Equal(t, SeverityHigh, f.Severity)
	assert.Contains(t, f.Message, "笛卡尔积")

	order, err := Check("SELECT id FROM t ORDER BY id")
	require.NoError(t, err)
	f = requireRule(t, order, RuleOrderWithoutLimit)
	assert.Equal(t, SeverityInfo, f.Severity)
	assert.Contains(t, f.Message, "排序开销")
}

// TestAcceptance_NonSelectAndDepth covers acceptance item 3: DML statements
// skip query rules, and deep subquery nesting yields a single info reminder.
func TestAcceptance_NonSelectAndDepth(t *testing.T) {
	for _, sql := range []string{
		"INSERT INTO t (id) VALUES (1)",
		"UPDATE t SET id = 2",
		"DELETE FROM t WHERE id = 3",
	} {
		report, err := Check(sql)
		require.NoError(t, err)
		assert.Empty(t, report.Findings, sql)
	}

	deep := "SELECT id FROM (SELECT id FROM (SELECT id FROM (SELECT id FROM (SELECT id FROM t) a) b) c) d"
	report, err := Check(deep)
	require.NoError(t, err)
	f := requireRule(t, report, RuleSubqueryDepth)
	assert.Equal(t, SeverityInfo, f.Severity)
	assert.Equal(t, 1, report.InfoCount)
}

// TestAcceptance_BatchAttribution covers acceptance item 4: per-statement
// indices, statement-local fragments, stable ordering, and totals that match
// the listed findings.
func TestAcceptance_BatchAttribution(t *testing.T) {
	sql := "SELECT * FROM users; " +
		"UPDATE users SET name = 'x' WHERE id = 1; " +
		"SELECT a.id FROM a CROSS JOIN b; " +
		"SELECT id FROM orders WHERE id > 5 ORDER BY id"
	batch, err := CheckAll(sql)
	require.NoError(t, err)
	require.Len(t, batch.Reports, 4)

	// Statement indices are correct and fragments stay within each statement.
	stmtTexts := []string{
		"SELECT * FROM users",
		"UPDATE users SET name = 'x' WHERE id = 1",
		"SELECT a.id FROM a CROSS JOIN b",
		"SELECT id FROM orders WHERE id > 5 ORDER BY id",
	}
	for i, report := range batch.Reports {
		for _, finding := range report.Findings {
			assert.Equal(t, i+1, finding.Statement)
			assert.Contains(t, stmtTexts[i], finding.Fragment)
		}
	}
	assert.Empty(t, batch.Reports[1].Findings, "UPDATE must be clean")

	// Stable severity ordering within every report.
	for _, report := range batch.Reports {
		for j := 1; j < len(report.Findings); j++ {
			assert.LessOrEqual(t,
				severityRank(report.Findings[j-1].Severity),
				severityRank(report.Findings[j].Severity))
		}
	}

	// Totals reconcile with the listed findings.
	var total int
	for _, report := range batch.Reports {
		total += len(report.Findings)
		assert.Len(t, report.Findings, report.HighCount+report.MediumCount+report.InfoCount)
	}
	assert.Equal(t, total, batch.HighCount+batch.MediumCount+batch.InfoCount)
}
