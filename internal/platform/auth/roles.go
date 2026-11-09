package auth

import "slices"

// RoleAdmin is the only role there is for now (DECISIONS.md 2026-11-05): there is
// no read role yet.
const RoleAdmin = "admin"

// RolesFromGroups works out the roles of a person from their Authelia groups at
// login. Roles are not stored in users: they are computed here at every login
// and kept in the session. adminGroups is configuration (AUTH_ADMIN_GROUPS), so
// a read role can be added later without a code change. A group name must match
// exactly. No role means no access: the caller must not make a session.
func RolesFromGroups(groups, adminGroups []string) []string {
	for _, g := range groups {
		if g != "" && slices.Contains(adminGroups, g) {
			return []string{RoleAdmin}
		}
	}
	return nil
}

// SafeReturnTo is the path the browser is sent to after a login, or "/" when
// what was asked for is not a path of this app. It is checked when the login
// starts and again when it ends. An absolute URL, a protocol-relative one
// (//evil.example), a backslash (browsers treat \ as /) and control characters
// would all make the app an open redirect.
func SafeReturnTo(p string) string {
	const maxLen = 512
	if p == "" || len(p) > maxLen || p[0] != '/' {
		return "/"
	}
	if len(p) > 1 && (p[1] == '/' || p[1] == '\\') {
		return "/"
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return "/"
		}
	}
	return p
}
