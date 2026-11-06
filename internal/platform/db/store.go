package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// UserLogin is what the OIDC login gives us about a user.
type UserLogin struct {
	Issuer      string
	Subject     string
	Email       string
	DisplayName string
}

// Store is the typed access to the platform tables. Modules get their own
// store.go over the same pool and the same pattern.
type Store struct {
	q *dbgen.Queries
}

// NewStore creates a Store over the pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// UpsertUserOnLogin creates the user on the first login and refreshes the
// profile on later ones. The id is a UUIDv7 made here, used only for new rows.
func (s *Store) UpsertUserOnLogin(ctx context.Context, l UserLogin) (dbgen.User, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return dbgen.User{}, fmt.Errorf("new user id: %w", err)
	}
	u, err := s.q.UpsertUserOnLogin(ctx, dbgen.UpsertUserOnLoginParams{
		ID: id, OidcIssuer: l.Issuer, OidcSubject: l.Subject, Email: l.Email, DisplayName: l.DisplayName,
	})
	if err != nil {
		return dbgen.User{}, fmt.Errorf("upsert user: %w", err)
	}
	return u, nil
}

// GetUser returns ErrNotFound if there is no such user.
func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (dbgen.User, error) {
	u, err := s.q.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.User{}, ErrNotFound
	}
	if err != nil {
		return dbgen.User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// ArchiveUser hides the user (NFK-09: never deleted). version is the version
// the caller read: ErrStaleVersion if someone changed the row since, and
// ErrNotFound if there is no such user.
func (s *Store) ArchiveUser(ctx context.Context, id uuid.UUID, version int32) (dbgen.User, error) {
	u, err := s.q.ArchiveUser(ctx, dbgen.ArchiveUserParams{ID: id, Version: version})
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbgen.User{}, fmt.Errorf("archive user: %w", err)
	}
	// No row matched id and version: tell "unknown id" from "stale version".
	if _, err := s.q.GetUser(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return dbgen.User{}, ErrNotFound
	} else if err != nil {
		return dbgen.User{}, fmt.Errorf("archive user: %w", err)
	}
	return dbgen.User{}, ErrStaleVersion
}
