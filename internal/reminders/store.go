package reminders

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the reminders module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

func (s *Store) ListReminders(ctx context.Context) ([]Reminder, error) {
	rows, err := s.q.ListReminders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToReminder(r))
	}
	return out, nil
}

func (s *Store) GetReminder(ctx context.Context, id uuid.UUID) (Reminder, error) {
	row, err := s.q.GetReminder(ctx, id)
	if err != nil {
		return Reminder{}, err
	}
	return dbRowToReminder(row), nil
}

func (s *Store) CreateReminder(ctx context.Context, id, idsEntryID, externalRefID uuid.UUID, reminderType, externalRefType, title, titleEn, description string, dueDate string) (Reminder, error) {
	var dDate pgtype.Date
	t, _ := time.Parse("2006-01-02", dueDate)
	dDate = pgtype.Date{Time: t, Valid: true}

	var extRefType *string
	if externalRefType != "" {
		extRefType = &externalRefType
	}

	row, err := s.q.CreateReminder(ctx, dbgen.CreateReminderParams{
		ID:              id,
		ReminderType:    reminderType,
		IdsEntryID:      &idsEntryID,
		ExternalRefType: extRefType,
		ExternalRefID:   &externalRefID,
		Title:           title,
		TitleEn:         titleEn,
		Description:     description,
		DueDate:         dDate,
	})
	if err != nil {
		return Reminder{}, err
	}
	return dbRowToReminder(row), nil
}

func (s *Store) UpdateReminder(ctx context.Context, title, titleEn, description *string, dueDate *string, id uuid.UUID, version int) (Reminder, error) {
	var params dbgen.UpdateReminderParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if title != nil {
		params.Title = *title
	}
	if titleEn != nil {
		params.TitleEn = *titleEn
	}
	if description != nil {
		params.Description = *description
	}
	if dueDate != nil {
		var dDate pgtype.Date
		t, _ := time.Parse("2006-01-02", *dueDate)
		dDate = pgtype.Date{Time: t, Valid: true}
		params.DueDate = dDate
	}
	row, err := s.q.UpdateReminder(ctx, params)
	if err != nil {
		return Reminder{}, err
	}
	return dbRowToReminder(row), nil
}

func (s *Store) ArchiveReminder(ctx context.Context, id uuid.UUID, version int) (Reminder, error) {
	row, err := s.q.ArchiveReminder(ctx, dbgen.ArchiveReminderParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Reminder{}, err
	}
	return dbRowToReminder(row), nil
}

func (s *Store) MarkReminderNotified(ctx context.Context, id uuid.UUID, version int) (Reminder, error) {
	row, err := s.q.MarkReminderNotified(ctx, dbgen.MarkReminderNotifiedParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Reminder{}, err
	}
	return dbRowToReminder(row), nil
}

func (s *Store) ExpiredReminders(ctx context.Context) ([]Reminder, error) {
	rows, err := s.q.ExpiredReminders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToReminder(r))
	}
	return out, nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToReminder(r dbgen.Reminder) Reminder {
	rem := Reminder{
		ID:             r.ID,
		ReminderType:   r.ReminderType,
		Title:          r.Title,
		TitleEn:        r.TitleEn,
		Description:    r.Description,
		DueDate:        r.DueDate.Time.Format("2006-01-02"),
		Notified:       r.Notified,
		Version:        int(r.Version),
		CreatedAt:      r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      r.UpdatedAt.Format(time.RFC3339),
	}
	if r.IdsEntryID != nil {
		rem.IdsEntryID = r.IdsEntryID
	}
	if r.ExternalRefID != nil {
		rem.ExternalRefID = r.ExternalRefID
	}
	if r.ExternalRefType != nil {
		rem.ExternalRefType = *r.ExternalRefType
	}
	if r.NotifiedAt != nil {
		t := r.NotifiedAt.Format(time.RFC3339)
		rem.NotifiedAt = &t
	}
	if r.ArchivedAt != nil {
		rem.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		rem.ArchivedAt = &t
	}
	return rem
}
