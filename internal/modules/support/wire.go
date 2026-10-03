package support

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/mail"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/infrastructure"
	supporthttp "github.com/kaoqy/Nodeloc-Store/internal/modules/support/transport/http"
)

// Module 是工单与 AI 客服模块的运行时依赖。
type Module struct {
	Service *application.Service
	Handler *supporthttp.Handler
	Store   *infrastructure.GormStore
}

// Dependencies 是容器注入给本模块的其它模块能力。
type Dependencies struct {
	// Orders / Users / Catalog / Activities 都是只读适配器，AI 通过它们
	// 在授权范围内查询数据，而不是直接访问数据库。
	Orders     contract.OrderReader
	Users      contract.UserReader
	Catalog    contract.CatalogReader
	Activities contract.ActivityReader
	Notifier   contract.Notifier
	Mailer     contract.MailSender
	// Staff 用于把提醒事件投递给管理端员工（站内 + 邮箱）。
	Staff contract.StaffRecipients
	// Refunder 让 AI 能对当前用户自己的已支付订单发起退款。
	Refunder contract.Refunder
	// Cards / Fulfillment / Pricing 是新增工具用到的只读与补发能力，
	// 都由既有模块提供，AI 只是调用方。
	Cards       contract.CardReader
	Fulfillment contract.FulfillmentRetrier
	Pricing     contract.ProductPricer
}

// Wire 构造模块。SecretKey 用于加密 AI API Key。
func Wire(db *gorm.DB, cfg *config.Config, deps Dependencies) *Module {
	store := infrastructure.NewGormStore(db)
	service, err := application.NewService(application.Deps{
		Repo:        store,
		Orders:      deps.Orders,
		Users:       deps.Users,
		Catalog:     deps.Catalog,
		Activities:  deps.Activities,
		Notifier:    deps.Notifier,
		Mailer:      deps.Mailer,
		Staff:       deps.Staff,
		Refunder:    deps.Refunder,
		Cards:       deps.Cards,
		Fulfillment: deps.Fulfillment,
		Pricing:     deps.Pricing,
		SecretKey:   cfg.JWT.Secret,
	})
	if err != nil {
		panic(err)
	}
	// 真实的大模型客户端需要解密 API Key，解密函数由服务自己提供。
	service.SetModel(infrastructure.NewSecureModelClient(service.DecryptSecret))
	// 首次启动写入内置说明文章，让 AI 一开始就有依据。
	if err := service.SeedKnowledge(context.Background(), 0); err != nil {
		supportlog("seed knowledge: %v", err)
	}
	if err := service.SyncToolDefinitions(context.Background()); err != nil {
		supportlog("sync tools: %v", err)
	}
	return &Module{Service: service, Handler: supporthttp.NewHandler(service), Store: store}
}

// ── 身份适配器 ───────────────────────────────────────────────────────

// UserAdapter 让 AI 读取当前用户的公开资料字段。
type UserAdapter struct {
	Find func(ctx context.Context, userID uint) (*models.User, error)
}

func (a UserAdapter) Context(ctx context.Context, userID uint) (*contract.UserContext, error) {
	if a.Find == nil || userID == 0 {
		return nil, domain.ErrForbidden
	}
	user, err := a.Find(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, domain.ErrForbidden
	}
	return &contract.UserContext{
		ID: user.ID, Username: user.Username, Nickname: user.Nickname, Role: user.Role,
		IsActive: user.IsActive, HasEmail: user.Email != nil && *user.Email != "",
		OAuthBound: user.OAuthUID != nil && *user.OAuthUID != "",
		Points:     user.Points, CreatedAt: user.CreatedAt,
	}, nil
}

// ── 订单适配器 ───────────────────────────────────────────────────────

// OrderAdapter 把支付模块的订单数据收敛成 AI 可见的字段。
// 它只返回调用者自己的订单，越权在适配器里就被拦住。
type OrderAdapter struct {
	DB *gorm.DB
}

func (a OrderAdapter) OrdersForUser(ctx context.Context, userID uint, limit int) ([]contract.OrderContext, error) {
	if a.DB == nil || userID == 0 {
		return nil, domain.ErrForbidden
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	var orders []models.Order
	if err := a.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").Limit(limit).Find(&orders).Error; err != nil {
		return nil, err
	}
	return a.decorate(ctx, orders)
}

