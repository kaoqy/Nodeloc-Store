package system

import (
	"context"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

// paidOrderStatuses are the order states that count towards revenue.
var paidOrderStatuses = []string{"paid", "completed"}

// RevenuePoint is one day of the dashboard revenue series.
type RevenuePoint struct {
	Date    string `json:"date"`
	Revenue int64  `json:"revenue"`
	Orders  int64  `json:"orders"`
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
}

// Stats aggregates the overview counters straight from the live database so the
// dashboard never renders placeholder numbers.
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
	if err := db.WithContext(ctx).Model(&models.Order{}).Count(&stats.OrdersTotal).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.Order{}).
		Where("status = ?", "pending").Count(&stats.OrdersPending).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.Order{}).
		Where("fulfillment_status = ?", "waiting_stock").Count(&stats.OrdersWaiting).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.Product{}).
		Where("is_archived = ?", false).Count(&stats.ProductsTotal).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.User{}).Count(&stats.UsersTotal).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.Card{}).
		Where("status = ?", "available").Count(&stats.CardsAvailable).Error; err != nil {
		return nil, err
	}

	type revenueRow struct {
		PaidAt    *time.Time
		CreatedAt time.Time
		Amount    int64
	}
	cutoff := startOfDay(time.Now()).AddDate(0, 0, -(days - 1))
	var rows []revenueRow
	if err := db.WithContext(ctx).Model(&models.Order{}).
		Select("paid_at, created_at, total_amount AS amount").
		Where("status IN ?", paidOrderStatuses).
		Where("COALESCE(paid_at, created_at) >= ?", cutoff).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	index := make(map[string]int, len(stats.RevenueSeries))
	for i, point := range stats.RevenueSeries {
		index[point.Date] = i
	}
	for _, row := range rows {
		at := row.CreatedAt
		if row.PaidAt != nil {
			at = *row.PaidAt
		}
		i, ok := index[at.Local().Format("2006-01-02")]
		if !ok {
			continue
		}
		stats.RevenueSeries[i].Revenue += row.Amount
		stats.RevenueSeries[i].Orders++
		stats.RevenuePeriod += row.Amount
	}

	return stats, nil
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
