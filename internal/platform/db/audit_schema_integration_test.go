//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const insertAudit = `
	INSERT INTO audit_log (actor_type, actor_id, actor_label, module, entity_type, entity_id, action, before, after)
	VALUES ($1, $2, $3, 'test', 'widget', $4::uuid, 'update', $5::jsonb, $6::jsonb)`

const (
	someUUID  = "0194fa5e-0000-7000-8000-000000000001"
	otherUUID = "0194fa5e-0000-7000-8000-000000000002"
)

func addAudit(t *testing.T, conn *pgx.Conn, entity, before, after string) {
	t.Helper()
	var b, a any
	if before != "" {
		b = before
	}
	if after != "" {
		a = after
	}
	if _, err := conn.Exec(context.Background(), insertAudit, "system", nil, "test", entity, b, a); err != nil {
		t.Fatalf("insert audit row: %v", err)
	}
}

func TestINS05_AuditLogChecksTheActor(t *testing.T) {
	ctx := context.Background()
	app := appConn(t, migrated(t))

	tests := []struct {
		name                 string
		actorType, id, label any
		wantErr              bool
	}{
		{"system with a label", "system", nil, "oidc-login", false},
		{"user with an id", "user", someUUID, "", false},
		{"api key with an id", "api_key", someUUID, "", false},
		{"unknown actor type", "robot", nil, "x", true},
		{"system without a label", "system", nil, "", true},
		{"system with an id", "system", someUUID, "x", true},
		{"user without an id", "user", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.Exec(ctx, insertAudit, tt.actorType, tt.id, tt.label, someUUID, nil, `{"a":1}`)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNFK09_AppCannotUpdateDeleteOrTruncateAuditRows(t *testing.T) {
	owner := migrated(t)
	app := appConn(t, owner)
	addAudit(t, app, someUUID, "", `{"email":"a@example.se"}`)

	mustFailWith(t, "UPDATE", app, `UPDATE audit_log SET action = 'tampered'`, "permission denied")
	mustFailWith(t, "DELETE", app, `DELETE FROM audit_log`, "permission denied")
	mustFailWith(t, "TRUNCATE", app, `TRUNCATE audit_log`, "permission denied")
	mustFailWith(t, "DROP", app, `DROP TABLE audit_log`, "must be owner")
}

// The trigger is a second lock behind the privileges: even the owner, connected
// directly, cannot rewrite history. Only audit_redact_personal_data() can.
func TestNFK09_TriggerBlocksEvenTheOwnerOutsideTheDefinerFunction(t *testing.T) {
	owner := migrated(t)
	ownerConn := connect(t, owner)
	addAudit(t, appConn(t, owner), someUUID, "", `{"email":"a@example.se"}`)

	mustFailWith(t, "owner UPDATE of a locked column", ownerConn, `UPDATE audit_log SET action = 'tampered'`, "append-only")
	mustFailWith(t, "owner UPDATE of a redactable column", ownerConn, `UPDATE audit_log SET after = '{}'`, "append-only")
	mustFailWith(t, "owner DELETE", ownerConn, `DELETE FROM audit_log`, "append-only")
	mustFailWith(t, "owner TRUNCATE", ownerConn, `TRUNCATE audit_log`, "append-only")
}

func TestINS05_RedactReplacesOldValuesAndLeavesTheRest(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	app := appConn(t, owner)
	addAudit(t, app, someUUID, `{"email":"old@example.se","display_name":"Anna","color":"red"}`, `{"email":"new@example.se","display_name":"Anna S","color":"blue"}`)
	addAudit(t, app, otherUUID, "", `{"email":"someone-else@example.se"}`)

	var n int
	if err := app.QueryRow(ctx, `SELECT audit_redact_personal_data('widget', $1::uuid, ARRAY['email','display_name'])`, someUUID).Scan(&n); err != nil {
		t.Fatalf("redact as the app role: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows redacted = %d, want 1", n)
	}

	var before, after string
	var redacted bool
	var version int
	if err := app.QueryRow(ctx, `SELECT before::text, after::text, redacted_at IS NOT NULL, version FROM audit_log WHERE entity_id = $1::uuid`, someUUID).Scan(&before, &after, &redacted, &version); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"old@example.se", "new@example.se", "Anna"} {
		if strings.Contains(before+after, leaked) {
			t.Errorf("%q survived the redaction: before=%s after=%s", leaked, before, after)
		}
	}
	if !strings.Contains(before, `"color": "red"`) || !strings.Contains(after, `"color": "blue"`) {
		t.Errorf("a field that was not asked for changed: before=%s after=%s", before, after)
	}
	if strings.Count(before+after, "[redacted]") != 4 {
		t.Errorf("want four [redacted] values: before=%s after=%s", before, after)
	}
	if !redacted || version != 2 {
		t.Errorf("redacted_at set = %v, version = %d (want true and 2)", redacted, version)
	}

	// Another entity is untouched.
	var other string
	if err := app.QueryRow(ctx, `SELECT after::text FROM audit_log WHERE entity_id = $1::uuid`, otherUUID).Scan(&other); err != nil || !strings.Contains(other, "someone-else@example.se") {
		t.Errorf("another entity's row changed: %s (%v)", other, err)
	}
}

func TestINS05_RedactNeedsFieldsAndOnlyTheAppRoleMayCallIt(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	app := appConn(t, owner)
	addAudit(t, app, someUUID, "", `{"email":"a@example.se"}`)

	if err := app.QueryRow(ctx, `SELECT audit_redact_personal_data('widget', $1::uuid, ARRAY[]::text[])`, someUUID).Scan(new(int)); err == nil {
		t.Error("an empty field list must be rejected")
	}
	// The function is owned by the redactor role, so the owner has no right to
	// call it: EXECUTE was revoked from PUBLIC and granted to the app role only.
	err := connect(t, owner).QueryRow(ctx, `SELECT audit_redact_personal_data('widget', $1::uuid, ARRAY['email'])`, someUUID).Scan(new(int))
	if err == nil || !strings.Contains(err.Error(), "permission denied for function") {
		t.Errorf("owner calling the function: err = %v, want permission denied for function", err)
	}
}

// Another SECURITY DEFINER function does not get through, whoever owns it. One
// that the owner wrote runs as the owner, and the trigger lets only the redactor
// role change old rows.
func TestINS05_AnotherDefinerFunctionOwnedByTheOwnerIsStopped(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	ownerConn := connect(t, owner)
	app := appConn(t, owner)
	addAudit(t, app, someUUID, "", `{"email":"a@example.se"}`)

	for name, body := range map[string]string{
		"a locked column":     `UPDATE audit_log SET action = 'tampered'`,
		"a redactable column": `UPDATE audit_log SET after = '{}'`,
		"a delete":            `DELETE FROM audit_log`,
	} {
		fn := "tamper_" + strings.ReplaceAll(name, " ", "_")
		if _, err := ownerConn.Exec(ctx, `
			CREATE FUNCTION `+fn+`() RETURNS void LANGUAGE sql SECURITY DEFINER AS $$ `+body+` $$;
			GRANT EXECUTE ON FUNCTION `+fn+`() TO linochi_erp_app`); err != nil {
			t.Fatal(err)
		}
		_, err := app.Exec(ctx, `SELECT `+fn+`()`)
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s through a definer function owned by the owner: err = %v, want the append-only refusal", name, err)
		}
	}
	var action, after string
	if err := app.QueryRow(ctx, `SELECT action, after::text FROM audit_log`).Scan(&action, &after); err != nil || action != "update" || !strings.Contains(after, "a@example.se") {
		t.Errorf("the row changed: action=%q after=%s err=%v", action, after, err)
	}
}

// Defence in depth: even if the redactor's privileges were widened by mistake,
// the trigger still lets it change only before, after and redacted_at.
func TestINS05_RedactorCannotChangeLockedColumnsEvenIfPrivilegesWereWidened(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	ownerConn := connect(t, owner)
	addAudit(t, appConn(t, owner), someUUID, "", `{"email":"a@example.se"}`)

	if _, err := ownerConn.Exec(ctx, `GRANT UPDATE (action) ON audit_log TO linochi_erp_audit_redactor`); err != nil {
		t.Fatal(err)
	}
	// The owner is a member of the redactor role, for the migrations' sake.
	if _, err := ownerConn.Exec(ctx, `SET ROLE linochi_erp_audit_redactor`); err != nil {
		t.Fatalf("the owner must be able to SET ROLE to the redactor: %v", err)
	}
	_, err := ownerConn.Exec(ctx, `UPDATE audit_log SET action = 'tampered'`)
	if err == nil || !strings.Contains(err.Error(), "only before, after and redacted_at") {
		t.Errorf("err = %v, want the column refusal from the trigger", err)
	}
}

func TestINS05_RedactFunctionIsHardened(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, migrated(t))

	var definer bool
	var owner string
	var config []string
	if err := conn.QueryRow(ctx, `SELECT prosecdef, pg_get_userbyid(proowner), coalesce(proconfig, '{}') FROM pg_proc WHERE proname = 'audit_redact_personal_data'`).Scan(&definer, &owner, &config); err != nil {
		t.Fatal(err)
	}
	if !definer {
		t.Error("the function must be SECURITY DEFINER")
	}
	if owner != "linochi_erp_audit_redactor" {
		t.Errorf("the function is owned by %q, want the redactor role", owner)
	}
	var pinned bool
	for _, c := range config {
		if strings.HasPrefix(c, "search_path=") {
			pinned = true
		}
	}
	if !pinned {
		t.Errorf("a SECURITY DEFINER function must pin search_path, config = %v", config)
	}

	var appMay, ownerMay, publicMay bool
	if err := conn.QueryRow(ctx, `
		SELECT has_function_privilege('linochi_erp_app', 'audit_redact_personal_data(text, uuid, text[])', 'EXECUTE'),
		       has_function_privilege('linochi_erp_owner', 'audit_redact_personal_data(text, uuid, text[])', 'EXECUTE'),
		       has_function_privilege('public', 'audit_redact_personal_data(text, uuid, text[])', 'EXECUTE')`).Scan(&appMay, &ownerMay, &publicMay); err != nil {
		t.Fatal(err)
	}
	if !appMay || ownerMay || publicMay {
		t.Errorf("EXECUTE: app=%v owner=%v public=%v, want true, false, false", appMay, ownerMay, publicMay)
	}
}

func TestINS05_AuditLogHasIndexesForTheFilters(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, migrated(t))
	rows, err := conn.Query(ctx, `SELECT indexdef FROM pg_indexes WHERE tablename = 'audit_log'`)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(defs, "\n")
	for _, want := range []string{"(entity_type, entity_id, created_at DESC)", "(actor_id, created_at DESC)", "(module, created_at DESC)", "(created_at DESC)"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing index on %s", want)
		}
	}
}

// The redactor role can log in nowhere and can change three columns of one
// table, nothing else.
func TestNFK09_RedactorRoleIsNoLoginAndHasOnlyTheNeededPrivileges(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, migrated(t))

	var canLogin, super, createDB, createRole bool
	if err := conn.QueryRow(ctx, `SELECT rolcanlogin, rolsuper, rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname = 'linochi_erp_audit_redactor'`).Scan(&canLogin, &super, &createDB, &createRole); err != nil {
		t.Fatal(err)
	}
	if canLogin || super || createDB || createRole {
		t.Errorf("login=%v super=%v createdb=%v createrole=%v, want all false", canLogin, super, createDB, createRole)
	}

	has := func(sql string) bool {
		t.Helper()
		var ok bool
		if err := conn.QueryRow(ctx, sql).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	for _, col := range []string{"before", "after", "redacted_at"} {
		if !has(`SELECT has_column_privilege('linochi_erp_audit_redactor', 'audit_log', '` + col + `', 'UPDATE')`) {
			t.Errorf("the redactor must be able to UPDATE %s", col)
		}
	}
	for _, col := range []string{"id", "created_at", "updated_at", "version", "actor_type", "actor_id", "actor_label", "module", "entity_type", "entity_id", "action", "reason", "trace_id"} {
		if has(`SELECT has_column_privilege('linochi_erp_audit_redactor', 'audit_log', '` + col + `', 'UPDATE')`) {
			t.Errorf("the redactor must not be able to UPDATE %s", col)
		}
	}
	for _, priv := range []string{"INSERT", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
		if has(`SELECT has_table_privilege('linochi_erp_audit_redactor', 'audit_log', '` + priv + `')`) {
			t.Errorf("the redactor must not have %s on audit_log", priv)
		}
	}
	if has(`SELECT has_table_privilege('linochi_erp_audit_redactor', 'users', 'SELECT')`) {
		t.Error("the redactor must not read users")
	}
	// The temporary CREATE on the schema, needed to hand over the function, is gone.
	if has(`SELECT has_schema_privilege('linochi_erp_audit_redactor', 'public', 'CREATE')`) {
		t.Error("the redactor must not be able to create objects")
	}
}

// The owner is a member of the redactor role, because the migrations hand the
// function to it and ALTER ... OWNER TO needs that. The app role never is.
func TestNFK09_OnlyTheOwnerIsAMemberOfTheRedactorRole(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	conn := connect(t, owner)

	var ownerMember, appMember, appCanSet bool
	if err := conn.QueryRow(ctx, `
		SELECT pg_has_role('linochi_erp_owner', 'linochi_erp_audit_redactor', 'MEMBER'),
		       pg_has_role('linochi_erp_app',   'linochi_erp_audit_redactor', 'MEMBER'),
		       pg_has_role('linochi_erp_app',   'linochi_erp_audit_redactor', 'USAGE')`).Scan(&ownerMember, &appMember, &appCanSet); err != nil {
		t.Fatal(err)
	}
	if !ownerMember || appMember || appCanSet {
		t.Errorf("owner member=%v, app member=%v, app can use=%v; want true, false, false", ownerMember, appMember, appCanSet)
	}

	app := appConn(t, owner)
	mustFailWith(t, "app SET ROLE to the redactor", app, `SET ROLE linochi_erp_audit_redactor`, "permission denied to set role")
	// The owner does not inherit the redactor's rights, it can only switch to it.
	var inherits bool
	if err := conn.QueryRow(ctx, `
		SELECT coalesce(bool_or(m.inherit_option), false)
		FROM pg_auth_members m
		WHERE pg_get_userbyid(m.roleid) = 'linochi_erp_audit_redactor' AND pg_get_userbyid(m.member) = 'linochi_erp_owner'`).Scan(&inherits); err != nil {
		t.Fatal(err)
	}
	if inherits {
		t.Error("the owner must not inherit the redactor's privileges, only be able to SET ROLE")
	}
}
