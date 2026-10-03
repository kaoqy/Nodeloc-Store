package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/activity/domain"
)

// Service 是活动模块的用例层：后台管理、结算计价、参与记录与统计都从这里走。
// 它不直接碰 HTTP 或 GORM，便于单测替换。
type Service struct {
	repo contract.Repository
	now  func() time.Time
}

func NewService(repo contract.Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("activity service requires a repository")
	}
	return &Service{repo: repo, now: time.Now}, nil
}

// ── 后台管理 ─────────────────────────────────────────────────────────

func (s *Service) List(ctx context.Context, filter domain.ListFilter) ([]domain.ActivityView, int64, error) {
	return s.repo.List(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id uint) (*domain.Activity, error) {
	return s.repo.GetByIDWithRules(ctx, id)
}

// SaveOptions 是创建/更新活动的完整输入。
type SaveOptions struct {
	Activity  domain.Activity
	Rules     []domain.ActivityRule
	ActorID   uint
	IP        string
	UserAgent string
}

// Create 校验并写入一次活动，同时保存结构化规则。
func (s *Service) Create(ctx context.Context, options SaveOptions) (*domain.Activity, error) {
	activity := options.Activity
	if err := s.validate(&activity, options.Rules, true); err != nil {
		return nil, err
	}
	if activity.CreatedBy == nil && options.ActorID > 0 {
		activity.CreatedBy = &options.ActorID
	}
	if err := s.repo.Create(ctx, &activity); err != nil {
		return nil, err
	}
	if len(options.Rules) > 0 {
		rules := normalizeRules(options.Rules)
		if err := s.repo.ReplaceRules(ctx, activity.ID, rules); err != nil {
			return nil, err
		}
	}
	if err := s.afterChange(ctx, &activity, "activity.create", options, ""); err != nil {
		return nil, err
	}
	return s.repo.GetByIDWithRules(ctx, activity.ID)
}

// Update 更新一次活动。保存前后快照进日志，便于回溯改了哪一条规则。
func (s *Service) Update(ctx context.Context, id uint, options SaveOptions) (*domain.Activity, error) {
	existing, err := s.repo.GetByIDWithRules(ctx, id)
	if err != nil {
		return nil, err
	}
	activity := options.Activity
	activity.Base = existing.Base
	activity.StockUsed = existing.StockUsed
	activity.QuotaUsed = existing.QuotaUsed
	if activity.CreatedBy == nil {
		activity.CreatedBy = existing.CreatedBy
	}
	if err := s.validate(&activity, options.Rules, false); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, &activity); err != nil {
		return nil, err
	}
	if options.Rules != nil {
		rules := normalizeRules(options.Rules)
		if err := s.repo.ReplaceRules(ctx, activity.ID, rules); err != nil {
			return nil, err
		}
	}
	before, _ := json.Marshal(existing)
	if err := s.afterChange(ctx, &activity, "activity.update", options, string(before)); err != nil {
		return nil, err
	}
	return s.repo.GetByIDWithRules(ctx, activity.ID)
}

// Duplicate 复制一个活动为草稿，规则一并复制，名称带（副本）。
func (s *Service) Duplicate(ctx context.Context, id uint, actorID uint) (*domain.Activity, error) {
	source, err := s.repo.GetByIDWithRules(ctx, id)
	if err != nil {
		return nil, err
	}
	copyActivity := *source
	copyActivity.Base = models.Base{}
	copyActivity.Name = source.Name + "（副本）"
	copyActivity.Status = models.ActivityStatusDraft
	copyActivity.StockUsed = 0
	copyActivity.QuotaUsed = 0
	copyActivity.CreatedBy = &actorID
	copyActivity.RulesList = nil
	if err := s.repo.Create(ctx, &copyActivity); err != nil {
		return nil, err
	}
	rules := make([]domain.ActivityRule, 0, len(source.RulesList))
	for _, rule := range source.RulesList {
		rule.Base = models.Base{}
		rule.ActivityID = copyActivity.ID
		rules = append(rules, rule)
	}
	if len(rules) > 0 {
		if err := s.repo.ReplaceRules(ctx, copyActivity.ID, rules); err != nil {
			return nil, err
		}
	}
	return s.repo.GetByIDWithRules(ctx, copyActivity.ID)
}

