package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findRule returns the first matching finding and whether it exists.
func findRule(t *testing.T, report *Report, rule string) (Finding, bool) {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.Rule == rule {
			return finding, true
		}
	}
	return Finding{}, false
}

// requireRule asserts the rule fires exactly once and returns the finding.
func requireRule(t *testing.T, report *Report, rule string) Finding {
	t.Helper()
	matches := 0
	var found Finding
	for _, finding := range report.Findings {
		if finding.Rule == rule {
			matches++
			found = finding
		}
	}
	require.Equal(t, 1, matches, "rule %s expected to fire once", rule)
	return found
}

// assertNoRule asserts the rule never fires.
func assertNoRule(t *testing.T, report *Report, rule string) {
	t.Helper()
	_, ok := findRule(t, report, rule)
	assert.False(t, ok, "rule %s unexpectedly fired", rule)
}

// TestRule_FullTableScan covers the high-severity no-WHERE/no-LIMIT case and
// verifies the two result-bound rules are mutually exclusive.
func TestRule_FullTableScan(t *testing.T) {
	report, err := Check("SELECT id FROM users")
	require.NoError(t, err)

	finding := requireRule(t, report, RuleFullTableScan)
	assert.Equal(t, SeverityHigh, finding.Severity)
	assert.Equal(t, 1, finding.Statement)
	assert.Contains(t, finding.Fragment, "SELECT id FROM users")
	assert.Equal(t, 0, finding.Start)
	assertNoRule(t, report, RuleUnboundedResult)

	require.Equal(t, 1, report.HighCount)
	require.Zero(t, report.MediumCount)
}

// TestRule_UnboundedResult covers the medium-severity WHERE-without-LIMIT case.
func TestRule_UnboundedResult(t *testing.T) {
	report, err := Check("SELECT id FROM users WHERE id > 10")
	require.NoError(t, err)

	finding := requireRule(t, report, RuleUnboundedResult)
	assert.Equal(t, SeverityMedium, finding.Severity)
	assertNoRule(t, report, RuleFullTableScan)
	assert.Zero(t, report.HighCount)
	require.Equal(t, 1, report.MediumCount)
}

// TestRule_LimitSuppressesBoundsRules verifies LIMIT calms both bound rules.
func TestRule_LimitSuppressesBoundsRules(t *testing.T) {
	report, err := Check("SELECT id FROM users LIMIT 100")
	require.NoError(t, err)
	assertNoRule(t, report, RuleFullTableScan)
	assertNoRule(t, report, RuleUnboundedResult)

	report, err = Check("SELECT id FROM users WHERE id > 10 LIMIT 100")
	require.NoError(t, err)
	assertNoRule(t, report, RuleFullTableScan)
	assertNoRule(t, report, RuleUnboundedResult)
}

// TestRule_StarProjection covers plain and qualified stars, and their offsets.
func TestRule_StarProjection(t *testing.T) {
	report, err := Check("SELECT * FROM users")
	require.NoError(t, err)
	finding := requireRule(t, report, RuleStarProjection)
	assert.Equal(t, SeverityMedium, finding.Severity)
	assert.Equal(t, "*", finding.Fragment)
	assert.Equal(t, 7, finding.Start)
	assert.Contains(t, finding.Message, "星号")

	report, err = Check("SELECT u.* FROM users u")
	require.NoError(t, err)
	finding = requireRule(t, report, RuleStarProjection)
	assert.Equal(t, "u.*", finding.Fragment)
	assert.Equal(t, 7, finding.Start)
}

// TestRule_StarAndBoundsCoexist verifies distinct rules on one query all fire
// and the report totals match the listed findings.
func TestRule_StarAndBoundsCoexist(t *testing.T) {
	report, err := Check("SELECT * FROM users")
	require.NoError(t, err)
	requireRule(t, report, RuleFullTableScan)
	requireRule(t, report, RuleStarProjection)
	assert.Equal(t, 1, report.HighCount)
	assert.Equal(t, 1, report.MediumCount)
	assert.Zero(t, report.InfoCount)
	assert.Len(t, report.Findings, report.HighCount+report.MediumCount+report.InfoCount)
}

// TestRules_IgnoreNonSelect verifies INSERT/UPDATE/DELETE never trigger the
// query rules even when their text lacks WHERE/LIMIT.
func TestRules_IgnoreNonSelect(t *testing.T) {
	cases := []string{
		"INSERT INTO users (id, name) VALUES (1, 'a')",
		"UPDATE users SET name = 'a'",
		"DELETE FROM users",
	}
	for _, sql := range cases {
		report, err := Check(sql)
		require.NoError(t, err)
		assert.Empty(t, report.Findings, sql)
	}
}
