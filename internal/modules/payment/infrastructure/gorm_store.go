package infrastructure

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrOrderNotFound         = domain.ErrOrderNotFound
	ErrPaymentOrderNotFound  = domain.ErrPaymentOrderNotFound
	ErrInsufficientStock     = domain.ErrInsufficientStock
	ErrProductNotPurchasable = domain.ErrProductNotPurchasable
)

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	if db == nil {
		panic("payment: nil gorm database")
	}
	return &GormStore{db: db}
}

func (s *GormStore) Migrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(&domain.PaymentOrder{}, &domain.Transaction{})
}

func (s *GormStore) CreatePaymentOrder(ctx context.Context, paymentOrder *domain.PaymentOrder) error {
	return s.db.WithContext(ctx).Create(paymentOrder).Error
}

func (s *GormStore) GetPaymentOrderByOrderNo(ctx context.Context, orderNo string) (*domain.PaymentOrder, error) {
	var result domain.PaymentOrder
	err := s.db.WithContext(ctx).Where("order_no = ?", strings.TrimSpace(orderNo)).First(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPaymentOrderNotFound
	}
	return &result, err
}

func (s *GormStore) GetPaymentOrderByTransactionID(ctx context.Context, transactionID string) (*domain.PaymentOrder, error) {
	var result domain.PaymentOrder
	err := s.db.WithContext(ctx).
		Where("provider_transaction_id = ?", strings.TrimSpace(transactionID)).
		First(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPaymentOrderNotFound
	}
	return &result, err
}

func (s *GormStore) SavePaymentOrder(ctx context.Context, paymentOrder *domain.PaymentOrder) error {
	return s.db.WithContext(ctx).Save(paymentOrder).Error
}

func (s *GormStore) CreateTransaction(ctx context.Context, transaction *domain.Transaction) error {
	return s.db.WithContext(ctx).Create(transaction).Error
}

func (s *GormStore) SaveTransaction(ctx context.Context, transaction *domain.Transaction) error {
	return s.db.WithContext(ctx).Save(transaction).Error
}

// GetLatestTransaction returns the newest transaction of the given type for an
// order, or nil when the order has none.
func (s *GormStore) GetLatestTransaction(ctx context.Context, orderNo, transactionType string) (*domain.Transaction, error) {
	var result domain.Transaction
	err := s.db.WithContext(ctx).
		Where("order_no = ? AND type = ?", strings.TrimSpace(orderNo), transactionType).
		Order("id DESC").
		First(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPurchasableProduct loads a product that may be ordered right now.
func (s *GormStore) GetPurchasableProduct(ctx context.Context, slug string) (*models.Product, error) {
	var product models.Product
	err := s.db.WithContext(ctx).
		Where("slug = ? AND is_published = ? AND is_archived = ?", strings.TrimSpace(slug), true, false).
		First(&product).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrProductNotPurchasable
	}
	return &product, err
}

func (s *GormStore) CountAvailableCards(ctx context.Context, productID uint) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.Card{}).
		Where("product_id = ? AND status = ?", productID, "available").
		Count(&count).Error
	return count, err
}

func (s *GormStore) CreateOrder(ctx context.Context, order *models.Order) error {
	return s.db.WithContext(ctx).Create(order).Error
}

func (s *GormStore) GetOrderByNo(ctx context.Context, orderNo string) (*models.Order, error) {
	var order models.Order
	err := s.db.WithContext(ctx).
		Preload("Product").
		Preload("User").
		Preload("Cards").
		Preload("Records").
		Where("order_no = ?", strings.TrimSpace(orderNo)).
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOrderNotFound
	}
	return &order, err
}

func (s *GormStore) ListOrdersByUser(ctx context.Context, userID uint, limit, offset int, status string) ([]models.Order, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := s.db.WithContext(ctx).Model(&models.Order{}).Where("user_id = ?", userID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orders := make([]models.Order, 0)
	err := query.
		Preload("Product").
		Preload("Cards").
		Preload("Records").
		Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&orders).Error
	return orders, total, err
}

func (s *GormStore) MarkOrderPaid(ctx context.Context, orderNo, transactionID string, platformFee, merchantPoints *int) (*models.Order, error) {
	var paid models.Order
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", strings.TrimSpace(orderNo)).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}

		if order.Status != "paid" && order.Status != "completed" {
			now := time.Now().UTC()
			updates := map[string]any{
				"status":         "paid",
				"transaction_id": transactionID,
				"paid_at":        now,
			}
			if platformFee != nil {
				updates["platform_fee"] = *platformFee
			}
			if merchantPoints != nil {
				updates["merchant_points"] = *merchantPoints
			}
			if err := tx.Model(&order).Updates(updates).Error; err != nil {
				return err
			}
		}

		return tx.Preload("Product").Preload("Cards").Preload("Records").First(&paid, order.ID).Error
	})
	return &paid, err
}

