package lint

import (
	"strings"

	"github.com/valkdb/postgresparser"
)

// rawFinding is a finding positioned relative to the raw text of the query
// node that produced it. resolveOffsets later turns the relative start into a
// byte offset inside the enclosing statement.
type rawFinding struct {
	rule      string
	severity  Severity
	message   string
	fragment  string
	relStart  int
	queryText string
}

// collector walks a ParsedQuery tree and accumulates findings. It holds no
// pointer into the parsed IR and never mutates it.
type collector struct {
	statementIndex int
	// rootText is the raw text of the top-level statement, used to resolve
	// absolute fragment offsets.
	rootText string
	raw      []rawFinding
	// seen deduplicates findings by rule name and relative text range, so one
	// issue is reported once even when the same query node is reachable through
	// several IR links (subquery list, CTE list, set-operation tables).
	seen map[string]struct{}
}

// buildReport runs every applicable rule against one parsed statement and
// assembles a sorted, self-consistent report. It never mutates query.
func buildReport(query *postgresparser.ParsedQuery, statementIndex int) *Report {
	c := &collector{
		statementIndex: statementIndex,
		rootText:       query.RawSQL,
		seen:           make(map[string]struct{}),
	}
	c.walk(query, 0)
	return finalize(c.resolveOffsets(), statementIndex)
}

// walk applies the statement-scoped rules to one query node and then recurses// into every nested query it owns. depth counts subquery nesting edges only.
func (c *collector) walk(query *postgresparser.ParsedQuery, depth int) {
	if query == nil {
		return
	}

	if query.Command == postgresparser.QueryCommandSelect {
		c.checkResultBounds(query)
		c.checkStarProjection(query)
		c.checkCartesianJoin(query)
		c.checkOrderWithoutLimit(query)
	}

	for i := range query.Subqueries {
		c.walk(query.Subqueries[i].Query, depth+1)
	}
	for i := range query.CTEs {
		// CTE bodies are reviewed as their own query scopes but do not add a
		// subquery-nesting level.
		c.walk(query.CTEs[i].ParsedQuery, depth)
	}
}

// hasLimit reports whether a query node bounds its result size. The parser
// populates Limit for both LIMIT and standalone OFFSET clauses.
func hasLimit(query *postgresparser.ParsedQuery) bool {
	return query.Limit != nil
}

// checkResultBounds enforces the mutually exclusive full-table-scan (high)
// and unbounded-result (medium) rules: exactly one can fire per query node.
func (c *collector) checkResultBounds(query *postgresparser.ParsedQuery) {
	hasWhere := len(query.Where) > 0
	switch {
	case !hasWhere && !hasLimit(query):
		c.add(query, RuleFullTableScan, SeverityHigh,
			"查询语句既没有 where 也没有 limit，存在全表扫描风险", query.RawSQL, 0)
	case hasWhere && !hasLimit(query):
		c.add(query, RuleUnboundedResult, SeverityMedium,
			"查询语句有 where 但没有 limit，结果集大小不可控", query.RawSQL, 0)
	}
}

// checkStarProjection flags any * or alias.* item in the projection list.
func (c *collector) checkStarProjection(query *postgresparser.ParsedQuery) {
	for _, column := range query.Columns {
		expr := strings.TrimSpace(column.Expression)
		if expr == "" {
			continue
		}
		if expr != "*" && !strings.HasSuffix(expr, ".*") {
			continue
		}
		fragment, relStart := locate(query.RawSQL, expr)
		c.add(query, RuleStarProjection, SeverityMedium,
			"查询语句使用星号选择列，字段没有收敛", fragment, relStart)
	}
}

// checkCartesianJoin flags every explicit join that carries neither an
// ON/USING condition nor an implicit NATURAL condition. Comma-separated FROM
// items are not joins in the IR and are intentionally left alone.
func (c *collector) checkCartesianJoin(query *postgresparser.ParsedQuery) {
	for _, table := range query.Tables {
		if table.JoinType == "" || table.Nested {
			continue
		}
		if strings.HasPrefix(table.JoinType, "NATURAL") || strings.TrimSpace(table.JoinCondition) != "" {
			continue
		}
		fragment, relStart := locate(query.RawSQL, joinFragment(table))
		c.add(query, RuleCartesianJoin, SeverityHigh,
			"join 缺少连接条件，存在笛卡尔积风险", fragment, relStart)
	}
}

// joinFragment picks the most specific available raw text for an
// unconditional join: the relation's own text when present, else the join
// type keyword (e.g. CROSS JOIN).
func joinFragment(table postgresparser.TableRef) string {
	if raw := strings.TrimSpace(table.Raw); raw != "" {
		return raw
	}
	return table.JoinType
}

// checkOrderWithoutLimit flags ORDER BY blocks that are not bounded by a
// LIMIT clause, since the database must sort the full result set.
func (c *collector) checkOrderWithoutLimit(query *postgresparser.ParsedQuery) {
	if len(query.OrderBy) == 0 || hasLimit(query) {
		return
	}
	fragment, relStart := locate(query.RawSQL, orderFragment(query))
	c.add(query, RuleOrderWithoutLimit, SeverityInfo,
		"order by 后面没有 limit，注意排序开销", fragment, relStart)
}

// orderFragment renders the leading ORDER BY item text from the IR. The full
// ORDER BY clause is not stored verbatim, so the first sort expression is
// used as the anchor fragment.
func orderFragment(query *postgresparser.ParsedQuery) string {
	if len(query.OrderBy) == 0 {
		return query.RawSQL
	}
	return strings.TrimSpace(query.OrderBy[0].Expression)
}

// add records a finding unless the same rule has already been reported at the
// same relative range inside this query node.
func (c *collector) add(query *postgresparser.ParsedQuery, rule string, severity Severity, message, fragment string, relStart int) {
	key := rule + "|" + query.RawSQL + "|" + itoaLen(relStart) + "|" + fragment
	if _, ok := c.seen[key]; ok {
		return
	}
	c.seen[key] = struct{}{}
	c.raw = append(c.raw, rawFinding{
		rule:      rule,
		severity:  severity,
		message:   message,
		fragment:  fragment,
		relStart:  relStart,
		queryText: query.RawSQL,
	})
}

// locate returns fragment and its byte offset inside scope. When the fragment
// cannot be located verbatim it falls back to the whole scope at offset zero,
// so every finding still carries a real piece of the statement text.
func locate(scope, fragment string) (string, int) {
	if fragment == "" {
		return scope, 0
	}
	if offset := strings.Index(scope, fragment); offset >= 0 {
		return fragment, offset
	}
	return scope, 0
}

// itoaLen is a tiny allocation-free integer formatting helper for map keys.
func itoaLen(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