func (a OrderAdapter) OrderForUser(ctx context.Context, userID uint, orderNo string) (*contract.OrderContext, error) {
	if a.DB == nil || userID == 0 || strings.TrimSpace(orderNo) == "" {
		return nil, nil
	}
	var order models.Order
	err := a.DB.WithContext(ctx).Where("order_no = ? AND user_id = ?", strings.TrimSpace(orderNo), userID).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := a.decorate(ctx, []models.Order{order})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// decorate 补上商品名并生成交付摘要。摘要只说明「发了什么类型、几条」，
// 卡密原文绝不进入 AI 上下文。
func (a OrderAdapter) decorate(ctx context.Context, orders []models.Order) ([]contract.OrderContext, error) {
	out := make([]contract.OrderContext, 0, len(orders))
	for _, order := range orders {
		item := contract.OrderContext{
			OrderNo: order.OrderNo, Quantity: order.Quantity,
			TotalAmount: order.TotalAmount, Status: order.Status,
			FulfillmentStatus: order.FulfillmentStatus,
			PaidAt:            order.PaidAt, DeliveredAt: order.DeliveredAt,
		}
		var product models.Product
		if err := a.DB.WithContext(ctx).Select("id", "name").First(&product, order.ProductID).Error; err == nil {
			item.ProductName = product.Name
		}
		item.DeliverySummary = deliverySummary(order)
		out = append(out, item)
	}
	return out, nil
}

// deliverySummary 只描述交付形态：卡密条数、人工发货状态或等待补货。
func deliverySummary(order models.Order) string {
	switch order.FulfillmentStatus {
	case "delivered", "completed":
		if order.DeliveryContent != nil && strings.TrimSpace(*order.DeliveryContent) != "" {
			lines := strings.Split(strings.TrimSpace(*order.DeliveryContent), "\n")
			return fmt.Sprintf("已交付，共 %d 项内容（卡密不在此处展示）", len(lines))
		}
		return "已交付"
	case "manual_pending":
		return "已付款，等待商家人工发货"
	case "waiting_stock":
		return "已付款，正在等待补货后自动发货"
	case "plugin_pending":
		return "已付款，交付提供方处理中"
	case "failed":
		return "交付遇到问题，系统正在自动重试"
	default:
		if order.Status == "paid" {
			return "已付款，交付即将开始"
		}
		return "尚未付款"
	}
}

// ── 商品适配器 ───────────────────────────────────────────────────────

// CatalogAdapter 只暴露已上架商品的公开字段。
type CatalogAdapter struct {
	DB *gorm.DB
}

func (a CatalogAdapter) ProductFacts(ctx context.Context, limit int) ([]contract.ProductFact, error) {
	if a.DB == nil {
		return nil, domain.ErrNotConfigured
	}
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	var products []models.Product
	if err := a.DB.WithContext(ctx).Preload("Category").
		Where("is_published = ? AND is_archived = ?", true, false).
		Order("sort_order ASC, id DESC").Limit(limit).Find(&products).Error; err != nil {
		return nil, err
	}
	out := make([]contract.ProductFact, 0, len(products))
	for _, product := range products {
		out = append(out, productFact(product))
	}
	return out, nil
}

func (a CatalogAdapter) ProductFact(ctx context.Context, productID uint) (*contract.ProductFact, error) {
	if a.DB == nil {
		return nil, domain.ErrNotConfigured
	}
	var product models.Product
	if err := a.DB.WithContext(ctx).Preload("Category").First(&product, productID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if !product.IsPublished || product.IsArchived {
		return nil, nil
	}
	fact := productFact(product)
	return &fact, nil
}

func productFact(product models.Product) contract.ProductFact {
	fact := contract.ProductFact{
		ID: product.ID, Name: product.Name, Price: product.Price,
		Stock: product.StockCount, Published: product.IsPublished,
	}
	if product.Summary != nil {
		fact.Summary = *product.Summary
	}
	if product.Category != nil {
		fact.Category = product.Category.Name
	}
	return fact
}

// ── 活动适配器 ───────────────────────────────────────────────────────

// ActivityAdapter 让 AI 介绍正在进行的活动。
type ActivityAdapter struct {
	DB *gorm.DB
}

func (a ActivityAdapter) ActiveActivities(ctx context.Context, limit int) ([]contract.ActivityFact, error) {
	if a.DB == nil {
		return nil, domain.ErrNotConfigured
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	now := time.Now().UTC()
	var activities []models.Activity
	if err := a.DB.WithContext(ctx).
		Where("status = ?", models.ActivityStatusRunning).
		Where("(start_at IS NULL OR start_at <= ?) AND (end_at IS NULL OR end_at > ?)", now, now).
		Order("sort_order ASC, id DESC").Limit(limit).Find(&activities).Error; err != nil {
		return nil, err
	}
	out := make([]contract.ActivityFact, 0, len(activities))
	for _, activity := range activities {
		out = append(out, contract.ActivityFact{
			ID: activity.ID, Name: activity.Name, Subtitle: activity.Subtitle,
			Type: activity.Type, TypeName: models.ActivityTypeLabels[activity.Type],
			StartAt: activity.StartAt, EndAt: activity.EndAt,
		})
	}
	return out, nil
}

// ── 通知适配器 ───────────────────────────────────────────────────────

// NotifierAdapter 把 AI 的站内通知写进买家的收件箱，可选再发一封邮件。
type NotifierAdapter struct {
	Send func(ctx context.Context, notification *models.Notification) error
	Mail func(ctx context.Context, userID uint, subject, body string) error
}

func (a NotifierAdapter) NotifyUser(ctx context.Context, userID uint, notificationType, title, content, link string) error {
	if a.Send == nil || userID == 0 {
		return domain.ErrNotConfigured
	}
	notification := &models.Notification{UserID: userID, Type: notificationType, Title: title}
	if strings.TrimSpace(content) != "" {
		value := content
		notification.Content = &value
	}
	if strings.TrimSpace(link) != "" {
		value := link
		notification.Link = &value
	}
	if err := a.Send(ctx, notification); err != nil {
		return err
	}
	if a.Mail != nil {
		if err := a.Mail(ctx, userID, title, content); err != nil {
			supportlog("ai notification mail failed: %v", err)
		}
	}
	return nil
}

// MailAdapter 使用店铺的 SMTP 配置发信；未开启时静默跳过。
type MailAdapter struct {
	Config MailConfigReader
	Lookup MailAddressReader
}

// MailConfigReader 返回当前 SMTP 配置与是否开启。
type MailConfigReader interface {
	SMTPConfig() (MailSettings, error)
}

// MailSettings 是 mail.Send 需要的连接参数。
type MailSettings struct {
	Host, Username, Password, Secure, From, SiteName string
	Port                                             int
	On                                               bool
}

// MailAddressReader 读取用户的收件地址。
type MailAddressReader interface {
	EmailFor(ctx context.Context, userID uint) (string, error)
}

// settings 读一次 SMTP 配置；未开启或未配置时返回 false。
func (a MailAdapter) settings() (mail.Config, bool, error) {
	if a.Config == nil {
		return mail.Config{}, false, nil
	}
	settings, err := a.Config.SMTPConfig()
	if err != nil {
		return mail.Config{}, false, err
	}
	if !settings.On {
		return mail.Config{}, false, nil
	}
	return mail.Config{
		Host: settings.Host, Port: settings.Port, User: settings.Username,
		Pass: settings.Password, Secure: settings.Secure, From: settings.From,
	}, true, nil
}

func (a MailAdapter) SendToUser(ctx context.Context, userID uint, subject, body string) error {
	if a.Lookup == nil {
		return nil
	}
	settings, on, err := a.settings()
	if err != nil || !on {
		return err
	}
	address, err := a.Lookup.EmailFor(ctx, userID)
	if err != nil || strings.TrimSpace(address) == "" {
		return err
	}
	return settings.Send(mail.Message{To: address, Subject: subject, Body: body})
}

func (a MailAdapter) SendToAddress(ctx context.Context, address, subject, body string) error {
	if strings.TrimSpace(address) == "" {
		return nil
	}
	settings, on, err := a.settings()
	if err != nil || !on {
		return err
	}
	return settings.Send(mail.Message{To: strings.TrimSpace(address), Subject: subject, Body: body})
}

// supportlog 与 application 层保持同样的日志前缀。
func supportlog(format string, args ...any) {
	log.Printf("[support] "+format, args...)
}
