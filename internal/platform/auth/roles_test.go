package auth_test

import (
	"slices"
	"testing"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
)

func TestNFK10_RolesComeFromAutheliaGroups(t *testing.T) {
	admin := []string{"admins", "linochi_erp_users"}
	tests := []struct {
		name   string
		groups []string
		config []string
		want   []string
	}{
		{"admins group", []string{"admins"}, admin, []string{auth.RoleAdmin}},
		{"linochi_erp_users group", []string{"linochi_erp_users"}, admin, []string{auth.RoleAdmin}},
		{"both give the one role once", []string{"admins", "linochi_erp_users"}, admin, []string{auth.RoleAdmin}},
		{"among others", []string{"family", "linochi_erp_users", "dev"}, admin, []string{auth.RoleAdmin}},
		{"no groups is no role", nil, admin, nil},
		{"only other groups is no role", []string{"family", "dev"}, admin, nil},
		// Group names are exact. A near miss must not be a role.
		{"case matters", []string{"Admins", "LINOCHI_ERP_USERS"}, admin, nil},
		{"a prefix is not the group", []string{"admins-readonly", "linochi_erp_users_old"}, admin, nil},
		{"whitespace is not trimmed from what Authelia says", []string{" admins"}, admin, nil},
		{"the mapping is configuration", []string{"ops"}, []string{"ops"}, []string{auth.RoleAdmin}},
		{"and so is what it leaves out", []string{"admins"}, []string{"ops"}, nil},
		{"nothing configured gives nothing", []string{"admins"}, nil, nil},
		{"an empty group name never matches an empty setting", []string{""}, []string{""}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auth.RolesFromGroups(tt.groups, tt.config); !slices.Equal(got, tt.want) {
				t.Errorf("RolesFromGroups(%q, %q) = %q, want %q", tt.groups, tt.config, got, tt.want)
			}
		})
	}
}

func TestNFK10_ReturnToMustBeAPathOfThisApp(t *testing.T) {
	// A percent-encoded CR LF is just characters in a path: it is the raw ones
	// that would split a header, and those are in the list below.
	ok := []string{"/", "/products", "/products/42?tab=stock", "/orders?status=open&page=2", "/a/b/c#section", "/%0d%0aSet-Cookie:x", "/%2f%2fevil.example"}
	for _, p := range ok {
		if got := auth.SafeReturnTo(p); got != p {
			t.Errorf("SafeReturnTo(%q) = %q, want it unchanged", p, got)
		}
	}
	bad := []string{
		"", "products", "https://evil.example", "http://evil.example/x", "//evil.example", "//evil.example/x",
		"/\\evil.example", "\\\\evil.example", "/\\/evil.example", "javascript:alert(1)",
		"/a\r\nSet-Cookie: x=1", "/a\nb", "/a\tb", "/a\x00b", "/a\x7fb", "/path\\with\\backslash", " /spaces-first",
		"/" + repeat("a", 600),
	}
	for _, p := range bad {
		if got := auth.SafeReturnTo(p); got != "/" {
			t.Errorf("SafeReturnTo(%q) = %q, want \"/\"", p, got)
		}
	}
}

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}
