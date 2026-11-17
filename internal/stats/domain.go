package stats

import (
	"github.com/google/uuid"
)

// KeyFigures holds summary statistics (STA-01, STA-02).
type KeyFigures struct {
	OrdersThisYear  int   `json:"orders_this_year"`
	RevenueThisYear int64 `json:"revenue_this_year"`
	Countries       int   `json:"countries"`
	Customers       int   `json:"customers"`
}

// OrderPerMonth represents monthly order and revenue data (STA-02).
type OrderPerMonth struct {
	Month   string `json:"month"`
	Orders  int    `json:"orders"`
	Revenue int64  `json:"revenue"`
}

// RevenueByChannel represents revenue per sales channel (STA-02).
type RevenueByChannel struct {
	Channel string `json:"channel"`
	Revenue int64  `json:"revenue"`
}

// RevenueByCountry represents revenue per shipping country (STA-02).
type RevenueByCountry struct {
	Country string `json:"country"`
	Revenue int64  `json:"revenue"`
}

// MarginPerProduct represents product margin data (STA-02).
type MarginPerProduct struct {
	ProductID     uuid.UUID `json:"product_id"`
	ProductName   string    `json:"product_name"`
	TotalQuantity int       `json:"total_quantity"`
	Revenue       int64     `json:"revenue"`
	Cost          int64     `json:"cost"`
	Margin        int64     `json:"margin"`
}

// OrderToShippedTime represents average fulfillment time (STA-04).
type OrderToShippedTime struct {
	AvgHours    float64 `json:"avg_hours"`
	TotalOrders int     `json:"total_orders"`
}

// RepurchaseRate represents customer retention (STA-04).
type RepurchaseRate struct {
	TotalCustomers int `json:"total_customers"`
	Repurchasers   int `json:"repurchasers"`
}

// SalesByCustomerType represents sales per customer type (STA-04).
type SalesByCustomerType struct {
	TypeCode   string `json:"type_code"`
	OrderCount int    `json:"order_count"`
	Revenue    int64  `json:"revenue"`
}

// InventoryTurnover represents inventory metrics (STA-03).
type InventoryTurnover struct {
	InventoryValue   int64   `json:"inventory_value"`
	CostOfGoodsSold  int64   `json:"cost_of_goods_sold"`
	WeeksOfInventory float64 `json:"weeks_of_inventory"`
}

// LowStockProduct represents a product with low inventory (STA-01).
type LowStockProduct struct {
	ProductID    uuid.UUID `json:"product_id"`
	ProductName  string    `json:"product_name"`
	CurrentStock int       `json:"current_stock"`
}

// OrderByStatus represents orders grouped by status (STA-01).
type OrderByStatus struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// ShipmentOnTheirWay represents an active shipment (STA-01).
type ShipmentOnTheirWay struct {
	ID             uuid.UUID `json:"id"`
	Channel        string    `json:"channel"`
	Status         string    `json:"status"`
	ShippingName   string    `json:"shipping_name"`
	TrackingNumber string    `json:"tracking_number,omitempty"`
	Carrier        string    `json:"carrier,omitempty"`
}

// InventoryStats holds inventory statistics (STA-02).
type InventoryStats struct {
	Items         []InventoryStatsItem `json:"items"`
	LowStockCount int                  `json:"low_stock_count"`
	TotalValue    int64                `json:"total_value"`
}

// InventoryStatsItem represents a product's inventory in the stats.
type InventoryStatsItem struct {
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Quantity    int       `json:"quantity"`
	Value       int64     `json:"value"`
}

// StatsCard represents a summary card for the dashboard (STA-03).
type StatsCard struct {
	Title  string  `json:"title"`
	Value  string  `json:"value"`
	Change *string `json:"change,omitempty"`
}

// RevenuePoint represents a point in the revenue trend (STA-01).
type RevenuePoint struct {
	Period  string `json:"period"`
	Orders  int    `json:"orders"`
	Revenue int    `json:"revenue"`
}
