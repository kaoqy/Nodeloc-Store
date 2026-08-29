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
	ErrOrderNotFound        = errors.New("order not found")
	ErrPaymentOrderNotFound = errors.New("payment order not found")
	ErrInsufficientStock    = errors.New("insufficient card stock")
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

func (s *GormStore) GetOrderByNo(ctx context.Context, orderNo string) (*models.Order, error) {
	var order models.Order
	err := s.db.WithContext(ctx).
		Preload("Product").
		Preload("Cards").
		Preload("Records").
		Where("order_no = ?", strings.TrimSpace(orderNo)).
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOrderNotFound
	}
	return &order, err
}

func (s *GormStore) ListOrdersByUser(ctx context.Context, userID uint, limit, offset int) ([]models.Order, int64, error) {
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
			return errors.New("order is not paid")
		}
		if current.FulfillmentStatus == "delivered" || current.FulfillmentStatus == "completed" {
			*order = current
			return nil
		}

		if !current.Product.AutoDeliver || current.Product.ProductType != "card" {
			now := time.Now().UTC()
			note := "Order requires manual delivery"
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
			note := "Payment received; waiting for card stock"
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
			"status":              "completed",
			"fulfillment_status": "delivered",
			"delivery_content":   deliveryContent,
			"delivered_at":       now,
		}).Error; err != nil {
			return err
		}

		return tx.Preload("Product").Preload("Cards").Preload("Records").First(order, current.ID).Error
	})
}
