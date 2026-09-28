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

// BuyerStat is one row of the top spenders panel.
type BuyerStat struct {
	UserID  uint   `json:"user_id"`
	Name    string `json:"name"`
	Orders  int64  `json:"orders"`
	Revenue int64  `json:"revenue"`
}

// CategoryStat is one row of the 分类销售 panel: which kind of goods the
// period's money came from.
type CategoryStat struct {
	Name     string `json:"name"`
	Products int64  `json:"products"`
	Orders   int64  `json:"orders"`
	Revenue  int64  `json:"revenue"`
}

// CouponStat is one row of the 优惠码 usage panel.
type CouponStat struct {
	CouponID uint   `json:"coupon_id"`
	Code     string `json:"code"`
	Uses     int64  `json:"uses"`
	Discount int64  `json:"discount"`
	Revenue  int64  `json:"revenue"`
}

// CardHealth is the key inventory behind the shop: how many are left, how many
// went out, and which product is closest to empty.
type CardHealth struct {
	Available int64               `json:"available"`
	Sold      int64               `json:"sold"`
	Disabled  int64               `json:"disabled"`
	Total     int64               `json:"total"`
	ByProduct []CardProductHealth `json:"by_product"`
}

// CardProductHealth is one product's key stock line.
type CardProductHealth struct {
	ProductID   uint    `json:"product_id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Available   int64   `json:"available"`
	Sold        int64   `json:"sold"`
	Disabled    int64   `json:"disabled"`
	SellThrough float64 `json:"sell_through"`
}

// Engagement counts the buyer-side activity that is not money: 签到, points and
// how many accounts actually carry a NodeLoc binding.
type Engagement struct {
	Checkins     int64 `json:"checkins_period"`
	CheckinUsers int64 `json:"checkin_users_period"`
	PointsIssued int64 `json:"points_issued_period"`
	PointsHeld   int64 `json:"points_held"`
	BoundUsers   int64 `json:"bound_users"`
	ActiveWeek   int64 `json:"active_week"`
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
	PaidPrev        int64   `json:"paid_prev"`
	PaidDelta       float64 `json:"paid_delta"`
	OrdersPrev      int64   `json:"orders_prev"`
	NewUsersPrev    int64   `json:"new_users_prev"`
	NewUsersDelta   float64 `json:"new_users_delta"`
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
	TopBuyers    []BuyerStat   `json:"top_buyers"`
	StockAlerts  []StockAlert  `json:"stock_alerts"`
	Funnel       []FunnelCount `json:"funnel"`
	RecentOrders []RecentOrder `json:"recent_orders"`

	// StockAlertThreshold is the 库存告急 line the shop owner set, so the panel
	// explains itself instead of hardcoding a number in the client.
	StockAlertThreshold int64          `json:"stock_alert_threshold"`
	CategorySales       []CategoryStat `json:"category_sales"`
	TopCoupons          []CouponStat   `json:"top_coupons"`
	CouponUses          int64          `json:"coupon_uses_period"`
	CouponDiscount      int64          `json:"coupon_discount_period"`
	CouponsTotal        int64          `json:"coupons_total"`
	CouponsActive       int64          `json:"coupons_active"`
	CardHealth          CardHealth     `json:"card_health"`
	Engagement          Engagement     `json:"engagement"`
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

	// 库存告急 is the shop's own number now; the panel shows it next to the list.
	stats.StockAlertThreshold = int64(lowStockAt)
	if rt, err := s.currentRuntime(); err == nil && rt != nil {
		stats.StockAlertThreshold = int64(rt.Features.AlertThreshold())
	}

	for _, step := range []func(context.Context, *gorm.DB, *DashboardStats, time.Time) error{
		statTrend, statPreviousRevenue, statBestSellers, statTopBuyers, statStockAlerts, statFunnel, statRecentOrders,
		statCategories, statCoupons, statCardHealth, statEngagement,
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

// statPreviousRevenue fills the window immediately before the reported one, so
// the dashboard can show 环比 for revenue, paid orders and new accounts. It
// attributes days by creation date exactly like statTrend does; mixing paid_at
// here would compare two different cohorts.
func statPreviousRevenue(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	from := cutoff.AddDate(0, 0, -stats.PeriodDays)

	var prev struct {
		Revenue int64
		Orders  int64
		Paid    int64
	}
	// Deleted rows must be spelled out: this query goes through Table("orders"),
	// which has no model for GORM to attach soft-delete scope to.
	err := db.WithContext(ctx).Table("orders").
		Select(`COALESCE(SUM(CASE WHEN status IN ? THEN total_amount ELSE 0 END), 0) AS revenue,
			COUNT(*) AS orders,
			COALESCE(SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END), 0) AS paid`,
			paidOrderStatuses, paidOrderStatuses).
		Where("created_at >= ? AND created_at < ?", from, cutoff).
		Where("deleted_at IS NULL").Scan(&prev).Error
	if err != nil {
		return err
	}
	stats.RevenuePrev = prev.Revenue
	stats.OrdersPrev = prev.Orders
	stats.PaidPrev = prev.Paid
	stats.RevenueDelta = periodDelta(stats.RevenuePeriod, prev.Revenue)
	stats.PaidDelta = periodDelta(stats.PaidPeriod, prev.Paid)

	var joined int64
	if err := db.WithContext(ctx).Model(&models.User{}).
		Where("created_at >= ? AND created_at < ?", from, cutoff).Count(&joined).Error; err != nil {
		return err
	}
	stats.NewUsersPrev = joined
	stats.NewUsersDelta = periodDelta(stats.NewUsers, joined)
	return nil
}

// periodDelta reads a zero baseline as "nothing to compare against": growth from
// nothing is reported as a full new period rather than as an infinite jump.
func periodDelta(current, previous int64) float64 {
	switch {
	case previous > 0:
		return float64(current-previous) / float64(previous) * 100
	case current > 0:
		return 100
	}
	return 0
}

func statTopBuyers(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	type row struct {
		UserID  uint
		Name    string
		Orders  int64
		Revenue int64
	}
	var rows []row
	err := db.WithContext(ctx).Table("orders").
		Select(`orders.user_id AS user_id, COALESCE(users.username, '') AS name,
			COUNT(*) AS orders, COALESCE(SUM(orders.total_amount), 0) AS revenue`).
		Joins("LEFT JOIN users ON users.id = orders.user_id").
		Where("orders.status IN ?", paidOrderStatuses).
		Where("orders.created_at >= ?", cutoff).
		Where("orders.deleted_at IS NULL").
		Group("orders.user_id, users.username").
		Order("revenue DESC").Limit(5).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		stats.TopBuyers = append(stats.TopBuyers, BuyerStat(item))
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
		Having("available <= ?", stats.StockAlertThreshold).
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

// statCategories attributes the period's paid revenue to each category.
func statCategories(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	type row struct {
		Name     string
		Products int64
		Orders   int64
		Revenue  int64
	}
	var rows []row
	// Deleted orders have to be excluded by hand: Table("orders") carries no
	// model for GORM to attach the soft-delete scope to.
	err := db.WithContext(ctx).Table("orders").
		Select(`COALESCE(categories.name, '未分类') AS name,
			COUNT(DISTINCT orders.product_id) AS products,
			COUNT(*) AS orders,
			COALESCE(SUM(orders.total_amount), 0) AS revenue`).
		Joins("LEFT JOIN products ON products.id = orders.product_id").
		Joins("LEFT JOIN categories ON categories.id = products.category_id").
		Where("orders.status IN ?", paidOrderStatuses).
		Where("orders.created_at >= ?", cutoff).
		Where("orders.deleted_at IS NULL").
		Group("COALESCE(categories.name, '未分类')").
		Order("revenue DESC").Limit(8).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		stats.CategorySales = append(stats.CategorySales, CategoryStat(item))
	}
	return nil
}

// statCoupons reports what the promotions cost and which code was spent most.
// The numbers come off the orders, not coupons.used_count, so a cancelled or
// refunded order cannot inflate what the shop gave away.
func statCoupons(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	var totals struct {
		Uses     int64
		Discount int64
	}
	err := db.WithContext(ctx).Table("orders").
		Select("COUNT(*) AS uses, COALESCE(SUM(discount_amount), 0) AS discount").
		Where("coupon_code <> ''").
		Where("status IN ?", paidOrderStatuses).
		Where("created_at >= ?", cutoff).
		Where("deleted_at IS NULL").Scan(&totals).Error
	if err != nil {
		return err
	}
	stats.CouponUses = totals.Uses
	stats.CouponDiscount = totals.Discount

	type row struct {
		CouponID uint
		Code     string
		Uses     int64
		Discount int64
		Revenue  int64
	}
	var rows []row
	err = db.WithContext(ctx).Table("orders").
		Select(`MAX(orders.coupon_id) AS coupon_id, orders.coupon_code AS code,
			COUNT(*) AS uses, COALESCE(SUM(orders.discount_amount), 0) AS discount,
			COALESCE(SUM(orders.total_amount), 0) AS revenue`).
		Where("orders.coupon_code <> ''").
		Where("orders.status IN ?", paidOrderStatuses).
		Where("orders.created_at >= ?", cutoff).
		Where("orders.deleted_at IS NULL").
		Group("orders.coupon_code").
		Order("discount DESC").Limit(5).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		stats.TopCoupons = append(stats.TopCoupons, CouponStat(item))
	}
	if err := db.WithContext(ctx).Model(&models.Coupon{}).Count(&stats.CouponsTotal).Error; err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&models.Coupon{}).
		Where("is_active = ?", true).Count(&stats.CouponsActive).Error
}

// statCardHealth counts the keys behind the shop and breaks the biggest stock
// lines down per product, so 卡密 shows where the inventory actually sits.
func statCardHealth(ctx context.Context, db *gorm.DB, stats *DashboardStats, _ time.Time) error {
	var health struct {
		Available int64
		Sold      int64
		Disabled  int64
		Total     int64
	}
	err := db.WithContext(ctx).Table("cards").
		Select(`COALESCE(SUM(CASE WHEN status = 'available' THEN 1 ELSE 0 END), 0) AS available,
			COALESCE(SUM(CASE WHEN status = 'sold' THEN 1 ELSE 0 END), 0) AS sold,
			COALESCE(SUM(CASE WHEN status = 'disabled' THEN 1 ELSE 0 END), 0) AS disabled,
			COUNT(*) AS total`).
		Where("deleted_at IS NULL").Scan(&health).Error
	if err != nil {
		return err
	}
	stats.CardHealth.Available = health.Available
	stats.CardHealth.Sold = health.Sold
	stats.CardHealth.Disabled = health.Disabled
	stats.CardHealth.Total = health.Total

	type row struct {
		ProductID uint
		Name      string
		Slug      string
		Available int64
		Sold      int64
		Disabled  int64
	}
	var rows []row
	err = db.WithContext(ctx).Table("cards").
		Select(`cards.product_id AS product_id, products.name AS name, products.slug AS slug,
			COALESCE(SUM(CASE WHEN cards.status = 'available' THEN 1 ELSE 0 END), 0) AS available,
			COALESCE(SUM(CASE WHEN cards.status = 'sold' THEN 1 ELSE 0 END), 0) AS sold,
			COALESCE(SUM(CASE WHEN cards.status = 'disabled' THEN 1 ELSE 0 END), 0) AS disabled`).
		Joins("LEFT JOIN products ON products.id = cards.product_id").
		Where("cards.deleted_at IS NULL").
		Group("cards.product_id, products.name, products.slug").
		Order("sold DESC").Limit(6).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		line := CardProductHealth{
			ProductID: item.ProductID,
			Name:      item.Name,
			Slug:      item.Slug,
			Available: item.Available,
			Sold:      item.Sold,
			Disabled:  item.Disabled,
		}
		// Sell-through ignores disabled keys: a retired batch is not a demand
		// signal, and counting it would make a cleaned-up product look sold out.
		if moving := item.Available + item.Sold; moving > 0 {
			line.SellThrough = float64(item.Sold) / float64(moving) * 100
		}
		stats.CardHealth.ByProduct = append(stats.CardHealth.ByProduct, line)
	}
	return nil
}

// statEngagement measures the buyer-side habits that keep an account coming
// back: 签到, the points in circulation and how many accounts came through
// NodeLoc.
func statEngagement(ctx context.Context, db *gorm.DB, stats *DashboardStats, cutoff time.Time) error {
	var checkins struct {
		Count int64
		Users int64
	}
	err := db.WithContext(ctx).Table("check_ins").
		Select("COUNT(*) AS count, COUNT(DISTINCT user_id) AS users").
		Where("created_at >= ?", cutoff).
		Where("deleted_at IS NULL").Scan(&checkins).Error
	if err != nil {
		return err
	}
	stats.Engagement.Checkins = checkins.Count
	stats.Engagement.CheckinUsers = checkins.Users

	var issued int64
	err = db.WithContext(ctx).Table("point_ledgers").
		Where("delta > 0 AND created_at >= ? AND deleted_at IS NULL", cutoff).
		Select("COALESCE(SUM(delta), 0)").Scan(&issued).Error
	if err != nil {
		return err
	}
	stats.Engagement.PointsIssued = issued

	var held int64
	if err := db.WithContext(ctx).Model(&models.User{}).
		Select("COALESCE(SUM(points), 0)").Scan(&held).Error; err != nil {
		return err
	}
	stats.Engagement.PointsHeld = held

	var bound int64
	// 绑定 NodeLoc is counted on the link table: it is what a binding really is,
	// and the denormalised column on users carries GORM's spelled-out name
	// (o_auth_provider), which is easy to hand-write wrong and impossible to rename.
	if err := db.WithContext(ctx).Table("oauth_identities").
		Where("deleted_at IS NULL").
		Distinct("user_id").Count(&bound).Error; err != nil {
		return err
	}
	stats.Engagement.BoundUsers = bound

	var active int64
	if err := db.WithContext(ctx).Model(&models.User{}).
		Where("last_login_at >= ?", time.Now().AddDate(0, 0, -7)).Count(&active).Error; err != nil {
		return err
	}
	stats.Engagement.ActiveWeek = active
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