// SetStatus 是上下架/暂停/恢复的统一入口，全部记日志。
func (s *Service) SetStatus(ctx context.Context, id uint, status string, actorID uint, ip, reason string) (*domain.Activity, error) {
	status = strings.TrimSpace(status)
	if !models.ValidActivityStatus(status) {
		return nil, fmt.Errorf("%w: 未知的活动状态 %q", domain.ErrInvalidInput, status)
	}
	activity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	before, _ := json.Marshal(activity)
	activity.Status = status
	activity.TerminateReason = reason
	if err := s.repo.Update(ctx, activity); err != nil {
		return nil, err
	}
	after, _ := json.Marshal(activity)
	action := "activity.status"
	log := &domain.ActivityLog{
		ActivityID: id, ActorID: &actorID, Action: action,
		Detail: reason, Before: string(before), After: string(after),
		IP: ip, Result: "ok",
	}
	_ = s.repo.AppendLog(ctx, log)
	return activity, nil
}

// Delete 是软删除：状态改为已删除，历史订单的参与记录与快照保持不动。
func (s *Service) Delete(ctx context.Context, id uint, actorID uint, ip string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	log := &domain.ActivityLog{ActivityID: id, Action: "activity.delete", Result: "ok", IP: ip}
	if actorID > 0 {
		log.ActorID = &actorID
	}
	return s.repo.AppendLog(ctx, log)
}

// RefreshStatuses 按时间自动推进活动状态：到点开始、过期结束。
// 已暂停/已下架的活动不会被自动改状态，尊重店家的手动决定。
func (s *Service) RefreshStatuses(ctx context.Context) (int, error) {
	activities, err := s.repo.ListActive(ctx, s.now())
	if err != nil {
		return 0, err
	}
	changed := 0
	for i := range activities {
		activity := activities[i]
		if next, ok := s.timeStatus(&activity); ok {
			activity.Status = next
			if err := s.repo.Update(ctx, &activity); err != nil {
				return changed, err
			}
			changed++
		}
	}
	return changed, nil
}

// timeStatus 判断一个活动是否该由时间推进状态。返回 false 表示不需要变动。
func (s *Service) timeStatus(activity *domain.Activity) (string, bool) {
	if activity.Status != models.ActivityStatusRunning && activity.Status != models.ActivityStatusScheduled {
		return "", false
	}
	now := s.now()
	if activity.StartAt != nil && now.Before(*activity.StartAt) {
		if activity.Status != models.ActivityStatusScheduled {
			return models.ActivityStatusScheduled, true
		}
		return "", false
	}
	if activity.EndAt != nil && !now.Before(*activity.EndAt) {
		if activity.Status != models.ActivityStatusEnded {
			return models.ActivityStatusEnded, true
		}
		return "", false
	}
	if activity.Status != models.ActivityStatusRunning {
		return models.ActivityStatusRunning, true
	}
	return "", false
}

func (s *Service) Stats(ctx context.Context, id uint) (*domain.ActivityStats, error) {
	return s.repo.Stats(ctx, id)
}

func (s *Service) Records(ctx context.Context, id uint, limit, offset int) ([]domain.ActivityRecord, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.repo.ListRecords(ctx, id, limit, offset)
}

func (s *Service) Logs(ctx context.Context, id uint, limit, offset int) ([]domain.ActivityLog, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.repo.ListLogs(ctx, id, limit, offset)
}

// ── 结算计价 ─────────────────────────────────────────────────────────

// Price 计算最优活动优惠。多条活动同时命中时取优惠最大的一条，避免叠加
// 造成的价格倒挂；活动之间不叠加是默认策略，需要叠加的活动在规则里显式声明。
func (s *Service) Price(ctx context.Context, input domain.MatchInput) (*domain.Pricing, error) {
	if input.Quantity <= 0 {
		input.Quantity = 1
	}
	if input.Now.IsZero() {
		input.Now = s.now()
	}
	activities, err := s.repo.ListActive(ctx, input.Now)
	if err != nil {
		return nil, err
	}
	if input.CategoryID == nil {
		if category, err := s.repo.ProductCategory(ctx, input.ProductID); err == nil {
			input.CategoryID = category
		}
	}

	var best *domain.Pricing
	for i := range activities {
		activity := activities[i]
		if err := s.applies(ctx, &activity, input); err != nil {
			continue
		}
		rules, err := s.repo.ListRules(ctx, activity.ID)
		if err != nil {
			continue
		}
		pricing, err := s.priceWithRules(&activity, rules, input)
		if err != nil || pricing == nil || pricing.DiscountAmount <= 0 {
			continue
		}
		if best == nil || pricing.DiscountAmount > best.DiscountAmount {
			best = pricing
		}
	}
	if best == nil {
		return nil, domain.ErrNoActivity
	}
	return best, nil
}