func (s *GormStore) MarkOrderRefunded(ctx context.Context, orderNo string) error {
	result := s.db.WithContext(ctx).Model(&models.Order{}).
		Where("order_no = ?", strings.TrimSpace(orderNo)).
		Updates(map[string]any{"status": "refunded", "fulfillment_status": "cancelled"})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func (s *GormStore) ListAllOrders(ctx context.Context, limit, offset int, status, search string) ([]models.Order, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := s.db.WithContext(ctx).Model(&models.Order{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if pattern := strings.TrimSpace(search); pattern != "" {
		like := "%" + pattern + "%"
		userIDs := s.db.Model(&models.User{}).Select("id").Where("username LIKE ? OR email LIKE ?", like, like)
		productIDs := s.db.Model(&models.Product{}).Select("id").Where("name LIKE ? OR slug LIKE ?", like, like)
		query = query.Where("order_no LIKE ? OR transaction_id LIKE ? OR customer_contact LIKE ? OR user_id IN (?) OR product_id IN (?)",
			like, like, like, userIDs, productIDs)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orders := make([]models.Order, 0)
	err := query.
		Preload("Product").
		Preload("User").
		Preload("Cards").
		Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&orders).Error
	return orders, total, err
}

func (s *GormStore) UpdateOrderStatus(ctx context.Context, orderNo string, status string) (*models.Order, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, ErrOrderNotFound
	}
	result := s.db.WithContext(ctx).Model(&models.Order{}).
		Where("order_no = ?", orderNo).
		Update("status", status)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrOrderNotFound
	}
	return s.GetOrderByNo(ctx, orderNo)
}

func (s *GormStore) SetOrderDeliveryContent(ctx context.Context, orderNo string, content string) (*models.Order, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, ErrOrderNotFound
	}
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&models.Order{}).
		Where("order_no = ?", orderNo).
		Updates(map[string]any{
			"delivery_content":   content,
			"fulfillment_status": "delivered",
			"delivered_at":       now,
			"status":             "completed",
			// The queued-for-manual-handling note would otherwise keep showing
			// next to the delivered content.
			"delivery_note": nil,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrOrderNotFound
	}
	return s.GetOrderByNo(ctx, orderNo)
}

// Fulfill atomically performs automatic card delivery. Manual products are
// queued for manual handling, while insufficient card inventory is marked as
// waiting_stock so a later stock import can retry fulfillment safely.
func (s *GormStore) Fulfill(ctx context.Context, order *models.Order) error {
	if order == nil || order.ID == 0 {
		return ErrOrderNotFound
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Product").First(&current, order.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}

		if current.Status != "paid" && current.Status != "completed" {
			return domain.ErrNotPayable
		}
		if current.FulfillmentStatus == "delivered" || current.FulfillmentStatus == "completed" {
			*order = current
			return nil
		}

		// A missing product row (deleted after checkout) is treated as manual so
		// payment is never dropped on the floor.
		if current.Product == nil || !current.Product.AutoDeliver || current.Product.ProductType != "card" {
			now := time.Now().UTC()
			note := "本单需要商家人工发货，处理结果会同步到订单中。"
			record := models.DeliveryRecord{
				OrderID:      current.ID,
				Sequence:     1,
				DeliveryType: "manual",
				Status:       "pending",
				Note:         &note,
				CompletedAt:  nil,
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			if err := tx.Model(&current).Updates(map[string]any{
				"fulfillment_status": "manual_pending",
				"delivery_note":      note,
				"updated_at":         now,
			}).Error; err != nil {
				return err
			}
			return tx.Preload("Product").Preload("Records").First(order, current.ID).Error
		}

		var cards []models.Card
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("product_id = ? AND status = ?", current.ProductID, "available").
			Order("id ASC").Limit(current.Quantity).Find(&cards).Error; err != nil {
			return err
		}
		if len(cards) < current.Quantity {
			note := "支付已完成，卡密库存不足，补货后会自动交付。"
			record := models.DeliveryRecord{
				OrderID:      current.ID,
				Sequence:     1,
				DeliveryType: "card",
				Status:       "waiting_stock",
				Note:         &note,
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			if err := tx.Model(&current).Updates(map[string]any{
				"fulfillment_status": "waiting_stock",
				"delivery_note":      note,
			}).Error; err != nil {
				return err
			}
			*order = current
			order.FulfillmentStatus = "waiting_stock"
			order.DeliveryNote = &note
			return nil
		}

		now := time.Now().UTC()
		contents := make([]string, 0, len(cards))
		cardIDs := make([]uint, 0, len(cards))
		for _, card := range cards {
			contents = append(contents, card.Content)
			cardIDs = append(cardIDs, card.ID)
		}
		deliveryContent := strings.Join(contents, "\n")

		if err := tx.Model(&models.Card{}).Where("id IN ?", cardIDs).Updates(map[string]any{
			"status":   "sold",
			"order_id": current.ID,
			"sold_at":  now,
		}).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.Product{}).Where("id = ?", current.ProductID).
			UpdateColumn("stock_count", gorm.Expr("CASE WHEN stock_count >= ? THEN stock_count - ? ELSE 0 END", current.Quantity, current.Quantity)).Error; err != nil {
			return err
		}

		record := models.DeliveryRecord{
			OrderID:      current.ID,
			Sequence:     1,
			DeliveryType: "card",
			Status:       "completed",
			Content:      &deliveryContent,
			CompletedAt:  &now,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}

		if err := tx.Model(&current).Updates(map[string]any{
			"status":             "completed",
			"fulfillment_status": "delivered",
			"delivery_content":   deliveryContent,
			"delivered_at":       now,
			// Drop the "waiting for stock" note so the buyer only sees the cards.
			"delivery_note": nil,
		}).Error; err != nil {
			return err
		}

		return tx.Preload("Product").Preload("Cards").Preload("Records").First(order, current.ID).Error
	})
}
