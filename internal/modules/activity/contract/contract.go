package contract

import (
	"context"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/domain"
)

// Repository 是活动模块的持久化端口。
type Repository interface {
	List(ctx context.Context, filter domain.ListFilter) ([]domain.ActivityView, int64, error)
	GetByID(ctx context.Context, id uint) (*domain.Activity, error)
	GetByIDWithRules(ctx context.Context, id uint) (*domain.Activity, error)
	Create(ctx context.Context, activity *domain.Activity) error
	Update(ctx context.Context, activity *domain.Activity) error
	Delete(ctx context.Context, id uint) error

	ListRules(ctx context.Context, activityID uint) ([]domain.ActivityRule, error)
	ReplaceRules(ctx context.Context, activityID uint, rules []domain.ActivityRule) error

	// ListActive 是结算用读取：只取此刻应该生效的活动，按 SortOrder 排序。
	ListActive(ctx context.Context, now time.Time) ([]domain.Activity, error)
	// Reserve 在一条事务里占用库存/名额并落参与记录，返回新的记录 ID。
	Reserve(ctx context.Context, activityID uint, record *domain.ActivityRecord) error
	Release(ctx context.Context, recordID uint, status string) error
	MarkRecordStatus(ctx context.Context, orderID uint, status string) error
	CountUserRecords(ctx context.Context, activityID, userID uint) (int64, error)
	ListRecords(ctx context.Context, activityID uint, limit, offset int) ([]domain.ActivityRecord, int64, error)
	ListRecordsByUser(ctx context.Context, userID uint, limit, offset int) ([]domain.ActivityRecord, int64, error)

	Stats(ctx context.Context, activityID uint) (*domain.ActivityStats, error)
	ListLogs(ctx context.Context, activityID uint, limit, offset int) ([]domain.ActivityLog, int64, error)
	AppendLog(ctx context.Context, entry *domain.ActivityLog) error

	// HasPurchased 判断首次购买类活动是否还适用。
	HasPurchased(ctx context.Context, userID uint) (bool, error)
	ProductCategory(ctx context.Context, productID uint) (*uint, error)

	// CouponClaim 记录一次优惠券领取；同一用户同一券只能领一次。
	CouponClaim(ctx context.Context, record *domain.CouponRecord) error
	ListCouponRecords(ctx context.Context, userID uint, limit, offset int) ([]domain.CouponRecord, int64, error)
	CountCouponRecord(ctx context.Context, couponID, userID uint) (int64, error)
}

// Pricing 是活动模块对支付模块暴露的唯一契约：给定订单上下文，返回活动
// 优惠。支付模块只调用这个接口，不感知活动的任何存储细节。
type Pricing interface {
	// Price 计算一次下单可用的最优活动优惠。没有命中返回 domain.ErrNoActivity。
	Price(ctx context.Context, input domain.MatchInput) (*domain.Pricing, error)
	// Reserve 在订单创建成功后占用活动库存与名额；失败时订单仍然成立，
	// 由后台对账处理，因此返回的错误只用于日志。
	Reserve(ctx context.Context, orderID uint, record *domain.ActivityRecord) error
}