// applies 判断活动的时间、范围、人群、库存与每人限次是否允许参与。
func (s *Service) applies(ctx context.Context, activity *domain.Activity, input domain.MatchInput) error {
	if activity.Status != models.ActivityStatusRunning {
		return domain.ErrNotRunning
	}
	if activity.RequireLogin && input.UserID == 0 {
		return domain.ErrNotApplicable
	}
	if activity.StartAt != nil && input.Now.Before(*activity.StartAt) {
		return domain.ErrNotRunning
	}
	if activity.EndAt != nil && !input.Now.Before(*activity.EndAt) {
		return domain.ErrNotRunning
	}
	if activity.StockLimit > 0 && activity.StockUsed >= activity.StockLimit {
		return domain.ErrStockExhausted
	}
	if activity.QuotaLimit > 0 && activity.QuotaUsed >= activity.QuotaLimit {
		return domain.ErrQuotaExhausted
	}
	if !s.scopeMatches(activity, input) {
		return domain.ErrNotApplicable
	}
	if input.UserID > 0 && activity.PerUserLimit > 0 {
		used, err := s.repo.CountUserRecords(ctx, activity.ID, input.UserID)
		if err != nil {
			return err
		}
		if used >= int64(activity.PerUserLimit) {
			return domain.ErrPerUserLimit
		}
	}
	switch activity.UserScope {
	case "new_user":
		if !input.IsNewUser {
			return domain.ErrNotApplicable
		}
	case "first_purchase":
		if !input.IsFirstOrder {
			return domain.ErrNotApplicable
		}
	case "member":
		if input.UserID == 0 || input.Role == "" || input.Role == "user" {
			return domain.ErrNotApplicable
		}
	case "role":
		if activity.UserRoleScope != "" && input.Role != activity.UserRoleScope {
			return domain.ErrNotApplicable
		}
	}
	return nil
}

func (s *Service) scopeMatches(activity *domain.Activity, input domain.MatchInput) bool {
	products := models.ParseIDList(activity.ProductIDs)
	categories := models.ParseIDList(activity.CategoryIDs)
	if len(products) == 0 && len(categories) == 0 {
		return true
	}
	for _, id := range products {
		if id == input.ProductID {
			return true
		}
	}
	if input.CategoryID != nil {
		for _, id := range categories {
			if id == *input.CategoryID {
				return true
			}
		}
	}
	return false
}

// priceWithRules 把结构化规则换算成一次折扣。金额单位与商品定价一致。
func (s *Service) priceWithRules(activity *domain.Activity, rules []domain.ActivityRule, input domain.MatchInput) (*domain.Pricing, error) {
	total := input.UnitPrice * input.Quantity
	if total <= 0 {
		return nil, nil
	}
	discount := 0
	applied := make([]string, 0, len(rules))
	stacking := activity.AllowStacking
	for _, rule := range rules {
		if !rule.IsEnabled {
			continue
		}
		config := parseRuleConfig(rule.Config)
		value := 0
		switch rule.RuleType {
		case domain.RuleFactorOff:
			// 乘区：成交价 = 售价 × 系数。折扣额取整到分，
			// 与下单、支付、后台统计三处用同一个舍入方式。
			if config.Factor > 0 && config.Factor < 1 {
				value = discountFromFactor(total, config.Factor)
			}
		case domain.RulePercentOff:
			if config.Percent > 0 && config.Percent < 100 {
				value = discountFromFactor(total, float64(config.Percent)/100)
			}
		case domain.RuleAmountOff:
			value = config.Amount
		case domain.RuleFixedPrice:
			if config.Price > 0 && config.Price < input.UnitPrice {
				value = (input.UnitPrice - config.Price) * input.Quantity
			}
		case domain.RuleFullReduce:
			value = tierAmount(config, total, input.Quantity)
		case domain.RuleFullQuantity:
			if config.Quantity > 0 && input.Quantity >= config.Quantity {
				value = config.Amount * input.Quantity
				if config.PerUnit > 0 {
					value = config.PerUnit * input.Quantity
				}
			}
		case domain.RuleBulkPrice:
			if config.Quantity > 0 && input.Quantity >= config.Quantity && config.Price > 0 && config.Price < input.UnitPrice {
				value = (input.UnitPrice - config.Price) * input.Quantity
			}
		}
		if value <= 0 {
			continue
		}
		if !stacking && value > discount {
			// 不可叠加时取最大的一条，而不是累加。
			discount = value
			applied = []string{rule.RuleType}
			continue
		}
		if stacking {
			discount += value
		}
		if !contains(applied, rule.RuleType) {
			applied = append(applied, rule.RuleType)
		}
	}
	if discount <= 0 {
		return nil, nil
	}
	// 活动也不能把订单打到 0：NodeLoc 需要收一笔真实金额。
	if limit := total - 1; discount > limit {
		discount = limit
	}
	snapshot, err := json.Marshal(map[string]any{
		"activity_id":   activity.ID,
		"activity_name": activity.Name,
		"type":          activity.Type,
		"rules":         rules,
		"unit_price":    input.UnitPrice,
		"quantity":      input.Quantity,
		"total":         total,
		"discount":      discount,
	})
	if err != nil {
		return nil, err
	}
	return &domain.Pricing{
		ActivityID:     activity.ID,
		ActivityName:   activity.Name,
		DiscountAmount: discount,
		Payable:        total - discount,
		Snapshot:       string(snapshot),
		AppliedRules:   applied,
	}, nil
}

