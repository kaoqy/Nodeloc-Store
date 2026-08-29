package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/payment/domain"
	"gorm.io/gorm"
)

// FulfillmentService handles order fulfillment after payment.
type FulfillmentService struct {
	db *gorm.DB
}

func NewFulfillmentService(db *gorm.DB) *FulfillmentService {
	return &FulfillmentService{db: db}
}

// Fulfill processes a paid order: auto-deliver cards or queue for manual delivery.
func (s *FulfillmentService) Fulfill(ctx context.Context, order *models.Order) error {
	if order == nil {
		return fmt.Errorf("order is required")
	}
	if order.ID == 0 {
		return fmt.Errorf("order has no ID")
	}

	var product models.Product
	if err := s.db.WithContext(ctx).First(&product, order.ProductID).Error; err != nil {
		return fmt.Errorf("lookup product: %w", err)
	}

	if product.ProductType == "manual" {
		return s.queueManualDelivery(ctx, order)
	}
	return s.autoDeliverCard(ctx, order, &product)
}

func (s *FulfillmentService) queueManualDelivery(ctx context.Context, order *models.Order) error {
	s.db.WithContext(ctx).Model(&order).Updates(map[string]interface{}{
		"fulfillment_status": "awaiting_manual",
	})
	return s.db.WithContext(ctx).Create(&models.DeliveryRecord{
		OrderID:      order.ID,
		Sequence:     1,
		DeliveryType: "manual",
		Status:       "awaiting_manual",
	}).Error
}

func (s *FulfillmentService) autoDeliverCard(ctx context.Context, order *models.Order, product *models.Product) error {
	tx := s.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var card models.Card
	err := tx.Where("product_id = ? AND status = ?", product.ID, "available").
		Order("id asc").
		First(&card).Error
	if err != nil {
		tx.Rollback()
		// No cards available: mark as waiting
		tx.Model(&order).Update("fulfillment_status", "waiting_stock")
		tx.Create(&models.DeliveryRecord{
			OrderID:      order.ID,
			Sequence:     1,
			DeliveryType: "card",
			Status:       "waiting_stock",
			Note:         "等待库存补充后自动发货",
		})
		return tx.Commit().Error
	}

	now := time.Now().UTC()
	tx.Model(&card).Updates(map[string]interface{}{
		"status":   "sold",
		"order_id": order.ID,
		"sold_at":  now,
	})
	tx.Model(&order).Updates(map[string]interface{}{
		"delivered_at":        now,
		"fulfillment_status":  "delivered",
	})
	tx.Create(&models.DeliveryRecord{
		OrderID:      order.ID,
		Sequence:     1,
		DeliveryType: "card",
		Status:       "delivered",
		Content:      card.Content,
		Note:         fmt.Sprintf("card_id=%d", card.ID),
		CompletedAt:  &now,
	})

	// Update stock count
	var count int64
	tx.Model(&models.Card{}).Where("product_id = ? AND status = ?", product.ID, "available").Count(&count)
	tx.Model(&product).Update("stock_count", count)

	return tx.Commit().Error
}

// Refund releases cards back to available status.
func (s *FulfillmentService) Refund(ctx context.Context, order *models.Order) error {
	var cards []models.Card
	if err := s.db.WithContext(ctx).Where("order_id = ?", order.ID).Find(&cards).Error; err != nil {
		return err
	}
	for _, card := range cards {
		s.db.WithContext(ctx).Model(&card).Updates(map[string]interface{}{
			"status":   "available",
			"order_id": nil,
			"sold_at":  nil,
		})
	}
	return nil
}
