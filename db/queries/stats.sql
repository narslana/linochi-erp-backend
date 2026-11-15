-- name: OrdersPerMonth :many
-- STA-02: orders and revenue per month
SELECT
    TO_CHAR(created_at, 'YYYY-MM') AS month,
    COUNT(*) AS orders,
    SUM(total_sek) AS revenue
FROM orders
WHERE archived = false
  AND created_at >= now() - interval '24 months'
GROUP BY TO_CHAR(created_at, 'YYYY-MM')
ORDER BY month DESC;

-- name: RevenueByChannel :many
-- STA-02: revenue per sales channel
SELECT channel, SUM(total_sek) AS revenue
FROM orders
WHERE archived = false
GROUP BY channel
ORDER BY revenue DESC;

-- name: RevenueByCountry :many
-- STA-02: revenue per country
SELECT shipping_country AS country, SUM(total_sek) AS revenue
FROM orders
WHERE archived = false
GROUP BY shipping_country
ORDER BY revenue DESC;

-- name: MarginPerProduct :many
-- STA-02: margin per product
SELECT
    p.id AS product_id,
    p.name AS product_name,
    SUM(ol.quantity) AS total_quantity,
    SUM(ol.total_sek) AS revenue,
    SUM(ol.quantity * sp.purchase_price) AS cost,
    SUM(ol.total_sek) - SUM(ol.quantity * sp.purchase_price) AS margin
FROM order_lines ol
JOIN orders o ON o.id = ol.order_id AND o.archived = false
JOIN products p ON p.id = ol.product_id AND p.archived = false
LEFT JOIN supplier_products sp ON sp.product_id = ol.product_id AND sp.archived = false
GROUP BY p.id, p.name
ORDER BY margin DESC;

-- name: KeyFigures :one
-- STA-01, STA-02: summary key figures
SELECT
    (SELECT COUNT(*) FROM orders WHERE archived = false AND created_at >= date_trunc('year', now())) AS orders_this_year,
    (SELECT COALESCE(SUM(total_sek), 0) FROM orders WHERE archived = false AND created_at >= date_trunc('year', now())) AS revenue_this_year,
    (SELECT COUNT(DISTINCT shipping_country) FROM orders WHERE archived = false) AS countries,
    (SELECT COUNT(*) FROM customers WHERE archived = false) AS customers;

-- name: OrderToShippedTime :many
-- STA-04: average time from order to shipped
SELECT
    AVG(EXTRACT(EPOCH FROM (shipped_at - created_at)) / 3600) AS avg_hours,
    COUNT(*) AS total_orders
FROM orders
WHERE archived = false
  AND shipped_at IS NOT NULL
  AND created_at IS NOT NULL;

-- name: RepurchaseRate :one
-- STA-04: percentage of customers with 2+ orders
SELECT
    (SELECT COUNT(DISTINCT customer_id) FROM orders WHERE archived = false) AS total_customers,
    (SELECT COUNT(*) FROM (
        SELECT customer_id FROM orders WHERE archived = false GROUP BY customer_id HAVING COUNT(*) > 1
    ) sub) AS repurchasers;

-- name: SalesByCustomerType :many
-- STA-04: sales per customer type
SELECT
    c.type_code,
    COUNT(o.id) AS order_count,
    SUM(o.total_sek) AS revenue
FROM customers c
JOIN orders o ON o.customer_id = c.id AND o.archived = false
WHERE c.archived = false
GROUP BY c.type_code
ORDER BY revenue DESC;

-- name: InventoryTurnover :one
-- STA-03: inventory value, turnover rate, weeks of inventory
SELECT
    COALESCE(SUM(sm.quantity * sp.purchase_price), 0) AS inventory_value,
    COALESCE(SUM(ol.quantity * sp.purchase_price), 0) AS cost_of_goods_sold,
    CASE
        WHEN COALESCE(SUM(ol.quantity * sp.purchase_price), 0) = 0 THEN 0
        ELSE COALESCE(SUM(sm.quantity * sp.purchase_price), 0) / (SUM(ol.quantity * sp.purchase_price) / 365.0)
    END AS weeks_of_inventory
FROM stockmovements sm
LEFT JOIN supplier_products sp ON sp.product_id = sm.product_id AND sp.archived = false
LEFT JOIN order_lines ol ON ol.product_id = sm.product_id
JOIN orders o ON o.id = ol.order_id AND o.archived = false
WHERE sm.movement_type != 'reservation' AND sm.movement_type != 'release'
  AND sm.quantity > 0;

-- name: LowStockProducts :many
-- STA-01: products with low stock
SELECT
    p.id AS product_id,
    p.name AS product_name,
    COALESCE(SUM(CASE WHEN sm.movement_type != 'reservation' AND sm.movement_type != 'release' THEN sm.quantity ELSE 0 END), 0) AS current_stock
FROM products p
LEFT JOIN stockmovements sm ON sm.product_id = p.id
WHERE p.archived = false
GROUP BY p.id, p.name
HAVING SUM(CASE WHEN sm.movement_type != 'reservation' AND sm.movement_type != 'release' THEN sm.quantity ELSE 0 END) < 10
ORDER BY current_stock ASC;

-- name: OrdersByStatus :many
-- STA-01: orders waiting per status
SELECT status, COUNT(*) AS count
FROM orders
WHERE archived = false
GROUP BY status
ORDER BY count DESC;

