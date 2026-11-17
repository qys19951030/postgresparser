package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRule_CartesianJoin covers CROSS JOIN, the legal form of a join without
// a join condition (a bare JOIN without ON/USING is itself a syntax error).
func TestRule_CartesianJoin(t *testing.T) {
	report, err := Check("SELECT a.id FROM a CROSS JOIN b")
	require.NoError(t, err)
	finding := requireRule(t, report, RuleCartesianJoin)
	assert.Equal(t, SeverityHigh, finding.Severity)
	assert.Equal(t, "b", finding.Fragment)
	assert.Contains(t, finding.Message, "笛卡尔积")
	assert.Equal(t, 2, report.HighCount)
}

// TestRule_JoinWithConditionSuppressed verifies ON and USING calm the rule,
// and so does NATURAL JOIN, whose condition is implicit.
func TestRule_JoinWithConditionSuppressed(t *testing.T) {
	cases := []string{
		"SELECT a.id FROM a JOIN b ON a.id = b.aid",
		"SELECT a.id FROM a LEFT JOIN b ON a.id = b.aid",
		"SELECT a.id FROM a JOIN b USING (id)",
		"SELECT a.id FROM a NATURAL JOIN b",
		"SELECT a.id FROM a, b WHERE a.id = b.aid",
	}
	for _, sql := range cases {
		report, err := Check(sql)
		require.NoError(t, err)
		assertNoRule(t, report, RuleCartesianJoin)
	}
}

// TestRule_CartesianJoinNestedOnce verifies a bad join inside a subquery is
// reported once rather than once per nesting level.
func TestRule_CartesianJoinNestedOnce(t *testing.T) {
	report, err := Check("SELECT x.id FROM (SELECT a.id FROM a CROSS JOIN b) x")
	require.NoError(t, err)
	requireRule(t, report, RuleCartesianJoin)
}

// TestRule_OrderWithoutLimit covers the info-severity ordering rule.
func TestRule_OrderWithoutLimit(t *testing.T) {
	report, err := Check("SELECT id FROM users ORDER BY created_at DESC")
	require.NoError(t, err)
	finding := requireRule(t, report, RuleOrderWithoutLimit)
	assert.Equal(t, SeverityInfo, finding.Severity)
	assert.Equal(t, "created_at", finding.Fragment)
	assert.Equal(t, 1, report.InfoCount)
	assert.Contains(t, finding.Message, "排序")
}

// TestRule_OrderWithLimitSuppressed verifies LIMIT calms the ordering rule.
func TestRule_OrderWithLimitSuppressed(t *testing.T) {
	report, err := Check("SELECT id FROM users ORDER BY created_at DESC LIMIT 20")
	require.NoError(t, err)
	assertNoRule(t, report, RuleOrderWithoutLimit)
}

// TestRule_JoinAndOrderCombined verifies severity ordering puts the high
// findings before the info ordering finding, with stable offsets.
func TestRule_JoinAndOrderCombined(t *testing.T) {
	report, err := Check("SELECT a.id FROM a CROSS JOIN b ORDER BY a.id")
	require.NoError(t, err)
	require.Len(t, report.Findings, 3)
	assert.Equal(t, SeverityHigh, report.Findings[0].Severity)
	assert.Equal(t, SeverityHigh, report.Findings[1].Severity)
	assert.Equal(t, RuleCartesianJoin, report.Findings[1].Rule)
	assert.Equal(t, RuleOrderWithoutLimit, report.Findings[2].Rule)
	assert.Equal(t, SeverityInfo, report.Findings[2].Severity)
	assert.Equal(t, 2, report.HighCount)
	assert.Equal(t, 1, report.InfoCount)
}
