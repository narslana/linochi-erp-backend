package stats

import (
	"context"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Service holds the business logic for statistics (STA-01 to STA-04).
type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

// keyFiguresInt64 extracts an int64 from an interface{} (nullable SUM).
func keyFiguresInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case *int64:
		if val == nil {
			return 0
		}
		return *val
	case *float64:
		if val == nil {
			return 0
		}
		return int64(*val)
	default:
		return 0
	}
}

// keyFiguresFloat64 extracts a float64 from an interface{} (nullable AVG).
func keyFiguresFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	case *float64:
		if val == nil {
			return 0
		}
		return *val
	default:
		return 0
	}
}

// keyFiguresInt extracts an int from an interface{}.
func keyFiguresInt(v interface{}) int {
	return int(keyFiguresInt64(v))
}

// KeyFigures returns summary statistics (STA-01, STA-02).
func (s *Service) KeyFigures(ctx context.Context) (KeyFigures, error) {
	row, err := s.store.KeyFigures(ctx)
	if err != nil {
		return KeyFigures{}, err
	}
	return KeyFigures{
		OrdersThisYear:  int(row.OrdersThisYear),
		RevenueThisYear: keyFiguresInt64(row.RevenueThisYear),
		Countries:       int(row.Countries),
		Customers:       int(row.Customers),
	}, nil
}

// OrdersPerMonth returns monthly order and revenue data (STA-02).
func (s *Service) OrdersPerMonth(ctx context.Context) ([]OrderPerMonth, error) {
	rows, err := s.store.OrdersPerMonth(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]OrderPerMonth, 0, len(rows))
	for _, r := range rows {
		out = append(out, OrderPerMonth{
			Month:   r.Month,
			Orders:  int(r.Orders),
			Revenue: keyFiguresInt64(r.Revenue),
		})
	}
	return out, nil
}

// RevenueByChannel returns revenue per sales channel (STA-02).
func (s *Service) RevenueByChannel(ctx context.Context) ([]RevenueByChannel, error) {
	rows, err := s.store.RevenueByChannel(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RevenueByChannel, 0, len(rows))
	for _, r := range rows {
		out = append(out, RevenueByChannel{
			Channel: r.Channel,
			Revenue: keyFiguresInt64(r.Revenue),
		})
	}
	return out, nil
}

// RevenueByCountry returns revenue per shipping country (STA-02).
func (s *Service) RevenueByCountry(ctx context.Context) ([]RevenueByCountry, error) {
	rows, err := s.store.RevenueByCountry(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RevenueByCountry, 0, len(rows))
	for _, r := range rows {
		out = append(out, RevenueByCountry{
			Country: r.Country,
			Revenue: keyFiguresInt64(r.Revenue),
		})
	}
	return out, nil
}

// MarginPerProduct returns product margin data (STA-02).
func (s *Service) MarginPerProduct(ctx context.Context) ([]MarginPerProduct, error) {
	rows, err := s.store.MarginPerProduct(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MarginPerProduct, 0, len(rows))
	for _, r := range rows {
		out = append(out, MarginPerProduct{
			ProductID:     r.ProductID,
			ProductName:   r.ProductName,
			TotalQuantity: int(r.TotalQuantity),
			Revenue:       r.Revenue,
			Cost:          r.Cost,
			Margin:        int64(r.Margin),
		})
	}
	return out, nil
}

// OrderToShippedTime returns average fulfillment time (STA-04).
func (s *Service) OrderToShippedTime(ctx context.Context) (OrderToShippedTime, error) {
	rows, err := s.store.OrderToShippedTime(ctx)
	if err != nil {
		return OrderToShippedTime{}, err
	}
	if len(rows) == 0 {
		return OrderToShippedTime{}, nil
	}
	r := rows[0]
	return OrderToShippedTime{
		AvgHours:    r.AvgHours,
		TotalOrders: int(r.TotalOrders),
	}, nil
}

// RepurchaseRate returns customer retention rate (STA-04).
func (s *Service) RepurchaseRate(ctx context.Context) (RepurchaseRate, error) {
	row, err := s.store.RepurchaseRate(ctx)
	if err != nil {
		return RepurchaseRate{}, err
	}
	return RepurchaseRate{
		TotalCustomers: int(row.TotalCustomers),
		Repurchasers:   int(row.Repurchasers),
	}, nil
}

// SalesByCustomerType returns sales per customer type (STA-04).
func (s *Service) SalesByCustomerType(ctx context.Context) ([]SalesByCustomerType, error) {
	rows, err := s.store.SalesByCustomerType(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SalesByCustomerType, 0, len(rows))
	for _, r := range rows {
		out = append(out, SalesByCustomerType{
			TypeCode:   r.TypeCode,
			OrderCount: int(r.OrderCount),
			Revenue:    keyFiguresInt64(r.Revenue),
		})
	}
	return out, nil
}

// InventoryTurnover returns inventory metrics (STA-03).
func (s *Service) InventoryTurnover(ctx context.Context) (InventoryTurnover, error) {
	row, err := s.store.InventoryTurnover(ctx)
	if err != nil {
		return InventoryTurnover{}, err
	}
	return InventoryTurnover{
		InventoryValue:   keyFiguresInt64(row.InventoryValue),
		CostOfGoodsSold:  keyFiguresInt64(row.CostOfGoodsSold),
		WeeksOfInventory: keyFiguresFloat64(row.WeeksOfInventory),
	}, nil
}

// LowStockProducts returns products with low inventory (STA-01).
func (s *Service) LowStockProducts(ctx context.Context) ([]LowStockProduct, error) {
	rows, err := s.store.LowStockProducts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LowStockProduct, 0, len(rows))
	for _, r := range rows {
		out = append(out, LowStockProduct{
			ProductID:    r.ProductID,
			ProductName:  r.ProductName,
			CurrentStock: keyFiguresInt(r.CurrentStock),
		})
	}
	return out, nil
}

// OrdersByStatus returns orders grouped by status (STA-01).
func (s *Service) OrdersByStatus(ctx context.Context) ([]OrderByStatus, error) {
	rows, err := s.store.OrdersByStatus(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]OrderByStatus, 0, len(rows))
	for _, r := range rows {
		out = append(out, OrderByStatus{
			Status: r.Status,
			Count:  int(r.Count),
		})
	}
	return out, nil
}

// ShipmentsOnTheirWay returns active shipments (STA-01).
func (s *Service) ShipmentsOnTheirWay(ctx context.Context) ([]ShipmentOnTheirWay, error) {
	rows, err := s.store.ShipmentsOnTheirWay(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ShipmentOnTheirWay, 0, len(rows))
	for _, r := range rows {
		out = append(out, ShipmentOnTheirWay{
			ID:             r.ID,
			Channel:        r.Channel,
			Status:         r.Status,
			ShippingName:   r.ShippingName,
			TrackingNumber: r.TrackingNumber,
			Carrier:        r.Carrier,
		})
	}
	return out, nil
}

// CardsDue returns boards with due cards (STA-01).
func (s *Service) CardsDue(ctx context.Context) ([]dbgen.CardsDueRow, error) {
	return s.store.CardsDue(ctx)
}