// Reserve 在订单生成后占用活动名额，并把参与记录写进快照。
func (s *Service) Reserve(ctx context.Context, orderID uint, record *domain.ActivityRecord) error {
	if record == nil || record.ActivityID == 0 {
		return nil
	}
	if record.Quantity <= 0 {
		record.Quantity = 1
	}
	if record.OrderID == nil && orderID > 0 {
		record.OrderID = &orderID
	}
	return s.repo.Reserve(ctx, record.ActivityID, record)
}

// MarkOrderSettled 在支付成功时把参与记录改为 used。
func (s *Service) MarkOrderSettled(ctx context.Context, orderID uint) error {
	return s.repo.MarkRecordStatus(ctx, orderID, "used")
}

// MarkOrderRefunded 在退款时把参与记录改为 refunded 并回退库存/名额。
func (s *Service) MarkOrderRefunded(ctx context.Context, orderID uint) error {
	return s.repo.MarkRecordStatus(ctx, orderID, "refunded")
}

// MarkOrderCancelled 在订单取消时释放活动占位。
func (s *Service) MarkOrderCancelled(ctx context.Context, orderID uint) error {
	return s.repo.MarkRecordStatus(ctx, orderID, "cancelled")
}

// RecordsForUser 是买家侧的「我的活动」列表。
func (s *Service) RecordsForUser(ctx context.Context, userID uint, limit, offset int) ([]domain.ActivityRecord, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListRecordsByUser(ctx, userID, limit, offset)
}

// PublicActivities 是买家侧的活动中心：只取进行中的活动，按排序与开始时间。
func (s *Service) PublicActivities(ctx context.Context, activityType string, limit int) ([]domain.Activity, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	activities, err := s.repo.ListActive(ctx, s.now())
	if err != nil {
		return nil, 0, err
	}
	out := make([]domain.Activity, 0, len(activities))
	for _, activity := range activities {
		if !activity.ShowInList {
			continue
		}
		if activityType != "" && activity.Type != activityType {
			continue
		}
		if activity.EndAt != nil && !s.now().Before(*activity.EndAt) {
			continue
		}
		out = append(out, activity)
		if len(out) >= limit {
			break
		}
	}
	return out, int64(len(out)), nil
}

// PublicActivity 是活动详情。下架或已删除的活动对买家不可见。
func (s *Service) PublicActivity(ctx context.Context, id uint) (*domain.Activity, error) {
	activity, err := s.repo.GetByIDWithRules(ctx, id)
	if err != nil {
		return nil, err
	}
	if activity.Status != models.ActivityStatusRunning && activity.Status != models.ActivityStatusScheduled {
		return nil, domain.ErrActivityNotFound
	}
	return activity, nil
}

