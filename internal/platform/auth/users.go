// Package auth holds users, and later sessions and the OIDC login (B5, B6).
package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

const (
	auditModule = "auth"
	auditEntity = "user"
	// loginActor is the system actor of the login flow: a person logging in is
	// not yet known to the app when the write happens.
	loginActor = "oidc-login"
	// loginAction is the audit action of every login after the first.
	loginAction = "login"
)

// UserLogin is what the OIDC login tells us about a user.
type UserLogin struct {
	Issuer      string
	Subject     string
	Email       string
	DisplayName string
}

// UserStore reads and writes users. Every write records an audit entry in the
// same transaction (INS-05).
type UserStore struct {
	pool  *pgxpool.Pool
	audit *audit.Recorder
}

// NewUserStore creates a UserStore.
func NewUserStore(pool *pgxpool.Pool, rec *audit.Recorder) *UserStore {
	return &UserStore{pool: pool, audit: rec}
}

// UpsertOnLogin creates the user on the first login and refreshes the profile
// and the time of the last login on later ones. The first login is audited as a
// create with the whole record, later ones as a login with the fields that
// changed. Concurrent first logins give one user and one create.
func (s *UserStore) UpsertOnLogin(ctx context.Context, l UserLogin) (dbgen.User, error) {
	ctx = audit.WithActor(ctx, audit.SystemActor(loginActor))
	id, err := uuid.NewV7()
	if err != nil {
		return dbgen.User{}, fmt.Errorf("new user id: %w", err)
	}

	var user dbgen.User
	err = db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		q := tx.Queries()
		// DO NOTHING: no row back means the user exists, whoever got there first.
		created, err := q.InsertUserIfNew(ctx, dbgen.InsertUserIfNewParams{
			ID: id, OidcIssuer: l.Issuer, OidcSubject: l.Subject, Email: l.Email, DisplayName: l.DisplayName,
		})
		if err == nil {
			user = created
			return s.audit.Record(ctx, tx, userEntry(audit.Create, created.ID, nil, created))
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("insert user: %w", err)
		}

		before, err := q.LockUserByOIDC(ctx, dbgen.LockUserByOIDCParams{OidcIssuer: l.Issuer, OidcSubject: l.Subject})
		if err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		after, err := q.UpdateUserOnLogin(ctx, dbgen.UpdateUserOnLoginParams{
			ID: before.ID, Email: l.Email, DisplayName: l.DisplayName,
		})
		if err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		user = after
		return s.audit.Record(ctx, tx, userEntry(loginAction, after.ID, before, after))
	})
	if err != nil {
		return dbgen.User{}, fmt.Errorf("user login: %w", err)
	}
	return user, nil
}

// Get returns db.ErrNotFound if there is no such user.
func (s *UserStore) Get(ctx context.Context, id uuid.UUID) (dbgen.User, error) {
	u, err := dbgen.New(s.pool).GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.User{}, db.ErrNotFound
	}
	if err != nil {
		return dbgen.User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// Archive hides the user (NFK-09: never deleted). version is the version the
// caller read: db.ErrStaleVersion if someone changed the row since, and
// db.ErrNotFound if there is no such user. The actor is the one in the context,
// and without one the call fails with audit.ErrNoActor and nothing is saved.
// Archiving an archived user again with its current version changes nothing.
func (s *UserStore) Archive(ctx context.Context, id uuid.UUID, version int32) (dbgen.User, error) {
	var user dbgen.User
	err := db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		q := tx.Queries()
		before, err := q.LockUser(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		if before.Version != version {
			return db.ErrStaleVersion
		}
		if before.ArchivedAt != nil {
			user = before
			return nil
		}
		after, err := q.ArchiveUser(ctx, dbgen.ArchiveUserParams{ID: id, Version: version})
		if err != nil {
			return fmt.Errorf("archive user: %w", err)
		}
		user = after
		// The whole record before, as the archive entry stores it.
		return s.audit.Record(ctx, tx, userEntry(audit.Archive, id, before, nil))
	})
	if err != nil {
		return dbgen.User{}, fmt.Errorf("archive user %s: %w", id, err)
	}
	return user, nil
}

func userEntry(action string, id uuid.UUID, before, after any) audit.Entry {
	return audit.Entry{
		Module: auditModule, EntityType: auditEntity, EntityID: id,
		Action: action, Before: before, After: after,
	}
}
