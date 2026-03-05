package dateparse

import (
	"fmt"
	"regexp"
	"strings"
)

// dateExpressions maps natural language date terms to JQL date expressions.
var dateExpressions = map[string]string{
	"today":        "startOfDay()",
	"yesterday":    `"-1d"`,
	"recent":       `"-7d"`,
	"recently":     `"-7d"`,
	"last week":    `"-1w"`,
	"this week":    "startOfWeek()",
	"last month":   `"-30d"`,
	"this month":   "startOfMonth()",
	"last quarter": `"-90d"`,
	"this quarter": `"-90d"`,
	"this year":    "startOfYear()",
	"last year":    `"-365d"`,
}

var isoDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ParseDateExpression converts a natural language date expression into a JQL date value.
// If unrecognized, returns the input as-is (letting JQL handle it).
func ParseDateExpression(expr string) string {
	normalized := strings.ToLower(strings.TrimSpace(expr))

	if jql, ok := dateExpressions[normalized]; ok {
		return jql
	}

	// Passthrough ISO dates
	if isoDatePattern.MatchString(normalized) {
		return fmt.Sprintf("%q", normalized)
	}

	// Passthrough JQL functions and relative expressions
	if strings.HasPrefix(normalized, "-") || strings.Contains(normalized, "(") {
		return expr
	}

	// Return quoted for JQL
	return fmt.Sprintf("%q", expr)
}

// ToJQLDateClause builds a JQL clause like `updated >= "-7d"` from a field name and expression.
func ToJQLDateClause(field string, expr string) string {
	return fmt.Sprintf("%s >= %s", field, ParseDateExpression(expr))
}

// ToCQLDateClause builds a CQL date clause for Confluence search.
// CQL uses: lastModified >= "2024-01-01" or lastModified >= now("-7d")
func ToCQLDateClause(field string, expr string) string {
	normalized := strings.ToLower(strings.TrimSpace(expr))

	cqlMappings := map[string]string{
		"today":        `now("-0d")`,
		"yesterday":    `now("-1d")`,
		"recent":       `now("-7d")`,
		"recently":     `now("-7d")`,
		"last week":    `now("-1w")`,
		"this week":    `now("-0w")`,
		"last month":   `now("-30d")`,
		"this month":   `now("-0M")`,
		"last quarter": `now("-90d")`,
		"this quarter": `now("-90d")`,
		"this year":    `now("-0y")`,
		"last year":    `now("-365d")`,
	}

	if cql, ok := cqlMappings[normalized]; ok {
		return fmt.Sprintf("%s >= %s", field, cql)
	}

	if isoDatePattern.MatchString(normalized) {
		return fmt.Sprintf(`%s >= "%s"`, field, normalized)
	}

	return fmt.Sprintf(`%s >= "%s"`, field, expr)
}
