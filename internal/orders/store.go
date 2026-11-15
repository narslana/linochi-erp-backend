package orders

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Store wraps sqlc queries for the orders module.
type Store struct {
	q *dbgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: dbgen.New(pool)}
}

// ─── Orders ───────────────────────────────────────────────────────────────────

func (s *Store) ListOrders(ctx context.Context, status *string, customerID *uuid.UUID, fromDate, toDate *string) ([]Order, error) {
	var arg dbgen.ListOrdersParams
	if status != nil {
		arg.Status = *status
	}
	if customerID != nil {
		arg.CustomerID = *customerID
	}
	if fromDate != nil {
		if t, err := time.Parse(time.DateOnly, *fromDate); err == nil {
			arg.FromDate = pgtype.Date{Time: t, Valid: true}
		}
	}
	if toDate != nil {
		if t, err := time.Parse(time.DateOnly, *toDate); err == nil {
			arg.ToDate = pgtype.Date{Time: t, Valid: true}
		}
	}
	rows, err := s.q.ListOrders(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := make([]Order, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToOrder(r))
	}
	return out, nil
}

func (s *Store) GetOrder(ctx context.Context, id uuid.UUID) (Order, error) {
	row, err := s.q.GetOrder(ctx, id)
	if err != nil {
		return Order{}, err
	}
	return dbRowToOrder(row), nil
}

func (s *Store) GetOrderByExternalId(ctx context.Context, externalID string) (Order, error) {
	row, err := s.q.GetOrderByExternalId(ctx, externalID)
	if err != nil {
		return Order{}, err
	}
	return dbRowToOrder(row), nil
}

func (s *Store) CreateOrder(ctx context.Context, id, customerID uuid.UUID, externalID, channel, status, currency string, exchangeRateSek, subtotalSek, taxSek, freightSek, commissionSek, totalSek int64, shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry, billingName, billingAddress, billingCity, billingPostalCode, billingCountry, trackingNumber, carrier, shippingLabelUrl string, shippedAt, deliveredAt *string, notes string) (Order, error) {
	var deliveredAtTime *time.Time
	if deliveredAt != nil {
		t, _ := time.Parse(time.RFC3339, *deliveredAt)
		deliveredAtTime = &t
	}

	var exchangeRate pgtype.Numeric
	exchangeRate.Int = big.NewInt(exchangeRateSek)
	exchangeRate.Valid = true

	row, err := s.q.CreateOrder(ctx, dbgen.CreateOrderParams{
		ID:                 id,
		ExternalID:         externalID,
		Channel:            channel,
		Status:             status,
		Currency:           currency,
		ExchangeRateSek:    exchangeRate,
		SubtotalSek:        subtotalSek,
		TaxSek:             taxSek,
		FreightSek:         freightSek,
		CommissionSek:      commissionSek,
		TotalSek:           totalSek,
		CustomerID:         customerID,
		ShippingName:       shippingName,
		ShippingAddress:    shippingAddress,
		ShippingCity:       shippingCity,
		ShippingPostalCode: shippingPostalCode,
		ShippingCountry:    shippingCountry,
		BillingName:        billingName,
		BillingAddress:     billingAddress,
		BillingCity:        billingCity,
		BillingPostalCode:  billingPostalCode,
		BillingCountry:     billingCountry,
		TrackingNumber:     trackingNumber,
		Carrier:            carrier,
		ShippingLabelUrl:   shippingLabelUrl,
		DeliveredAt:        deliveredAtTime,
		Notes:              notes,
	})
	if err != nil {
		return Order{}, err
	}
	return dbRowToOrder(row), nil
}

