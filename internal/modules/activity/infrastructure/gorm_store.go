package infrastructure

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/domain"
)

// GormStore 是活动模块的持久化适配器。
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) base() *gorm.DB {
	// 已删除的活动不从默认查询里返回；ActivityStatusDeleted 是软删除标记。
	return s.db.Model(&models.Activity{}).Where("status <> ?", models.ActivityStatusDeleted)
}

func (s *GormStore) List(ctx context.Context, filter domain.ListFilter) ([]domain.ActivityView, int64, error) {
	query := s.base()
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if kind := strings.TrimSpace(filter.Type); kind != "" {
		query = query.Where("type = ?", kind)
	}
	keyword := strings.TrimSpace(filter.Keyword)
	if keyword == "" {
		keyword = strings.TrimSpace(filter.Search)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR subtitle LIKE ? OR description LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	switch filter.Sort {
	case "name":
		query = query.Order("name ASC")
	case "newest":
		query = query.Order("created_at DESC")
	case "sort_order":
		query = query.Order("sort_order ASC, id DESC")
	default:
		query = query.Order("sort_order ASC, id DESC")
	}
	var rows []models.Activity
	if err := query.Limit(limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	views := make([]domain.ActivityView, 0, len(rows))
	for i := range rows {
		view := domain.ActivityView{Activity: rows[i]}
		if err := s.fillStats(ctx, &view); err != nil {
			return nil, 0, err
		}
		views = append(views, view)
	}
	return views, total, nil
}

// fillStats 给列表行补上参与数据。列表最多 200 行，逐条聚合比引入缓存简单，
// 而且统计口径与详情页完全一致。
func (s *GormStore) fillStats(ctx context.Context, view *domain.ActivityView) error {
	type aggregate struct {
		OrderCount    int64
		UserCount     int64
		DiscountTotal int64
		RevenueTotal  int64
	}
	var agg aggregate
	err := s.db.WithContext(ctx).Model(&models.ActivityRecord{}).
		Select("COUNT(*) AS order_count, COUNT(DISTINCT user_id) AS user_count, COALESCE(SUM(discount_amount),0) AS discount_total, COALESCE(SUM(payable_amount),0) AS revenue_total").
		Where("activity_id = ? AND status IN ?", view.ID, []string{"reserved", "used"}).
		Scan(&agg).Error
	if err != nil {
		return err
	}
	view.OrderCount = agg.OrderCount
	view.UserCount = agg.UserCount
	view.DiscountTotal = agg.DiscountTotal
	view.RevenueTotal = agg.RevenueTotal
	if view.QuotaLimit > 0 && agg.UserCount > 0 {
		view.ConversionRate = float64(agg.UserCount) / float64(view.QuotaLimit)
	}
	return nil
}

func (s *GormStore) GetByID(ctx context.Context, id uint) (*domain.Activity, error) {
	var activity domain.Activity
	if err := s.base().WithContext(ctx).First(&activity, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrActivityNotFound
		}
		return nil, err
	}
	return &activity, nil
}

func (s *GormStore) GetByIDWithRules(ctx context.Context, id uint) (*domain.Activity, error) {
	activity, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rules, err := s.ListRules(ctx, id)
	if err != nil {
		return nil, err
	}
	activity.RulesList = rules
	return activity, nil
}

func (s *GormStore) Create(ctx context.Context, activity *domain.Activity) error {
	return s.db.WithContext(ctx).Create(activity).Error
}

func (s *GormStore) Update(ctx context.Context, activity *domain.Activity) error {
	return s.db.WithContext(ctx).Save(activity).Error
}

func (s *GormStore) Delete(ctx context.Context, id uint) error {
	result := s.base().WithContext(ctx).Where("id = ?", id).Update("status", models.ActivityStatusDeleted)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrActivityNotFound
	}
	return nil
}

