package system

import (
	"context"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"gorm.io/gorm"
)

// paidOrderStatuses are the order states that count towards revenue.
var paidOrderStatuses = []string{"paid", "completed"}

// lowStockAt flags a card product whose remaining stock cannot cover a couple
// more sales; it is a review prompt, not a reorder point.
const lowStockAt = 3

// RevenuePoint is one day of the dashboard trend.
type RevenuePoint struct {
	Date    string `json:"date"`
	Revenue int64  `json:"revenue"`
	Orders  int64  `json:"orders"`
	Users   int64  `json:"users"`
}

// ProductStat is one row of the best sellers panel.
type ProductStat struct {
	ProductID uint   `json:"product_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Orders    int64  `json:"orders"`
	Revenue   int64  `json:"revenue"`
}

// StockAlert is a card product that is about to run out.
type StockAlert struct {
	ProductID uint   `json:"product_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Available int64  `json:"available"`
	Sold      int64  `json:"sold"`
}

// FunnelCount is one stage of the order pipeline.
type FunnelCount struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// RecentOrder is a compact row for the live activity panel.
type RecentOrder struct {
	OrderNo           string     `json:"order_no"`
	Product           string     `json:"product"`
	Buyer             string     `json:"buyer"`
	Amount            int        `json:"amount"`
	Status            string     `json:"status"`
	FulfillmentStatus string     `json:"fulfillment_status"`
	CreatedAt         time.Time  `json:"created_at"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
}

// DashboardStats carries the real aggregates the admin overview renders.
type DashboardStats struct {
	RevenueTotal   int64          `json:"revenue_total"`
	RevenuePeriod  int64          `json:"revenue_period"`
	OrdersTotal    int64          `json:"orders_total"`
	OrdersPending  int64          `json:"orders_pending"`
	OrdersWaiting  int64          `json:"orders_waiting"`
	ProductsTotal  int64          `json:"products_total"`
	UsersTotal     int64          `json:"users_total"`
	CardsAvailable int64          `json:"cards_available"`
	PeriodDays     int            `json:"period_days"`
	RevenueSeries  []RevenuePoint `json:"revenue_series"`

	RevenuePrev     int64   `json:"revenue_prev"`
	RevenueDelta    float64 `json:"revenue_delta"`
	OrdersPeriod    int64   `json:"orders_period"`
	PaidPeriod      int64   `json:"paid_period"`
	Conversion      float64 `json:"conversion"`
	Aov             int64   `json:"aov"`
	RefundedPeriod  int64   `json:"refunded_period"`
	ManualPending   int64   `json:"orders_manual_pending"`
	DeliveredPeriod int64   `json:"delivered_period"`
	NewUsers        int64   `json:"new_users_period"`
	ActiveBuyers    int64   `json:"active_buyers_period"`
	RepeatBuyers    int64   `json:"repeat_buyers_period"`

	TopProducts  []ProductStat `json:"top_products"`
	StockAlerts  []StockAlert  `json:"stock_alerts"`
	Funnel       []FunnelCount `json:"funnel"`
	RecentOrders []RecentOrder `json:"recent_orders"`
}

// Stats aggregates the overview numbers straight from the live database, so the
// dashboard never renders placeholders. Day bucketing happens in Go because
// SQLite and MySQL spell their date functions differently.
func (s *Service) Stats(ctx context.Context, days int) (*DashboardStats, error) {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil, ErrNotInstalled
	}
	if days <= 0 {
		days = 30
	}
	if days > 90 {
		days = 90
	}

	stats := &DashboardStats{PeriodDays: days, RevenueSeries: revenueBuckets(days)}
	if err := db.WithContext(ctx).Model(&models.Order{}).
		Where("status IN ?", paidOrderStatuses).
		Select("COALESCE(SUM(total_amount), 0)").Scan(&stats.RevenueTotal).Error; err != nil {
		return nil, err
	}
	orders := func(where string, args ...any) func(*gorm.DB) *gorm.DB {
		return func(tx *gorm.DB) *gorm.DB {
			m := tx.Model(&models.Order{})
			if where == "" {
				return m
			}
			return m.Where(where, args...)
		}
	}
	counts := []struct {
		query func(tx *gorm.DB) *gorm.DB
		dest  *int64
	}{
		{orders(""), &stats.OrdersTotal},
		{orders("status = ?", "pending"), &stats.OrdersPending},
		{orders("fulfillment_status = ?", "waiting_stock"), &stats.OrdersWaiting},
		{orders("fulfillment_status = ?", "manual_pending"), &stats.ManualPending},
		{func(tx *gorm.DB) *gorm.DB { return tx.Model(&models.Product{}).Where("is_archived = ?", false) }, &stats.ProductsTotal},
		{func(tx *gorm.DB) *gorm.DB { return tx.Model(&models.User{}) }, &stats.UsersTotal},
		{func(tx *gorm.DB) *gorm.DB { return tx.Model(&models.Card{}).Where("status = ?", "available") }, &stats.CardsAvailable},
	}
	for _, item := range counts {
		if err := item.query(db.WithContext(ctx)).Count(item.dest).Error; err != nil {
			return nil, err
		}
	}

	latest := startOfDay(time.Now())
	cutoff := latest.AddDate(0, 0, -(days - 1))

	for _, step := range []func(context.Context, *gorm.DB, *DashboardStats, time.Time) error{
		statTrend, statPreviousRevenue, statBestSellers, statStockAlerts, statFunnel, statRecentOrders,
	} {
		if err := step(ctx, db, stats, cutoff); err != nil {
			return nil, err
		}
	}
	return stats, nil
}

// statTrend fills the daily series and every counter derived from the period's
// orders and new accounts. A day is attributed to the order's creation date, so
// 下单量, 支付转化 and 营收 all describe the same cohort.
func statTrend(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	type orderRow struct {
		UserID            uint
		Amount            int
		Status            string
		FulfillmentStatus string
		CreatedAt         time.Time
	}
	var orders []orderRow
	if err := db.WithContext(ctx).Model(&models.Order{}).
		Select("user_id, total_amount AS amount, status, fulfillment_status, created_at").
		Where("created_at >= ?", cutoff).Scan(&orders).Error; err != nil {
		return err
	}

	var joined []time.Time
	if err := db.WithContext(ctx).Model(&models.User{}).
		Where("created_at >= ?", cutoff).Pluck("created_at", &joined).Error; err != nil {
		return err
	}

	index := make(map[string]int, len(stats.RevenueSeries))
	for i, point := range stats.RevenueSeries {
		index[point.Date] = i
	}
	for _, at := range joined {
		if i, ok := index[at.Local().Format("2006-01-02")]; ok {
			stats.RevenueSeries[i].Users++
		}
		stats.NewUsers++
	}

	buyers := make(map[uint]int, len(orders))
	for _, row := range orders {
		stats.OrdersPeriod++
		if row.Status == "refunded" {
			stats.RefundedPeriod++
		}
		paid := row.Status == "paid" || row.Status == "completed"
		if !paid {
			continue
		}
		stats.PaidPeriod++
		buyers[row.UserID]++
		if row.FulfillmentStatus == "delivered" || row.FulfillmentStatus == "completed" {
			stats.DeliveredPeriod++
		}
		at := row.CreatedAt
		stats.RevenuePeriod += int64(row.Amount)
		if i, ok := index[at.Local().Format("2006-01-02")]; ok {
			stats.RevenueSeries[i].Revenue += int64(row.Amount)
			stats.RevenueSeries[i].Orders++
		}
	}

	for _, ordersByBuyer := range buyers {
		if ordersByBuyer > 1 {
			stats.RepeatBuyers++
		}
	}
	stats.ActiveBuyers = int64(len(buyers))
	if stats.PaidPeriod > 0 {
		stats.Aov = stats.RevenuePeriod / stats.PaidPeriod
	}
	if stats.OrdersPeriod > 0 {
		stats.Conversion = float64(stats.PaidPeriod) / float64(stats.OrdersPeriod) * 100
	}
	return nil
}

// statPreviousRevenue sums the window immediately before the reported one, so
// the dashboard can show 环比. It attributes days by creation date exactly like
// statTrend does; mixing paid_at here would compare two different cohorts.
func statPreviousRevenue(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	from := cutoff.AddDate(0, 0, -stats.PeriodDays)
	var prev int64
	err := db.WithContext(ctx).Model(&models.Order{}).
		Where("status IN ?", paidOrderStatuses).
		Where("created_at >= ? AND created_at < ?", from, cutoff).
		Select("COALESCE(SUM(total_amount), 0)").Scan(&prev).Error
	if err != nil {
		return err
	}
	stats.RevenuePrev = prev
	switch {
	case prev > 0:
		stats.RevenueDelta = float64(stats.RevenuePeriod-prev) / float64(prev) * 100
	case stats.RevenuePeriod > 0:
		stats.RevenueDelta = 100
	}
	return nil
}

func statBestSellers(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	type row struct {
		ProductID uint
		Name      string
		Slug      string
		Orders    int64
		Revenue   int64
	}
	var rows []row
	err := db.WithContext(ctx).Table("orders").
		Select(`orders.product_id AS product_id, products.name AS name, products.slug AS slug,
			COUNT(*) AS orders, COALESCE(SUM(orders.total_amount), 0) AS revenue`).
		Joins("LEFT JOIN products ON products.id = orders.product_id").
		Where("orders.status IN ?", paidOrderStatuses).
		Where("orders.created_at >= ?", cutoff).
		Group("orders.product_id, products.name, products.slug").
		Order("revenue DESC").Limit(6).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		stats.TopProducts = append(stats.TopProducts, ProductStat(item))
	}
	return nil
}

func statStockAlerts(ctx context.Context, db *gorm.DB, stats *DashboardStats, _ time.Time) error {
	type row struct {
		ProductID uint
		Name      string
		Slug      string
		Available int64
		Sold      int64
	}
	var rows []row
	err := db.WithContext(ctx).Table("products").
		Select(`products.id AS product_id, products.name AS name, products.slug AS slug,
			COALESCE(SUM(CASE WHEN cards.status = 'available' THEN 1 ELSE 0 END), 0) AS available,
			COALESCE(SUM(CASE WHEN cards.status = 'sold' THEN 1 ELSE 0 END), 0) AS sold`).
		Joins("LEFT JOIN cards ON cards.product_id = products.id AND cards.deleted_at IS NULL").
		Where("products.deleted_at IS NULL AND products.is_archived = ? AND products.product_type = ?", false, "card").
		Group("products.id, products.name, products.slug").
		Having("available <= ?", int64(lowStockAt)).
		Order("available ASC").Limit(6).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		stats.StockAlerts = append(stats.StockAlerts, StockAlert(item))
	}
	return nil
}

func statFunnel(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	if err := db.WithContext(ctx).Model(&models.Order{}).
		Select("status, COUNT(*) AS count").
		Where("created_at >= ?", cutoff).
		Group("status").Scan(&rows).Error; err != nil {
		return err
	}
	counts := make(map[string]int64, len(rows))
	for _, item := range rows {
		counts[item.Status] = item.Count
	}
	for _, stage := range []struct{ key, label string }{
		{"pending", "待支付"},
		{"paid", "已支付"},
		{"completed", "已完成"},
		{"cancelled", "已取消"},
		{"refunded", "已退款"},
		{"failed", "交易失败"},
	} {
		if count, ok := counts[stage.key]; ok && count > 0 {
			stats.Funnel = append(stats.Funnel, FunnelCount{Key: stage.key, Label: stage.label, Count: count})
		}
	}
	return nil
}

func statRecentOrders(ctx context.Context, db *gorm.DB, stats *DashboardStats, _ time.Time) error {
	var orders []models.Order
	if err := db.WithContext(ctx).Preload("Product").Preload("User").
		Order("id DESC").Limit(8).Find(&orders).Error; err != nil {
		return err
	}
	for _, order := range orders {
		item := RecentOrder{
			OrderNo:           order.OrderNo,
			Amount:            order.TotalAmount,
			Status:            order.Status,
			FulfillmentStatus: order.FulfillmentStatus,
			CreatedAt:         order.CreatedAt,
			PaidAt:            order.PaidAt,
		}
		if order.Product != nil {
			item.Product = order.Product.Name
		}
		if order.User != nil {
			item.Buyer = order.User.Username
		}
		stats.RecentOrders = append(stats.RecentOrders, item)
	}
	return nil
}

func revenueBuckets(days int) []RevenuePoint {
	latest := startOfDay(time.Now())
	series := make([]RevenuePoint, days)
	for i := range series {
		series[i].Date = latest.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
	}
	return series
}

func startOfDay(value time.Time) time.Time {
	local := value.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}
