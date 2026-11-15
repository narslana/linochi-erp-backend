package stats

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the stats module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

func (s *Store) KeyFigures(ctx context.Context) (dbgen.KeyFiguresRow, error) {
	return s.q.KeyFigures(ctx)
}

func (s *Store) OrdersPerMonth(ctx context.Context) ([]dbgen.OrdersPerMonthRow, error) {
	return s.q.OrdersPerMonth(ctx)
}

func (s *Store) RevenueByChannel(ctx context.Context) ([]dbgen.RevenueByChannelRow, error) {
	return s.q.RevenueByChannel(ctx)
}

func (s *Store) RevenueByCountry(ctx context.Context) ([]dbgen.RevenueByCountryRow, error) {
	return s.q.RevenueByCountry(ctx)
}

func (s *Store) MarginPerProduct(ctx context.Context) ([]dbgen.MarginPerProductRow, error) {
	return s.q.MarginPerProduct(ctx)
}

func (s *Store) OrderToShippedTime(ctx context.Context) ([]dbgen.OrderToShippedTimeRow, error) {
	return s.q.OrderToShippedTime(ctx)
}

func (s *Store) RepurchaseRate(ctx context.Context) (dbgen.RepurchaseRateRow, error) {
	return s.q.RepurchaseRate(ctx)
}

func (s *Store) SalesByCustomerType(ctx context.Context) ([]dbgen.SalesByCustomerTypeRow, error) {
	return s.q.SalesByCustomerType(ctx)
}

func (s *Store) InventoryTurnover(ctx context.Context) (dbgen.InventoryTurnoverRow, error) {
	return s.q.InventoryTurnover(ctx)
}

func (s *Store) LowStockProducts(ctx context.Context) ([]dbgen.LowStockProductsRow, error) {
	return s.q.LowStockProducts(ctx)
}

func (s *Store) OrdersByStatus(ctx context.Context) ([]dbgen.OrdersByStatusRow, error) {
	return s.q.OrdersByStatus(ctx)
}

func (s *Store) ShipmentsOnTheirWay(ctx context.Context) ([]dbgen.ShipmentsOnTheirWayRow, error) {
	return s.q.ShipmentsOnTheirWay(ctx)
}

func (s *Store) CardsDue(ctx context.Context) ([]dbgen.CardsDueRow, error) {
	return s.q.CardsDue(ctx)
}