func (s *GormStore) ListRules(ctx context.Context, activityID uint) ([]domain.ActivityRule, error) {
	var rules []domain.ActivityRule
	if err := s.db.WithContext(ctx).Where("activity_id = ?", activityID).
		Order("sort_order ASC, id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// ReplaceRules 先删后建，保证规则行的唯一约束（活动+规则类型）不会被旧行占住。
func (s *GormStore) ReplaceRules(ctx context.Context, activityID uint, rules []domain.ActivityRule) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("activity_id = ?", activityID).Delete(&models.ActivityRule{}).Error; err != nil {
			return err
		}
		for i := range rules {
			rules[i].ID = 0
			rules[i].ActivityID = activityID
			if err := tx.Create(&rules[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *GormStore) ListActive(ctx context.Context, now time.Time) ([]domain.Activity, error) {
	var activities []domain.Activity
	if err := s.base().WithContext(ctx).
		Where("status = ?", models.ActivityStatusRunning).
		Order("sort_order ASC, id ASC").
		Find(&activities).Error; err != nil {
		return nil, err
	}
	return activities, nil
}

// Reserve 在一个事务里做三件事：占用活动库存/名额、写参与记录、更新计数器。
// 使用条件更新（WHERE 带上限判断）而不是先读后写，避免并发超卖。
func (s *GormStore) Reserve(ctx context.Context, activityID uint, record *domain.ActivityRecord) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var activity models.Activity
		if err := tx.Where("id = ?", activityID).First(&activity).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrActivityNotFound
			}
			return err
		}
		if activity.Status != models.ActivityStatusRunning {
			return domain.ErrNotRunning
		}
		if activity.StockLimit > 0 && activity.StockUsed+record.Quantity > activity.StockLimit {
			return domain.ErrStockExhausted
		}
		if activity.QuotaLimit > 0 && activity.QuotaUsed+1 > activity.QuotaLimit {
			return domain.ErrQuotaExhausted
		}

		updates := map[string]any{"stock_used": gorm.Expr("stock_used + ?", record.Quantity)}
		where := "id = ? AND status = ?"
		args := []any{activityID, models.ActivityStatusRunning}
		if activity.StockLimit > 0 {
			where += " AND stock_used + ? <= stock_limit"
			args = append(args, record.Quantity)
		}
		updates["quota_used"] = gorm.Expr("quota_used + 1")
		if activity.QuotaLimit > 0 {
			where += " AND quota_used + 1 <= quota_limit"
		}
		result := tx.Model(&models.Activity{}).Where(where, args...).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if activity.StockLimit > 0 && activity.StockUsed+record.Quantity > activity.StockLimit {
				return domain.ErrStockExhausted
			}
			return domain.ErrQuotaExhausted
		}

		record.ActivityID = activityID
		if record.Status == "" {
			record.Status = "reserved"
		}
		return tx.Create(record).Error
	})
}

func (s *GormStore) Release(ctx context.Context, recordID uint, status string) error {
	return s.releaseRecords(ctx, "id = ?", []any{recordID}, status)
}

func (s *GormStore) MarkRecordStatus(ctx context.Context, orderID uint, status string) error {
	return s.releaseRecords(ctx, "order_id = ?", []any{orderID}, status)
}

// releaseRecords 在事务里把参与记录标记为最终状态，并回退活动计数器。
// 只有仍在 reserved/used 的记录会回退，重复调用不会重复扣减。
func (s *GormStore) releaseRecords(ctx context.Context, where string, args []any, status string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []models.ActivityRecord
		q := tx.Where(where, args...)
		if status == "used" {
			q = q.Where("status = ?", "reserved")
		} else {
			q = q.Where("status IN ?", []string{"reserved", "used"})
		}
		if err := q.Find(&records).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		released := make(map[uint]int)
		quota := make(map[uint]int)
		for _, record := range records {
			released[record.ActivityID] += record.Quantity
			if record.Status != "used" || status == "refunded" || status == "cancelled" {
				quota[record.ActivityID]++
			}
			if err := tx.Model(&models.ActivityRecord{}).Where("id = ?", record.ID).
				Update("status", status).Error; err != nil {
				return err
			}
		}
		for activityID, quantity := range released {
			updates := map[string]any{"stock_used": gorm.Expr("MAX(stock_used - ?, 0)", quantity)}
			if count := quota[activityID]; count > 0 {
				updates["quota_used"] = gorm.Expr("MAX(quota_used - ?, 0)", count)
			}
			if err := tx.Model(&models.Activity{}).Where("id = ?", activityID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *GormStore) CountUserRecords(ctx context.Context, activityID, userID uint) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.ActivityRecord{}).
		Where("activity_id = ? AND user_id = ? AND status IN ?", activityID, userID, []string{"reserved", "used"}).
		Count(&count).Error
	return count, err
}

