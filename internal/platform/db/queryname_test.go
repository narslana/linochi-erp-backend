package db

import "testing"

func TestOBS_QueryNameComesFromTheSqlcComment(t *testing.T) {
	tests := []struct {
		name, sql, want string
	}{
		{"sqlc one", "-- name: GetUser :one\nSELECT * FROM users WHERE id = $1", "GetUser"},
		{"sqlc exec", "-- name: ArchiveUser :one\nUPDATE users SET archived_at = now()", "ArchiveUser"},
		{"leading whitespace", "\n  -- name: ListUsers :many\nSELECT 1", "ListUsers"},
		{"no comment is one bounded label", "SELECT 1", "other"},
		{"ping", ";", "other"},
		{"empty", "", "other"},
		{"a comment that is not a name", "-- hello\nSELECT 1", "other"},
		{"hostile name cannot make new label values", "-- name: x; DROP TABLE users :one\nSELECT 1", "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QueryName(tt.sql); got != tt.want {
				t.Errorf("QueryName(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}