func (s *Store) UpdateOrder(ctx context.Context, externalID, channel, status, currency, trackingNumber, carrier, shippingLabelUrl, notes *string, exchangeRateSek, subtotalSek, taxSek, freightSek, commissionSek, totalSek *int64, customerID *uuid.UUID, shippingName, shippingAddress, shippingCity, shippingPostalCode, shippingCountry, billingName, billingAddress, billingCity, billingPostalCode, billingCountry *string, shippedAt, deliveredAt *string, id uuid.UUID, version int) (Order, error) {
	var params dbgen.UpdateOrderParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if externalID != nil {
		params.ExternalID = *externalID
	}
	if channel != nil {
		params.Channel = *channel
	}
	if status != nil {
		params.Status = *status
	}
	if currency != nil {
		params.Currency = *currency
	}
	if exchangeRateSek != nil {
		var er pgtype.Numeric
		er.Int = big.NewInt(*exchangeRateSek)
		er.Valid = true
		params.ExchangeRateSek = er
	}
	if subtotalSek != nil {
		params.SubtotalSek = *subtotalSek
	}
	if taxSek != nil {
		params.TaxSek = *taxSek
	}
	if freightSek != nil {
		params.FreightSek = *freightSek
	}
	if commissionSek != nil {
		params.CommissionSek = *commissionSek
	}
	if totalSek != nil {
		params.TotalSek = *totalSek
	}
	if customerID != nil {
		params.CustomerID = *customerID
	}
	if shippingName != nil {
		params.ShippingName = *shippingName
	}
	if shippingAddress != nil {
		params.ShippingAddress = *shippingAddress
	}
	if shippingCity != nil {
		params.ShippingCity = *shippingCity
	}
	if shippingPostalCode != nil {
		params.ShippingPostalCode = *shippingPostalCode
	}
	if shippingCountry != nil {
		params.ShippingCountry = *shippingCountry
	}
	if billingName != nil {
		params.BillingName = *billingName
	}
	if billingAddress != nil {
		params.BillingAddress = *billingAddress
	}
	if billingCity != nil {
		params.BillingCity = *billingCity
	}
	if billingPostalCode != nil {
		params.BillingPostalCode = *billingPostalCode
	}
	if billingCountry != nil {
		params.BillingCountry = *billingCountry
	}
	if trackingNumber != nil {
		params.TrackingNumber = *trackingNumber
	}
	if carrier != nil {
		params.Carrier = *carrier
	}
	if shippingLabelUrl != nil {
		params.ShippingLabelUrl = *shippingLabelUrl
	}
	if notes != nil {
		params.Notes = *notes
	}

	row, err := s.q.UpdateOrder(ctx, params)
	if err != nil {
		return Order{}, err
	}
	return dbRowToOrder(row), nil
}

func (s *Store) ArchiveOrder(ctx context.Context, id uuid.UUID, version int) (Order, error) {
	row, err := s.q.ArchiveOrder(ctx, dbgen.ArchiveOrderParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return Order{}, err
	}
	return dbRowToOrder(row), nil
}

func (s *Store) UpdateOrderStatus(ctx context.Context, status string, shippedAt, deliveredAt *string, trackingNumber, carrier *string, id uuid.UUID, version int) (Order, error) {
	var sp, dp *time.Time
	if shippedAt != nil {
		t, _ := time.Parse(time.RFC3339, *shippedAt)
		sp = &t
	}
	if deliveredAt != nil {
		t, _ := time.Parse(time.RFC3339, *deliveredAt)
		dp = &t
	}

	var tn, c string
	if trackingNumber != nil {
		tn = *trackingNumber
	}
	if carrier != nil {
		c = *carrier
	}

	row, err := s.q.UpdateOrderStatus(ctx, dbgen.UpdateOrderStatusParams{
		ID:             id,
		Status:         status,
		Version:        int32(version), //nolint:gosec // version is bounded by optimistic locking
		ShippedAt:      sp,
		DeliveredAt:    dp,
		TrackingNumber: tn,
		Carrier:        c,
	})
	if err != nil {
		return Order{}, err
	}
	return dbRowToOrder(row), nil
}

// ─── Order Lines ──────────────────────────────────────────────────────────────

func (s *Store) ListOrderLines(ctx context.Context, orderID uuid.UUID) ([]OrderLine, error) {
	rows, err := s.q.ListOrderLines(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]OrderLine, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbRowToOrderLine(r))
	}
	return out, nil
}

func (s *Store) CreateOrderLine(ctx context.Context, id, orderID, productID uuid.UUID, quantity, quantityShipped, unitPriceSek, taxAmountSek, totalSek int64, taxRate float64) (OrderLine, error) {
	var taxRateNumeric pgtype.Numeric
	taxRateNumeric.Int = big.NewInt(int64(taxRate * 100))
	taxRateNumeric.Valid = true

	row, err := s.q.CreateOrderLine(ctx, dbgen.CreateOrderLineParams{
		ID:              id,
		OrderID:         orderID,
		ProductID:       productID,
		Quantity:        int32(quantity),        //nolint:gosec // quantity is bounded by business logic
		QuantityShipped: int32(quantityShipped), //nolint:gosec // quantity shipped is bounded by business logic
		UnitPriceSek:    unitPriceSek,
		TaxRate:         taxRateNumeric,
		TaxAmountSek:    taxAmountSek,
		TotalSek:        totalSek,
	})
	if err != nil {
		return OrderLine{}, err
	}
	return dbRowToOrderLine(row), nil
}

