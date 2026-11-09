//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

var server *testdb.Server

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		s, err := testdb.StartServer(ctx)
		if err != nil {
			_, _ = os.Stderr.WriteString("start postgres: " + err.Error() + "\n")
			return 1
		}
		server = s
		defer func() { _ = s.Terminate(context.Background()) }()
		return m.Run()
	}())
}

type env struct {
	pool  *pgxpool.Pool
	users *auth.UserStore
	logs  *bytes.Buffer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := server.NewMigratedDatabase(t)
	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	pool, err := db.Open(context.Background(), db.Options{URL: testdb.AppURL(dsn), MaxConns: 8, Metrics: metrics, Tracer: tp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	logs := &bytes.Buffer{}
	rec, err := audit.NewRecorder(obs.NewLogger(logs, slog.LevelDebug), metrics)
	if err != nil {
		t.Fatal(err)
	}
	return &env{pool: pool, users: auth.NewUserStore(pool, rec), logs: logs}
}

const issuer = "https://auth.lab.internal"

var adminID = uuid.MustParse("0194fa5e-0000-7000-8000-0000000000a1")

func login(subject, email, name string) auth.UserLogin {
	return auth.UserLogin{Issuer: issuer, Subject: subject, Email: email, DisplayName: name}
}

func adminCtx() context.Context {
	return audit.WithActor(context.Background(), audit.UserActor(adminID))
}

func (e *env) auditRows(t *testing.T, userID uuid.UUID) []dbgen.AuditLog {
	t.Helper()
	rows, err := dbgen.New(e.pool).ListAuditEntriesForEntity(context.Background(),
		dbgen.ListAuditEntriesForEntityParams{EntityType: "user", EntityID: userID})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func keysOf(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	if raw == nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNFK28_UpsertUserOnLogin(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	first, err := e.users.UpsertOnLogin(ctx, login("sub-1", "anna@example.se", "Anna"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || first.LastLoginAt == nil || first.ArchivedAt != nil {
		t.Fatalf("new user: version=%d last_login=%v archived=%v", first.Version, first.LastLoginAt, first.ArchivedAt)
	}
	if first.ID.Version() != 7 {
		t.Errorf("id %s is a version %d UUID, want 7", first.ID, first.ID.Version())
	}

	time.Sleep(20 * time.Millisecond)
	second, err := e.users.UpsertOnLogin(ctx, login("sub-1", "anna.s@example.se", "Anna S"))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Errorf("the same issuer and subject gave a new id: %s then %s", first.ID, second.ID)
	}
	if second.Email != "anna.s@example.se" || second.DisplayName != "Anna S" {
		t.Errorf("profile was not refreshed: %q %q", second.Email, second.DisplayName)
	}
	if second.Version != 2 {
		t.Errorf("version = %d, want 2 after the second login", second.Version)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Error("created_at changed")
	}
	if !second.LastLoginAt.After(*first.LastLoginAt) {
		t.Error("last_login_at was not moved forward")
	}
}

func TestNFK28_UsersAreUniquePerIssuerAndSubject(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a, err := e.users.UpsertOnLogin(ctx, auth.UserLogin{Issuer: "https://a.example", Subject: "same", Email: "a@example.se"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.users.UpsertOnLogin(ctx, auth.UserLogin{Issuer: "https://b.example", Subject: "same", Email: "b@example.se"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Error("the same subject at two issuers must be two users")
	}
}

func TestNFK28_GetUserNotFound(t *testing.T) {
	e := newEnv(t)
	if _, err := e.users.Get(context.Background(), uuid.Must(uuid.NewV7())); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("err = %v, want db.ErrNotFound", err)
	}
}

// The login is a write like any other: the first one is a create with the whole
// record, later ones are logins that hold only what changed.
func TestINS05_LoginsAreAuditedAsCreateAndLogin(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	u, err := e.users.UpsertOnLogin(ctx, login("sub-1", "anna@example.se", "Anna"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.users.UpsertOnLogin(ctx, login("sub-1", "anna.s@example.se", "Anna")); err != nil { // email changed
		t.Fatal(err)
	}
	if _, err := e.users.UpsertOnLogin(ctx, login("sub-1", "anna.s@example.se", "Anna")); err != nil { // nothing else changed
		t.Fatal(err)
	}

	rows := e.auditRows(t, u.ID)
	if len(rows) != 3 {
		t.Fatalf("audit rows = %d, want 3 (create, login, login)", len(rows))
	}
	create := rows[0]
	if create.Action != audit.Create || create.Module != "auth" || create.Before != nil {
		t.Errorf("first row: %+v", create)
	}
	after := keysOf(t, create.After)
	for _, k := range []string{"id", "oidc_issuer", "oidc_subject", "email", "display_name", "last_login_at", "version"} {
		if _, ok := after[k]; !ok {
			t.Errorf("the create must hold the whole record, %q is missing: %v", k, after)
		}
	}
	for _, r := range rows {
		if r.ActorType != "system" || r.ActorLabel != "oidc-login" || r.ActorID != nil {
			t.Errorf("a login is made by the system actor oidc-login: %+v", r)
		}
	}

	second := rows[1]
	before, after2 := keysOf(t, second.Before), keysOf(t, second.After)
	if second.Action != "login" || len(before) != 2 || len(after2) != 2 {
		t.Fatalf("second row: action=%s before=%v after=%v, want email and last_login_at only", second.Action, before, after2)
	}
	if before["email"] != "anna@example.se" || after2["email"] != "anna.s@example.se" {
		t.Errorf("email before/after: %v / %v", before["email"], after2["email"])
	}
	if _, ok := after2["last_login_at"]; !ok {
		t.Error("last_login_at is part of every login")
	}

	third := rows[2]
	if b := keysOf(t, third.Before); len(b) != 1 {
		t.Errorf("a login that changes nothing else holds only last_login_at, before = %v", b)
	}
}

func TestNFK28_ArchiveWithStaleVersionIsRejected(t *testing.T) {
	e := newEnv(t)
	u, err := e.users.UpsertOnLogin(context.Background(), login("sub-1", "a@example.se", "A"))
	if err != nil {
		t.Fatal(err)
	}

	archived, err := e.users.Archive(adminCtx(), u.ID, u.Version)
	if err != nil {
		t.Fatalf("archive with the current version: %v", err)
	}
	if archived.Version != u.Version+1 || archived.ArchivedAt == nil {
		t.Fatalf("after archive: version=%d archived_at=%v", archived.Version, archived.ArchivedAt)
	}
	// Someone else already changed the row: the same, now stale, version is rejected.
	if _, err := e.users.Archive(adminCtx(), u.ID, u.Version); !errors.Is(err, db.ErrStaleVersion) {
		t.Fatalf("stale archive: err = %v, want db.ErrStaleVersion", err)
	}
	if _, err := e.users.Archive(adminCtx(), uuid.Must(uuid.NewV7()), 1); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("archive of a missing user: err = %v, want db.ErrNotFound", err)
	}
}

// NFK-09: archived records are hidden, never deleted. The archive keeps the
// whole record as before in the audit log, and a rejected attempt leaves no trace.
func TestINS05_ArchiveIsAuditedWithTheWholeRecordAsBefore(t *testing.T) {
	e := newEnv(t)
	u, err := e.users.UpsertOnLogin(context.Background(), login("sub-1", "a@example.se", "A"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.users.Archive(adminCtx(), u.ID, u.Version); err != nil {
		t.Fatal(err)
	}
	_, _ = e.users.Archive(adminCtx(), u.ID, u.Version) // stale: must not be audited

	rows := e.auditRows(t, u.ID)
	if len(rows) != 2 {
		t.Fatalf("audit rows = %d, want create and archive only", len(rows))
	}
	archive := rows[1]
	if archive.Action != audit.Archive || archive.After != nil {
		t.Errorf("archive row: action=%s after=%s", archive.Action, archive.After)
	}
	before := keysOf(t, archive.Before)
	if before["email"] != "a@example.se" || before["archived_at"] != nil {
		t.Errorf("before must be the whole record as it was: %v", before)
	}
	if archive.ActorType != "user" || archive.ActorID == nil || *archive.ActorID != adminID {
		t.Errorf("the archive is made by the user in the context: %+v", archive)
	}

	got, err := e.users.Get(context.Background(), u.ID)
	if err != nil || got.ArchivedAt == nil {
		t.Fatalf("an archived user is hidden, not deleted: %+v, %v", got, err)
	}
}

func TestINS05_ArchivingTwiceWithTheCurrentVersionChangesNothing(t *testing.T) {
	e := newEnv(t)
	u, _ := e.users.UpsertOnLogin(context.Background(), login("sub-1", "a@example.se", "A"))
	first, err := e.users.Archive(adminCtx(), u.ID, u.Version)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.users.Archive(adminCtx(), u.ID, first.Version)
	if err != nil {
		t.Fatal(err)
	}
	if again.Version != first.Version || !again.ArchivedAt.Equal(*first.ArchivedAt) {
		t.Errorf("a second archive changed the row: version %d -> %d", first.Version, again.Version)
	}
	if n := len(e.auditRows(t, u.ID)); n != 2 {
		t.Errorf("audit rows = %d, want 2: a second archive is not a change", n)
	}
}

func TestINS05_ArchiveWithoutAnActorFailsAndChangesNothing(t *testing.T) {
	e := newEnv(t)
	u, _ := e.users.UpsertOnLogin(context.Background(), login("sub-1", "a@example.se", "A"))
	if _, err := e.users.Archive(context.Background(), u.ID, u.Version); !errors.Is(err, audit.ErrNoActor) {
		t.Fatalf("err = %v, want audit.ErrNoActor", err)
	}
	got, _ := e.users.Get(context.Background(), u.ID)
	if got.ArchivedAt != nil || got.Version != u.Version {
		t.Errorf("an unattributed archive was saved: %+v", got)
	}
}

// Two people logging in for the first time at once must give one user and one
// create, and the other seven are logins.
func TestINS05_ConcurrentFirstLoginsMakeOneUserAndOneCreate(t *testing.T) {
	e := newEnv(t)
	const n = 8
	var wg sync.WaitGroup
	ids := make([]uuid.UUID, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, err := e.users.UpsertOnLogin(context.Background(), login("same-person", "x@example.se", "X"))
			ids[i], errs[i] = u.ID, err
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("login %d: %v", i, err)
		}
		if ids[i] != ids[0] {
			t.Fatalf("logins gave different users: %v", ids)
		}
	}
	var users int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("users = %d, err %v", users, err)
	}
	var creates, logins int
	for _, r := range e.auditRows(t, ids[0]) {
		switch r.Action {
		case audit.Create:
			creates++
		case "login":
			logins++
		}
	}
	if creates != 1 || logins != n-1 {
		t.Errorf("audit: %d creates and %d logins, want 1 and %d", creates, logins, n-1)
	}
}

// A login for an archived user is refused before anything is written: no profile
// refresh, no new last_login_at and no audit entry for something that did not
// happen (NFK-09, INS-05).
func TestNFK10_LoginOfAnArchivedUserChangesNothingAndSaysSo(t *testing.T) {
	e := newEnv(t)
	u, err := e.users.UpsertOnLogin(context.Background(), login("anna", "anna@example.se", "Anna"))
	if err != nil {
		t.Fatal(err)
	}
	archived, err := e.users.Archive(adminCtx(), u.ID, u.Version)
	if err != nil {
		t.Fatal(err)
	}
	before := e.auditRows(t, u.ID)

	_, err = e.users.UpsertOnLogin(context.Background(), login("anna", "new-address@example.se", "Anna Renamed"))
	if !errors.Is(err, auth.ErrUserArchived) {
		t.Fatalf("err = %v, want auth.ErrUserArchived", err)
	}

	after, err := e.users.Get(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != archived.Version || after.Email != "anna@example.se" || after.DisplayName != "Anna" ||
		(after.LastLoginAt == nil) != (archived.LastLoginAt == nil) ||
		(after.LastLoginAt != nil && !after.LastLoginAt.Equal(*archived.LastLoginAt)) {
		t.Errorf("an archived user was changed by a refused login: %+v", after)
	}
	if rows := e.auditRows(t, u.ID); len(rows) != len(before) {
		t.Errorf("%d audit rows, want %d: a refused login must not be recorded as one", len(rows), len(before))
	}
}
