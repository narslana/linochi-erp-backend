//go:build integration

package boards_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/boards"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

// server is shared across all tests in this package.
var server *testdb.Server

func TestMain(m *testing.M) {
	ctx := context.Background()
	s, err := testdb.StartServer(ctx)
	if err != nil {
		panic(err)
	}
	server = s
	m.Run()
	_ = server.Terminate(ctx)
}

// env holds the database, stores and handler for integration tests.
type env struct {
	pool      *pgxpool.Pool
	rec       *audit.Recorder
	sessions  *auth.SessionStore
	mw        *auth.SessionMiddleware
	boardsSVC *boards.Service
	boardsH   *boards.Handler
	logger    *slog.Logger
	tracer    *sdktrace.TracerProvider
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()

	// Start a test database with migrations.
	dsn := server.NewMigratedDatabase(t)
	appDSN := testdb.AppURL(dsn)

	// Open the app pool.
	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(ctx) })

	pool, err := db.Open(ctx, db.Options{URL: appDSN, MaxConns: 4, Metrics: metrics, Tracer: tp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	logs := &bytes.Buffer{}
	logger := obs.NewLogger(logs, slog.LevelWarn)
	rec, err := audit.NewRecorder(logger, metrics)
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := auth.NewSessionStore(pool, rec, metrics, auth.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	store := boards.NewStore(pool)
	svc := boards.NewService(store, rec, pool)
	mw := auth.NewSessionMiddleware(sessions)
	h := boards.NewHandler(svc, mw, logger)

	return &env{
		pool:      pool,
		rec:       rec,
		sessions:  sessions,
		mw:        mw,
		boardsSVC: svc,
		boardsH:   h,
		logger:    logger,
		tracer:    tp,
	}
}

// ─── TAV-01: Boards ─────────────────────────────────────────────────────────

func TestTAV01_ListBoards(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	// Create a board.
	_, err := e.boardsSVC.CreateBoard(ctx, "Test Board")
	if err != nil {
		t.Fatal(err)
	}

	// List boards.
	boards, err := e.boardsSVC.ListBoards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) == 0 {
		t.Fatal("expected at least one board")
	}
	if boards[0].Name != "Test Board" {
		t.Errorf("board name = %q, want %q", boards[0].Name, "Test Board")
	}
}

func TestTAV01_CreateBoard(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	board, err := e.boardsSVC.CreateBoard(ctx, "Project Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if board.Name != "Project Alpha" {
		t.Errorf("board name = %q, want %q", board.Name, "Project Alpha")
	}
	if board.ID == uuid.Nil {
		t.Error("board ID should not be nil")
	}
}

func TestTAV01_ArchiveBoard(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	board, err := e.boardsSVC.CreateBoard(ctx, "To Archive")
	if err != nil {
		t.Fatal(err)
	}

	archived, err := e.boardsSVC.ArchiveBoard(ctx, board.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !archived.Archived {
		t.Error("archived board should have Archived = true")
	}
}

// ─── TAV-02: Columns ────────────────────────────────────────────────────────

func TestTAV02_DefaultColumns(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	_, err := e.boardsSVC.CreateBoard(ctx, "Column Test")
	if err != nil {
		t.Fatal(err)
	}

	// List boards to get the board ID.
	boards, err := e.boardsSVC.ListBoards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) == 0 {
		t.Fatal("expected at least one board")
	}

	cols, err := e.boardsSVC.ListColumns(ctx, boards[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 {
		t.Fatalf("expected 3 default columns, got %d", len(cols))
	}
	expected := []string{"To do", "In progress", "Done"}
	for i, c := range cols {
		if c.Name != expected[i] {
			t.Errorf("column %d name = %q, want %q", i, c.Name, expected[i])
		}
	}
}

func TestTAV02_CreateColumn(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	col, err := e.boardsSVC.CreateColumn(ctx, boardID, "Review", nil)
	if err != nil {
		t.Fatal(err)
	}
	if col.Name != "Review" {
		t.Errorf("column name = %q, want %q", col.Name, "Review")
	}
}

func TestTAV02_UpdateColumn(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	col, err := e.boardsSVC.CreateColumn(ctx, boardID, "Old Name", nil)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := e.boardsSVC.UpdateColumn(ctx, col.ID, ptrString("New Name"), nil, col.Version)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "New Name" {
		t.Errorf("column name = %q, want %q", updated.Name, "New Name")
	}
}

func TestTAV02_ArchiveColumn(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	col, err := e.boardsSVC.CreateColumn(ctx, boardID, "To Archive", nil)
	if err != nil {
		t.Fatal(err)
	}

	archived, err := e.boardsSVC.ArchiveColumn(ctx, col.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !archived.Archived {
		t.Error("archived column should have Archived = true")
	}
}

// ─── TAV-03: Cards ──────────────────────────────────────────────────────────

func TestTAV03_CreateCard(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	col, err := e.boardsSVC.GetColumn(ctx, getFirstColumn(t, ctx, e, boardID))
	if err != nil {
		t.Fatal(err)
	}

	card, err := e.boardsSVC.CreateCard(ctx, boardID, col.ID, "Test Card", ptrString("Description"), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if card.Title != "Test Card" {
		t.Errorf("card title = %q, want %q", card.Title, "Test Card")
	}
	if card.BoardID != boardID {
		t.Error("card board ID mismatch")
	}
}

func TestTAV03_CardWithDueDate(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	col, err := e.boardsSVC.GetColumn(ctx, getFirstColumn(t, ctx, e, boardID))
	if err != nil {
		t.Fatal(err)
	}

	dueDate := ptrString("2027-02-01")
	card, err := e.boardsSVC.CreateCard(ctx, boardID, col.ID, "Due Soon", nil, nil, dueDate, nil)
	if err != nil {
		t.Fatal(err)
	}
	if card.DueDate == nil || *card.DueDate != "2027-02-01" {
		t.Errorf("card due date = %v, want %q", card.DueDate, "2027-02-01")
	}
}

// ─── TAV-04: Update Card (Move) ─────────────────────────────────────────────

func TestTAV04_MoveCard(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	col1, err := e.boardsSVC.CreateColumn(ctx, boardID, "Column A", nil)
	if err != nil {
		t.Fatal(err)
	}
	col2, err := e.boardsSVC.CreateColumn(ctx, boardID, "Column B", nil)
	if err != nil {
		t.Fatal(err)
	}

	card, err := e.boardsSVC.CreateCard(ctx, boardID, col1.ID, "Moving Card", nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := e.boardsSVC.UpdateCard(ctx, card.ID, nil, &col2.ID, nil, nil, nil, nil, card.Version)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ColumnID != col2.ID {
		t.Errorf("card column ID = %v, want %v", updated.ColumnID, col2.ID)
	}
}

// ─── TAV-05: Checklists ─────────────────────────────────────────────────────

func TestTAV05_CreateChecklistItem(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	cardID := createCard(t, ctx, e, boardID)

	item, err := e.boardsSVC.CreateChecklistItem(ctx, cardID, "First Item", nil)
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "First Item" {
		t.Errorf("item title = %q, want %q", item.Title, "First Item")
	}
	if item.Completed {
		t.Error("new checklist item should not be completed")
	}
}

func TestTAV05_UpdateChecklistItem(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	cardID := createCard(t, ctx, e, boardID)

	item, err := e.boardsSVC.CreateChecklistItem(ctx, cardID, "To Complete", nil)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := e.boardsSVC.UpdateChecklistItem(ctx, item.ID, nil, ptrBool(true), nil, item.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Completed {
		t.Error("updated item should be completed")
	}
}

func TestTAV05_ListChecklistItems(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	cardID := createCard(t, ctx, e, boardID)

	_, err := e.boardsSVC.CreateChecklistItem(ctx, cardID, "Item 1", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.boardsSVC.CreateChecklistItem(ctx, cardID, "Item 2", nil)
	if err != nil {
		t.Fatal(err)
	}

	items, err := e.boardsSVC.ListChecklistItems(ctx, cardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 checklist items, got %d", len(items))
	}
}

// ─── TAV-06: Comments ───────────────────────────────────────────────────────

func TestTAV06_CreateComment(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	cardID := createCard(t, ctx, e, boardID)

	// Create a user for the comment author.
	userID, err := insertTestUser(t, ctx, e.pool)
	if err != nil {
		t.Fatal(err)
	}

	comment, err := e.boardsSVC.CreateComment(ctx, cardID, userID, "Nice work!")
	if err != nil {
		t.Fatal(err)
	}
	if comment.BodyMD != "Nice work!" {
		t.Errorf("comment body = %q, want %q", comment.BodyMD, "Nice work!")
	}
	if comment.AuthorID != userID {
		t.Error("comment author ID mismatch")
	}
}

func TestTAV06_ListComments(t *testing.T) {
	ctx := audit.WithActor(context.Background(), audit.SystemActor("test"))
	e := newEnv(t)

	boardID := createBoard(t, ctx, e)
	cardID := createCard(t, ctx, e, boardID)

	userID, err := insertTestUser(t, ctx, e.pool)
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.boardsSVC.CreateComment(ctx, cardID, userID, "First comment")
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.boardsSVC.CreateComment(ctx, cardID, userID, "Second comment")
	if err != nil {
		t.Fatal(err)
	}

	comments, err := e.boardsSVC.ListComments(ctx, cardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func createBoard(t *testing.T, ctx context.Context, e *env) uuid.UUID {
	t.Helper()
	board, err := e.boardsSVC.CreateBoard(ctx, "Test Board")
	if err != nil {
		t.Fatal(err)
	}
	return board.ID
}

func getFirstColumn(t *testing.T, ctx context.Context, e *env, boardID uuid.UUID) uuid.UUID {
	t.Helper()
	cols, err := e.boardsSVC.ListColumns(ctx, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) == 0 {
		t.Fatal("expected at least one column")
	}
	return cols[0].ID
}

func createCard(t *testing.T, ctx context.Context, e *env, boardID uuid.UUID) uuid.UUID {
	t.Helper()
	colID := getFirstColumn(t, ctx, e, boardID)
	card, err := e.boardsSVC.CreateCard(ctx, boardID, colID, "Test Card", nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return card.ID
}

func ptrString(s string) *string { return &s }
func ptrBool(b bool) *bool       { return &b }

// insertTestUser creates a user in the database for comment authoring.
func insertTestUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (uuid.UUID, error) {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO users (id, oidc_issuer, oidc_subject, email, display_name, archived_at) VALUES ($1, $2, $3, $4, $5, NULL) ON CONFLICT DO NOTHING",
		id, "test", id.String()[:8], "test"+id.String()[:8]+"@linochi.eu", "Test User",
	)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// _ suppresses unused import warnings
var _ = io.Discard
