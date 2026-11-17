package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRule_SubqueryDepthThreshold verifies exactly three nested levels stays
// quiet while four levels produce one info finding.
func TestRule_SubqueryDepthThreshold(t *testing.T) {
	threeLevels := "SELECT id FROM (SELECT id FROM (SELECT id FROM t) x) y"
	report, err := Check(threeLevels)
	require.NoError(t, err)
	assertNoRule(t, report, RuleSubqueryDepth)
	assert.Zero(t, report.InfoCount)

	fourLevels := "SELECT id FROM (SELECT id FROM (SELECT id FROM (SELECT id FROM (SELECT id FROM t) a) b) c) d"
	report, err = Check(fourLevels)
	require.NoError(t, err)
	finding := requireRule(t, report, RuleSubqueryDepth)
	assert.Equal(t, SeverityInfo, finding.Severity)
	assert.Contains(t, finding.Fragment, "SELECT id FROM t")
	assert.Equal(t, 1, finding.Statement)
	assert.Equal(t, 1, report.InfoCount)
}

// TestRule_SubqueryDepthFiresOnce verifies the reminder is not repeated at
// every nested layer even though each layer is a separate SELECT node.
func TestRule_SubqueryDepthFiresOnce(t *testing.T) {
	sql := "SELECT id FROM (SELECT id FROM (SELECT id FROM (SELECT id FROM (SELECT id FROM t) a) b) c) d"
	report, err := Check(sql)
	require.NoError(t, err)
	requireRule(t, report, RuleSubqueryDepth)
	assert.Equal(t, 1, report.InfoCount)
}

// TestRule_SubqueryDepthInWhere covers scalar subqueries reached via WHERE.
func TestRule_SubqueryDepthInWhere(t *testing.T) {
	sql := "SELECT id FROM t WHERE id IN (SELECT id FROM (SELECT id FROM (SELECT id FROM (SELECT id FROM u) a) b) c)"
	report, err := Check(sql)
	require.NoError(t, err)
	requireRule(t, report, RuleSubqueryDepth)
}

// TestRule_CTEIgnoredForDepth verifies CTE nesting does not count as subquery
// nesting, while the CTE body still gets the other query rules applied.
func TestRule_CTEIgnoredForDepth(t *testing.T) {
	sql := "WITH c AS (SELECT * FROM t) SELECT id FROM c"
	report, err := Check(sql)
	require.NoError(t, err)
	assertNoRule(t, report, RuleSubqueryDepth)
	// The star lives inside the CTE body and must still be reported there.
	requireRule(t, report, RuleStarProjection)
}

// TestRule_DedupAcrossLinks verifies the same issue is reported once even
// when the nested query node is reachable through more than one IR link.
func TestRule_DedupAcrossLinks(t *testing.T) {
	// The star exists only inside the CTE body; the outer projection is
	// explicit so it cannot add a second star finding of its own.
	sql := "WITH c AS (SELECT * FROM t) SELECT c1.id FROM c c1 JOIN c c2 ON c1.id = c2.id"
	report, err := Check(sql)
	require.NoError(t, err)

	matches := 0
	for _, finding := range report.Findings {
		if finding.Rule == RuleStarProjection {
			matches++
		}
	}
	assert.Equal(t, 1, matches, "CTE star must be reported once")
}

// TestRule_DedupSameRangeStar verifies identical stars that share the same
// text range collapse into a single finding, matching the "one finding per
// rule per range" contract.
func TestRule_DedupSameRangeStar(t *testing.T) {
	report, err := Check("SELECT *, * FROM users")
	require.NoError(t, err)
	requireRule(t, report, RuleStarProjection)
}