func (s *GormStore) ListRecords(ctx context.Context, activityID uint, limit, offset int) ([]domain.ActivityRecord, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.ActivityRecord{}).Where("activity_id = ?", activityID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []domain.ActivityRecord
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (s *GormStore) ListRecordsByUser(ctx context.Context, userID uint, limit, offset int) ([]domain.ActivityRecord, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.ActivityRecord{}).Where("user_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []domain.ActivityRecord
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (s *GormStore) Stats(ctx context.Context, activityID uint) (*domain.ActivityStats, error) {
	activity, err := s.GetByID(ctx, activityID)
	if err != nil {
		return nil, err
	}
	stats := &domain.ActivityStats{
		ActivityID: activityID,
		StockUsed:  activity.StockUsed,
		StockLimit: activity.StockLimit,
		QuotaUsed:  activity.QuotaUsed,
		QuotaLimit: activity.QuotaLimit,
	}
	type aggregate struct {
		Orders        int64
		Participants  int64
		PaidOrders    int64
		DiscountTotal int64
		RevenueTotal  int64
		RefundCount   int64
	}
	var agg aggregate
	if err := s.db.WithContext(ctx).Model(&models.ActivityRecord{}).
		Select(strings.Join([]string{
			"COUNT(*) AS orders",
			"COUNT(DISTINCT user_id) AS participants",
			"COALESCE(SUM(CASE WHEN status = 'used' THEN 1 ELSE 0 END),0) AS paid_orders",
			"COALESCE(SUM(discount_amount),0) AS discount_total",
			"COALESCE(SUM(CASE WHEN status = 'used' THEN payable_amount ELSE 0 END),0) AS revenue_total",
		}, ", ")).
		Where("activity_id = ?", activityID).Scan(&agg).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&models.ActivityRecord{}).
		Where("activity_id = ? AND status = ?", activityID, "refunded").
		Count(&agg.RefundCount).Error; err != nil {
		return nil, err
	}
	stats.Orders = agg.Orders
	stats.Participants = agg.Participants
	stats.PaidOrders = agg.PaidOrders
	stats.DiscountTotal = agg.DiscountTotal
	stats.RevenueTotal = agg.RevenueTotal
	stats.RefundCount = agg.RefundCount
	if activity.QuotaLimit > 0 {
		stats.ConversionRate = float64(agg.Participants) / float64(activity.QuotaLimit)
	}
	return stats, nil
}

func (s *GormStore) ListLogs(ctx context.Context, activityID uint, limit, offset int) ([]domain.ActivityLog, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.ActivityLog{})
	if activityID != 0 {
		query = query.Where("activity_id = ?", activityID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []domain.ActivityLog
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (s *GormStore) AppendLog(ctx context.Context, entry *domain.ActivityLog) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

func (s *GormStore) HasPurchased(ctx context.Context, userID uint) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.Order{}).
		Where("user_id = ? AND status IN ?", userID, []string{"paid", "delivered", "completed"}).
		Count(&count).Error
	return count > 0, err
}

func (s *GormStore) ProductCategory(ctx context.Context, productID uint) (*uint, error) {
	var product models.Product
	if err := s.db.WithContext(ctx).Select("id", "category_id").First(&product, productID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return product.CategoryID, nil
}

func (s *GormStore) CouponClaim(ctx context.Context, record *domain.CouponRecord) error {
	return s.db.WithContext(ctx).Create(record).Error
}

func (s *GormStore) ListCouponRecords(ctx context.Context, userID uint, limit, offset int) ([]domain.CouponRecord, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.CouponRecord{})
	if userID != 0 {
		query = query.Where("user_id = ?", userID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []domain.CouponRecord
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (s *GormStore) CountCouponRecord(ctx context.Context, couponID, userID uint) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.CouponRecord{}).
		Where("coupon_id = ? AND user_id = ?", couponID, userID).Count(&count).Error
	return count, err
}
