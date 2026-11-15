package calendar

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the calendar module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// ─── iCal Sources ─────────────────────────────────────────────────────────────

func (s *Store) ListICalSources(ctx context.Context) ([]ICalSource, error) {
	rows, err := s.q.ListICalSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ICalSource, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToICalSource(r))
	}
	return out, nil
}

func (s *Store) GetICalSource(ctx context.Context, id uuid.UUID) (ICalSource, error) {
	row, err := s.q.GetICalSource(ctx, id)
	if err != nil {
		return ICalSource{}, err
	}
	return dbRowToICalSource(row), nil
}

func (s *Store) CreateICalSource(ctx context.Context, id uuid.UUID, name, nameEn, icalURL, color string, syncInterval int) (ICalSource, error) {
	row, err := s.q.CreateICalSource(ctx, dbgen.CreateICalSourceParams{
		ID:           id,
		Name:         name,
		NameEn:       nameEn,
		IcalUrl:      icalURL,
		Color:        color,
		SyncInterval: int32(syncInterval), //nolint:gosec // sync interval is bounded
	})
	if err != nil {
		return ICalSource{}, err
	}
	return dbRowToICalSource(row), nil
}

func (s *Store) UpdateICalSource(ctx context.Context, name, nameEn, icalURL, color *string, syncInterval *int, enabled *bool, id uuid.UUID, version int) (ICalSource, error) {
	var params dbgen.UpdateICalSourceParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded
	if name != nil {
		params.Name = *name
	}
	if nameEn != nil {
		params.NameEn = *nameEn
	}
	if icalURL != nil {
		params.IcalUrl = *icalURL
	}
	if color != nil {
		params.Color = *color
	}
	if syncInterval != nil {
		params.SyncInterval = int32(*syncInterval) //nolint:gosec
	}
	if enabled != nil {
		params.Enabled = *enabled
	}
	row, err := s.q.UpdateICalSource(ctx, params)
	if err != nil {
		return ICalSource{}, err
	}
	return dbRowToICalSource(row), nil
}

func (s *Store) ArchiveICalSource(ctx context.Context, id uuid.UUID, version int) (ICalSource, error) {
	row, err := s.q.ArchiveICalSource(ctx, dbgen.ArchiveICalSourceParams{ID: id, Version: int32(version)}) //nolint:gosec
	if err != nil {
		return ICalSource{}, err
	}
	return dbRowToICalSource(row), nil
}

// ─── Calendar Events ──────────────────────────────────────────────────────────

func (s *Store) ListCalendarEvents(ctx context.Context, startTime, endTime time.Time) ([]CalendarEvent, error) {
	rows, err := s.q.ListCalendarEvents(ctx, dbgen.ListCalendarEventsParams{
		StartTime: startTime,
		EndTime:   endTime,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CalendarEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToCalendarEvent(r))
	}
	return out, nil
}

func (s *Store) GetCalendarEvent(ctx context.Context, id uuid.UUID) (CalendarEvent, error) {
	row, err := s.q.GetCalendarEvent(ctx, id)
	if err != nil {
		return CalendarEvent{}, err
	}
	return dbRowToCalendarEvent(row), nil
}

func (s *Store) UpsertCalendarEvent(ctx context.Context, id uuid.UUID, sourceType string, sourceID, icalUID *string, title, description string, startTime, endTime time.Time, allDay bool, color, erpLinkType string, erpLinkID *string) (CalendarEvent, error) {
	var srcID, linkID *uuid.UUID
	if sourceID != nil {
		parsed, err := uuid.Parse(*sourceID)
		if err == nil {
			srcID = &parsed
		}
	}
	if erpLinkID != nil {
		parsed, err := uuid.Parse(*erpLinkID)
		if err == nil {
			linkID = &parsed
		}
	}

	var icalUIDStr *string
	if icalUID != nil {
		icalUIDStr = icalUID
	}

	var erpLinkTypeStr *string
	if erpLinkType != "" {
		erpLinkTypeStr = &erpLinkType
	}

	row, err := s.q.UpsertCalendarEvent(ctx, dbgen.UpsertCalendarEventParams{
		ID:          id,
		SourceType:  sourceType,
		SourceID:    srcID,
		IcalUid:     icalUIDStr,
		Title:       title,
		Description: description,
		StartTime:   startTime,
		EndTime:     endTime,
		AllDay:      allDay,
		Color:       color,
		ErpLinkType: erpLinkTypeStr,
		ErpLinkID:   linkID,
	})
	if err != nil {
		return CalendarEvent{}, err
	}
	return dbRowToCalendarEvent(row), nil
}

func (s *Store) DeleteCalendarEventByICalUID(ctx context.Context, icalUID string) error {
	return s.q.DeleteCalendarEventByICalUID(ctx, &icalUID)
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToICalSource(r dbgen.IcalSource) ICalSource {
	s := ICalSource{
		ID:           r.ID,
		Name:         r.Name,
		NameEn:       r.NameEn,
		ICalURL:      r.IcalUrl,
		Color:        r.Color,
		SyncInterval: int(r.SyncInterval),
		Enabled:      r.Enabled,
		Version:      int(r.Version),
		CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		s.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		s.ArchivedAt = &t
	}
	return s
}

func dbRowToCalendarEvent(r dbgen.CalendarEvent) CalendarEvent {
	e := CalendarEvent{
		ID:          r.ID,
		SourceType:  r.SourceType,
		Title:       r.Title,
		Description: r.Description,
		StartTime:   r.StartTime.Format(time.RFC3339),
		EndTime:     r.EndTime.Format(time.RFC3339),
		AllDay:      r.AllDay,
		Color:       r.Color,
		Version:     int(r.Version),
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
	if r.SourceID != nil {
		e.SourceID = r.SourceID
	}
	if r.IcalUid != nil {
		s := *r.IcalUid
		e.ICalUID = &s
	}
	if r.ErpLinkID != nil {
		s := r.ErpLinkID.String()
		e.ERPLinkID = &s
	}
	if r.ArchivedAt != nil {
		e.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		e.ArchivedAt = &t
	}
	if r.ErpLinkType != nil {
		e.ERPLinkType = r.ErpLinkType
	}
	return e
}