-- name: ShipmentsOnTheirWay :many
-- STA-01: shipments on their way
SELECT o.id, o.channel, o.status, o.shipping_name, o.tracking_number, o.carrier
FROM orders o
WHERE o.archived = false
  AND o.status IN ('shipped', 'in_transit')
ORDER BY o.created_at DESC;

-- name: CardsDue :many
-- STA-01: cards (boards) that are due
SELECT b.id, b.name, COUNT(c.id) AS pending_cards
FROM boards b
JOIN cards c ON c.board_id = b.id AND c.archived = false
WHERE b.archived = false AND c.due_date IS NOT NULL AND c.due_date <= current_date
GROUP BY b.id, b.name
ORDER BY c.due_date ASC;

-- name: UpcomingReminders :many
-- REG-05: reminders due within next 30 days
SELECT id, title, title_en, due_date, reminder_type, notified
FROM reminders
WHERE archived = false
  AND due_date BETWEEN current_date AND current_date + 30
  AND notified = false
ORDER BY due_date ASC;

-- name: GetInventoryStats :one
-- STA-02: inventory value and low-stock count
SELECT
    COALESCE(SUM(sm.quantity * COALESCE(sp.purchase_price, 0)), 0) AS total_value,
    COALESCE(SUM(CASE WHEN COALESCE(SUM(sm.quantity), 0) < 10 THEN 1 ELSE 0 END), 0) AS low_stock_count
FROM products p
LEFT JOIN stockmovements sm ON sm.product_id = p.id AND sm.movement_type != 'reservation' AND sm.movement_type != 'release' AND sm.quantity > 0
LEFT JOIN supplier_products sp ON sp.product_id = sm.product_id AND sp.archived = false
WHERE p.archived = false
GROUP BY p.id;

-- name: GetInventoryStatsTotal :one
-- STA-02: total inventory value
SELECT COALESCE(SUM(sm.quantity * COALESCE(sp.purchase_price, 0)), 0) AS total_value
FROM stockmovements sm
LEFT JOIN supplier_products sp ON sp.product_id = sm.product_id AND sp.archived = false
WHERE sm.movement_type != 'reservation' AND sm.movement_type != 'release' AND sm.quantity > 0;

-- name: GetInventoryStatsLowStock :one
-- STA-02: count of products with low stock
SELECT COUNT(*) AS low_stock_count
FROM (
    SELECT p.id
    FROM products p
    LEFT JOIN stockmovements sm ON sm.product_id = p.id AND sm.movement_type != 'reservation' AND sm.movement_type != 'release' AND sm.quantity > 0
    WHERE p.archived = false
    GROUP BY p.id
    HAVING COALESCE(SUM(sm.quantity), 0) < 10
) sub;

-- name: GetInventoryStatsItems :many
-- STA-02: per-product inventory items
SELECT
    p.id AS product_id,
    p.name AS product_name,
    COALESCE(SUM(CASE WHEN sm.movement_type != 'reservation' AND sm.movement_type != 'release' THEN sm.quantity ELSE 0 END), 0) AS quantity,
    COALESCE(SUM(CASE WHEN sm.movement_type != 'reservation' AND sm.movement_type != 'release' THEN sm.quantity * COALESCE(sp.purchase_price, 0) ELSE 0 END), 0) AS value
FROM products p
LEFT JOIN stockmovements sm ON sm.product_id = p.id AND sm.movement_type != 'reservation' AND sm.movement_type != 'release'
LEFT JOIN supplier_products sp ON sp.product_id = sm.product_id AND sp.archived = false
WHERE p.archived = false
GROUP BY p.id, p.name
HAVING SUM(CASE WHEN sm.movement_type != 'reservation' AND sm.movement_type != 'release' THEN sm.quantity ELSE 0 END) > 0
ORDER BY value DESC;

-- name: GetStatsCards :many
-- STA-03: summary cards for dashboard
SELECT title, value, change
FROM (
    SELECT
        'Orders this month' AS title,
        COUNT(*)::text AS value,
        NULL AS change
    FROM orders
    WHERE archived = false
      AND created_at >= date_trunc('month', now())
    UNION ALL
    SELECT
        'Revenue this month' AS title,
        COALESCE(SUM(total_sek)::text, '0') AS value,
        NULL AS change
    FROM orders
    WHERE archived = false
      AND created_at >= date_trunc('month', now())
    UNION ALL
    SELECT
        'Low stock products' AS title,
        COUNT(*)::text AS value,
        NULL AS change
    FROM (
        SELECT p.id
        FROM products p
        LEFT JOIN stockmovements sm ON sm.product_id = p.id AND sm.movement_type != 'reservation' AND sm.movement_type != 'release' AND sm.quantity > 0
        WHERE p.archived = false
        GROUP BY p.id
        HAVING COALESCE(SUM(sm.quantity), 0) < 10
    ) sub
    UNION ALL
    SELECT
        'Orders in pipeline' AS title,
        COUNT(*)::text AS value,
        NULL AS change
    FROM orders
    WHERE archived = false
      AND status IN ('draft', 'confirmed', 'accepted')
) cards
ORDER BY title;

-- name: GetRevenueTrend :many
-- STA-01: revenue trend over time
SELECT
    TO_CHAR(created_at, 'YYYY-MM-DD') AS period,
    COUNT(*) AS orders,
    SUM(total_sek) AS revenue
FROM orders
WHERE archived = false
  AND created_at >= $1::date
  AND created_at < $2::date + interval '1 day'
GROUP BY TO_CHAR(created_at, 'YYYY-MM-DD')
ORDER BY period ASC;
