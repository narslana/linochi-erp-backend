package db

import "regexp"

// sqlcName matches the first line sqlc keeps in every generated query:
// "-- name: GetUser :one". Only a strict identifier followed by one of sqlc's
// command words counts, so SQL text can never invent new label values.
var sqlcName = regexp.MustCompile(`^\s*--\s*name:\s*([A-Za-z_][A-Za-z0-9_]*)\s+:(?:one|many|exec|execrows|execresult|execlastid|batchexec|batchone|batchmany|copyfrom)\b`)

// QueryName returns the sqlc query name of a statement, or "other" for
// anything unnamed (pings, ad-hoc SQL). It is the label of query metrics and
// part of span names, so it must stay a small, bounded set.
func QueryName(sql string) string {
	if m := sqlcName.FindStringSubmatch(sql); m != nil {
		return m[1]
	}
	return "other"
}
