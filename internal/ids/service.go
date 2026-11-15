package ids

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
)

// Service holds the business logic for the numbers and IDs registry (REG-01, REG-02, REG-03).
type Service struct {
	store *Store
	rec   *audit.Recorder
	pool  *pgxpool.Pool
}

func NewService(store *Store, rec *audit.Recorder, pool *pgxpool.Pool) *Service {
	return &Service{store: store, rec: rec, pool: pool}
}

func (s *Service) ListIdsEntries(ctx context.Context) ([]IdsEntry, error) {
	return s.store.ListIdsEntries(ctx)
}

func (s *Service) SearchIdsEntries(ctx context.Context, q string) ([]IdsEntry, error) {
	return s.store.SearchIdsEntries(ctx, q)
}

func (s *Service) GetIdsEntry(ctx context.Context, id uuid.UUID) (IdsEntry, error) {
	e, err := s.store.GetIdsEntry(ctx, id)
	if err != nil {
		return e, ProblemError(err)
	}
	return e, nil
}

func (s *Service) CreateIdsEntry(ctx context.Context, category, name, value, issuer, owner, note, documentURL string, validFrom, validTo *time.Time) (IdsEntry, error) {
	id := uuid.New()
	e, err := s.store.CreateIdsEntry(ctx, id, category, name, value, issuer, owner, note, documentURL, validFrom, validTo)
	if err != nil {
		return e, err
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "ids_entries",
		EntityType: "ids_entry",
		EntityID:   id,
		Action:     "Create",
		After:      map[string]any{"category": category, "name": name, "value": value},
		Reason:     actor.ID.String(),
	})
	return e, nil
}

func (s *Service) UpdateIdsEntry(ctx context.Context, id uuid.UUID, category, name, value, issuer, owner, note, documentURL string, validFrom, validTo *time.Time, version int) (IdsEntry, error) {
	e, err := s.store.GetIdsEntry(ctx, id)
	if err != nil {
		return e, ProblemError(err)
	}
	if version == 0 {
		version = e.Version
	}
	updated, err := s.store.UpdateIdsEntry(ctx, category, name, value, issuer, owner, note, documentURL, validFrom, validTo, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "ids_entries",
		EntityType: "ids_entry",
		EntityID:   id,
		Action:     "Update",
		After:      map[string]any{"category": category, "name": name, "value": value},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}

func (s *Service) ArchiveIdsEntry(ctx context.Context, id uuid.UUID, version int) (IdsEntry, error) {
	e, err := s.store.GetIdsEntry(ctx, id)
	if err != nil {
		return e, ProblemError(err)
	}
	if version == 0 {
		version = e.Version
	}
	updated, err := s.store.ArchiveIdsEntry(ctx, id, version)
	if err != nil {
		return updated, ProblemError(err)
	}
	actor, _ := audit.ActorFrom(ctx)
	_ = s.rec.Record(ctx, nil, audit.Entry{
		Module:     "ids_entries",
		EntityType: "ids_entry",
		EntityID:   id,
		Action:     "Archive",
		Before:     map[string]any{"name": e.Name, "value": e.Value},
		Reason:     actor.ID.String(),
	})
	return updated, nil
}
