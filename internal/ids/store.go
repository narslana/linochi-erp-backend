package ids

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the ids module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

func (s *Store) ListIdsEntries(ctx context.Context) ([]IdsEntry, error) {
	rows, err := s.q.ListIdsEntries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]IdsEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToIdsEntry(r))
	}
	return out, nil
}

func (s *Store) SearchIdsEntries(ctx context.Context, q string) ([]IdsEntry, error) {
	rows, err := s.q.SearchIdsEntries(ctx, "%"+q+"%")
	if err != nil {
		return nil, err
	}
	out := make([]IdsEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToIdsEntry(r))
	}
	return out, nil
}

func (s *Store) GetIdsEntry(ctx context.Context, id uuid.UUID) (IdsEntry, error) {
	row, err := s.q.GetIdsEntry(ctx, id)
	if err != nil {
		return IdsEntry{}, err
	}
	return dbRowToIdsEntry(row), nil
}

func (s *Store) CreateIdsEntry(ctx context.Context, id uuid.UUID, category, name, value, issuer, owner, note, documentURL string, validFrom, validTo *time.Time) (IdsEntry, error) {
	var validFromPg, validToPg pgtype.Date
	if validFrom != nil {
		validFromPg = pgtype.Date{Time: *validFrom, Valid: true}
	}
	if validTo != nil {
		validToPg = pgtype.Date{Time: *validTo, Valid: true}
	}
	row, err := s.q.CreateIdsEntry(ctx, dbgen.CreateIdsEntryParams{
		ID:          id,
		Category:    category,
		Name:        name,
		Value:       value,
		Issuer:      issuer,
		ValidFrom:   validFromPg,
		ValidTo:     validToPg,
		Owner:       owner,
		Note:        note,
		DocumentUrl: stringPtr(documentURL),
	})
	if err != nil {
		return IdsEntry{}, err
	}
	return dbRowToIdsEntry(row), nil
}

func (s *Store) UpdateIdsEntry(ctx context.Context, category, name, value, issuer, owner, note, documentURL string, validFrom, validTo *time.Time, id uuid.UUID, version int) (IdsEntry, error) {
	var validFromPg, validToPg pgtype.Date
	if validFrom != nil {
		validFromPg = pgtype.Date{Time: *validFrom, Valid: true}
	}
	if validTo != nil {
		validToPg = pgtype.Date{Time: *validTo, Valid: true}
	}
	row, err := s.q.UpdateIdsEntry(ctx, dbgen.UpdateIdsEntryParams{
		Category:    category,
		Name:        name,
		Value:       value,
		Issuer:      issuer,
		ValidFrom:   validFromPg,
		ValidTo:     validToPg,
		Owner:       owner,
		Note:        note,
		DocumentUrl: stringPtr(documentURL),
		ID:          id,
		Version:     int32(version), //nolint:gosec // version is bounded by optimistic locking
	})
	if err != nil {
		return IdsEntry{}, err
	}
	return dbRowToIdsEntry(row), nil
}

func (s *Store) ArchiveIdsEntry(ctx context.Context, id uuid.UUID, version int) (IdsEntry, error) {
	row, err := s.q.ArchiveIdsEntry(ctx, dbgen.ArchiveIdsEntryParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return IdsEntry{}, err
	}
	return dbRowToIdsEntry(row), nil
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func dbRowToIdsEntry(r dbgen.IdsEntry) IdsEntry {
	e := IdsEntry{
		ID:        r.ID,
		Category:  r.Category,
		Name:      r.Name,
		Value:     r.Value,
		Issuer:    r.Issuer,
		Owner:     r.Owner,
		Note:      r.Note,
		Version:   int(r.Version),
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ValidFrom.Valid {
		s := r.ValidFrom.Time.Format("2006-01-02")
		e.ValidFrom = &s
	}
	if r.ValidTo.Valid {
		s := r.ValidTo.Time.Format("2006-01-02")
		e.ValidTo = &s
	}
	if r.DocumentUrl != nil {
		e.DocumentURL = r.DocumentUrl
	}
	return e
}