// CouponRecords 是后台的优惠券领取记录列表。
func (s *Service) CouponRecords(ctx context.Context, userID uint, limit, offset int) ([]domain.CouponRecord, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.repo.ListCouponRecords(ctx, userID, limit, offset)
}

// ClaimCoupon 记录一次优惠券领取。同一用户同一张券只允许领取一次。
func (s *Service) ClaimCoupon(ctx context.Context, couponID, userID uint, source, ip string) (*domain.CouponRecord, error) {
	if couponID == 0 || userID == 0 {
		return nil, fmt.Errorf("%w: 需要登录后才能领取优惠券", domain.ErrInvalidInput)
	}
	count, err := s.repo.CountCouponRecord(ctx, couponID, userID)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("%w: 这张优惠券你已经领取过了", domain.ErrInvalidInput)
	}
	now := s.now().UTC()
	_ = ip
	record := &domain.CouponRecord{
		CouponID: couponID, UserID: userID, Status: "claimed",
		Source: source, ClaimedAt: now,
	}
	if err := s.repo.CouponClaim(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

// ── 校验 ─────────────────────────────────────────────────────────────

func (s *Service) validate(activity *domain.Activity, rules []domain.ActivityRule, creating bool) error {
	activity.Name = strings.TrimSpace(activity.Name)
	activity.Type = strings.TrimSpace(activity.Type)
	activity.Status = strings.TrimSpace(activity.Status)
	if activity.Name == "" {
		return fmt.Errorf("%w: 活动名称要填。", domain.ErrInvalidInput)
	}
	if len([]rune(activity.Name)) > 80 {
		return fmt.Errorf("%w: 活动名称最多 80 个字。", domain.ErrInvalidInput)
	}
	if !models.ValidActivityType(activity.Type) {
		return fmt.Errorf("%w: 请选择活动类型。", domain.ErrInvalidInput)
	}
	if activity.Status == "" {
		if creating {
			activity.Status = models.ActivityStatusDraft
		} else {
			return fmt.Errorf("%w: 活动状态无效。", domain.ErrInvalidInput)
		}
	}
	if !models.ValidActivityStatus(activity.Status) {
		return fmt.Errorf("%w: 活动状态无效。", domain.ErrInvalidInput)
	}
	if activity.StartAt != nil && activity.EndAt != nil && !activity.EndAt.After(*activity.StartAt) {
		return fmt.Errorf("%w: 结束时间必须晚于开始时间。", domain.ErrInvalidInput)
	}
	if activity.StockLimit < 0 || activity.QuotaLimit < 0 || activity.PerUserLimit < 0 {
		return fmt.Errorf("%w: 库存、名额与每人限次不能是负数。", domain.ErrInvalidInput)
	}
	if activity.PerUserLimit > 0 && activity.QuotaLimit > 0 && activity.PerUserLimit > activity.QuotaLimit {
		return fmt.Errorf("%w: 每人限次不能大于活动名额。", domain.ErrInvalidInput)
	}
	switch activity.UserScope {
	case "", "all":
		activity.UserScope = "all"
	case "new_user", "first_purchase", "member", "role":
	default:
		return fmt.Errorf("%w: 适用人群取值无效。", domain.ErrInvalidInput)
	}
	if activity.UserScope == "role" && strings.TrimSpace(activity.UserRoleScope) == "" {
		return fmt.Errorf("%w: 选择按角色限定时要指定角色。", domain.ErrInvalidInput)
	}
	if _, err := models.EncodeIDList(models.ParseIDList(activity.ProductIDs)); err != nil {
		return fmt.Errorf("%w: 适用商品格式无效。", domain.ErrInvalidInput)
	}
	if _, err := models.EncodeIDList(models.ParseIDList(activity.CategoryIDs)); err != nil {
		return fmt.Errorf("%w: 适用分类格式无效。", domain.ErrInvalidInput)
	}
	for _, rule := range rules {
		ruleType := strings.TrimSpace(rule.RuleType)
		if ruleType == "" {
			return fmt.Errorf("%w: 规则类型不能为空。", domain.ErrInvalidInput)
		}
		if !domain.ValidRuleType(ruleType) {
			return fmt.Errorf("%w: 未知的规则类型 %q。", domain.ErrInvalidInput, ruleType)
		}
		config := parseRuleConfig(rule.Config)
		if err := validateRuleConfig(ruleType, config); err != nil {
			return err
		}
	}
	if len(rules) == 0 && activity.AutoApply {
		return fmt.Errorf("%w: 自动应用的活动至少要配一条规则。", domain.ErrInvalidInput)
	}
	return nil
}

func validateRuleConfig(ruleType string, config domain.RuleConfig) error {
	switch ruleType {
	case domain.RulePercentOff:
		if config.Percent <= 0 || config.Percent >= 100 {
			return fmt.Errorf("%w: 折扣百分比要在 1 到 99 之间。", domain.ErrInvalidInput)
		}
	case domain.RuleAmountOff:
		if config.Amount <= 0 {
			return fmt.Errorf("%w: 立减金额要大于 0。", domain.ErrInvalidInput)
		}
	case domain.RuleFixedPrice, domain.RuleBulkPrice:
		if config.Price <= 0 {
			return fmt.Errorf("%w: 折后单价要大于 0。", domain.ErrInvalidInput)
		}
	case domain.RuleFullReduce:
		if len(config.Tiers) == 0 && (config.Threshold <= 0 || config.Amount <= 0) {
			return fmt.Errorf("%w: 满减规则要填门槛和减免金额。", domain.ErrInvalidInput)
		}
	case domain.RuleFullQuantity:
		if config.Quantity <= 0 {
			return fmt.Errorf("%w: 满件规则要填件数。", domain.ErrInvalidInput)
		}
	case domain.RuleCouponLock, domain.RuleGiftCoupon:
		if config.CouponID == 0 {
			return fmt.Errorf("%w: 优惠券类规则要选择一张优惠券。", domain.ErrInvalidInput)
		}
	}
	return nil
}

// ── 辅助 ─────────────────────────────────────────────────────────────

func (s *Service) afterChange(ctx context.Context, activity *domain.Activity, action string, options SaveOptions, before string) error {
	after, err := json.Marshal(activity)
	if err != nil {
		return err
	}
	entry := &domain.ActivityLog{
		ActivityID: activity.ID,
		Action:     action,
		Before:     before,
		After:      string(after),
		IP:         options.IP,
		UserAgent:  options.UserAgent,
		Result:     "ok",
	}
	if options.ActorID > 0 {
		entry.ActorID = &options.ActorID
	}
	return s.repo.AppendLog(ctx, entry)
}

func normalizeRules(rules []domain.ActivityRule) []domain.ActivityRule {
	out := make([]domain.ActivityRule, 0, len(rules))
	for i := range rules {
		rule := rules[i]
		rule.RuleType = strings.TrimSpace(rule.RuleType)
		if rule.Config == "" {
			rule.Config = "{}"
		}
		if rule.SortOrder == 0 {
			rule.SortOrder = i
		}
		out = append(out, rule)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out
}

func parseRuleConfig(raw string) domain.RuleConfig {
	var config domain.RuleConfig
	if strings.TrimSpace(raw) == "" {
		return config
	}
	_ = json.Unmarshal([]byte(raw), &config)
	return config
}

// tierAmount 取命中的最高一档。没有阶梯时退回单档门槛。
func tierAmount(config domain.RuleConfig, total, quantity int) int {
	amount := 0
	if len(config.Tiers) == 0 {
		if config.Threshold > 0 && total >= config.Threshold {
			return config.Amount
		}
		return 0
	}
	for _, tier := range config.Tiers {
		reached := true
		if tier.Threshold > 0 && total < tier.Threshold {
			reached = false
		}
		if tier.Quantity > 0 && quantity < tier.Quantity {
			reached = false
		}
		if !reached {
			continue
		}
		value := tier.Amount
		if tier.Percent > 0 && tier.Percent < 100 {
			value = total * (100 - tier.Percent) / 100
		}
		if value > amount {
			amount = value
		}
	}
	return amount
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// discountFromFactor 把「售价 × 系数」换算成应减金额。
//
// 金额在整条链路上都是整数（支付网关以最小货币单位结算），所以这里只做一次
// 舍入，并把结果四舍五入到分。前台展示、下单、支付、统计都复用同一个
// Pricing.DiscountAmount，不再各自算一遍。
func discountFromFactor(total int, factor float64) int {
	if total <= 0 || factor <= 0 || factor >= 1 {
		return 0
	}
	payable := float64(total) * factor
	discount := float64(total) - math.Round(payable)
	if discount < 0 {
		return 0
	}
	return int(math.Round(discount))
}