func (s *Store) UpdateOrderLine(ctx context.Context, quantity, quantityShipped *int, unitPriceSek, taxAmountSek, totalSek *int64, taxRate *float64, id uuid.UUID, version int) (OrderLine, error) {
	var params dbgen.UpdateOrderLineParams
	params.ID = id
	params.Version = int32(version) //nolint:gosec // version is bounded by optimistic locking
	if quantity != nil {
		params.Quantity = int32(*quantity) //nolint:gosec // quantity is bounded by business logic
	}
	if quantityShipped != nil {
		params.QuantityShipped = int32(*quantityShipped) //nolint:gosec // quantity shipped is bounded by business logic
	}
	if unitPriceSek != nil {
		params.UnitPriceSek = *unitPriceSek
	}
	if taxRate != nil {
		var tr pgtype.Numeric
		tr.Int = big.NewInt(int64(*taxRate * 100))
		tr.Valid = true
		params.TaxRate = tr
	}
	if taxAmountSek != nil {
		params.TaxAmountSek = *taxAmountSek
	}
	if totalSek != nil {
		params.TotalSek = *totalSek
	}
	row, err := s.q.UpdateOrderLine(ctx, params)
	if err != nil {
		return OrderLine{}, err
	}
	return dbRowToOrderLine(row), nil
}

func (s *Store) ArchiveOrderLine(ctx context.Context, id uuid.UUID, version int) (OrderLine, error) {
	row, err := s.q.ArchiveOrderLine(ctx, dbgen.ArchiveOrderLineParams{ID: id, Version: int32(version)}) //nolint:gosec // version is bounded by optimistic locking
	if err != nil {
		return OrderLine{}, err
	}
	return dbRowToOrderLine(row), nil
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func dbRowToOrder(r dbgen.Order) Order {
	var exchangeRateSek int64
	if r.ExchangeRateSek.Valid && r.ExchangeRateSek.Int != nil {
		exchangeRateSek = r.ExchangeRateSek.Int.Int64()
	}
	o := Order{
		ID:                 r.ID,
		ExternalID:         r.ExternalID,
		Channel:            r.Channel,
		Status:             r.Status,
		Currency:           r.Currency,
		ExchangeRateSek:    exchangeRateSek,
		SubtotalSek:        r.SubtotalSek,
		TaxSek:             r.TaxSek,
		FreightSek:         r.FreightSek,
		CommissionSek:      r.CommissionSek,
		TotalSek:           r.TotalSek,
		CustomerID:         r.CustomerID,
		ShippingName:       r.ShippingName,
		ShippingAddress:    r.ShippingAddress,
		ShippingCity:       r.ShippingCity,
		ShippingPostalCode: r.ShippingPostalCode,
		ShippingCountry:    r.ShippingCountry,
		BillingName:        r.BillingName,
		BillingAddress:     r.BillingAddress,
		BillingCity:        r.BillingCity,
		BillingPostalCode:  r.BillingPostalCode,
		BillingCountry:     r.BillingCountry,
		TrackingNumber:     r.TrackingNumber,
		Carrier:            r.Carrier,
		ShippingLabelUrl:   r.ShippingLabelUrl,
		Version:            int(r.Version),
		CreatedAt:          r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ShippedAt != nil {
		t := r.ShippedAt.Format(time.RFC3339)
		o.ShippedAt = &t
	}
	if r.DeliveredAt != nil {
		t := r.DeliveredAt.Format(time.RFC3339)
		o.DeliveredAt = &t
	}
	if r.ArchivedAt != nil {
		o.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		o.ArchivedAt = &t
	}
	return o
}

func dbRowToOrderLine(r dbgen.OrderLine) OrderLine {
	var taxRate float64
	if r.TaxRate.Valid && r.TaxRate.Int != nil {
		taxRate = float64(r.TaxRate.Int.Int64()) / 100
	}
	l := OrderLine{
		ID:              r.ID,
		OrderID:         r.OrderID,
		ProductID:       r.ProductID,
		Quantity:        int(r.Quantity),
		QuantityShipped: int(r.QuantityShipped),
		UnitPriceSek:    r.UnitPriceSek,
		TaxRate:         taxRate,
		TaxAmountSek:    r.TaxAmountSek,
		TotalSek:        r.TotalSek,
		Version:         int(r.Version),
		CreatedAt:       r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       r.UpdatedAt.Format(time.RFC3339),
	}
	if r.ArchivedAt != nil {
		l.Archived = true
		t := r.ArchivedAt.Format(time.RFC3339)
		l.ArchivedAt = &t
	}
	return l
}
