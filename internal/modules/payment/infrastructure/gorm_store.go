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
	return s.db.WithContext(ctx).AutoMigrate(&domain.PaymentOrder{}, &domain.Transaction{}, &domain.Transfer{})
}

func (s *GormStore) CreateTransfer(ctx context.Context, transfer *domain.Transfer) error {
	return s.db.WithContext(ctx).Create(transfer).Error
}

func (s *GormStore) SaveTransfer(ctx context.Context, transfer *domain.Transfer) error {
	return s.db.WithContext(ctx).Save(transfer).Error
}

// ListTransfers returns the 转账 ledger, newest first. A zero userID means the
// whole shop rather than one account, which is what the back office ledger shows.
func (s *GormStore) ListTransfers(ctx context.Context, userID uint, limit, offset int) ([]domain.Transfer, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := s.db.WithContext(ctx).Model(&domain.Transfer{})
	if userID != 0 {
		query = query.Where("user_id = ?", userID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []domain.Transfer
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
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

func (s *GormStore) ListOrdersByUser(ctx context.Context, userID uint, limit, offset int, status, search string) ([]models.Order, int64, error) {
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
	if pattern := strings.TrimSpace(search); pattern != "" {
		like := "%" + pattern + "%"
		productIDs := s.db.Model(&models.Product{}).Select("id").Where("name LIKE ?", like)
		query = query.Where("order_no LIKE ? OR transaction_id LIKE ? OR product_id IN (?)", like, like, productIDs)
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
				"status":  "paid",
				"paid_at": now,
			}
			// A signed redirect can settle a payment without naming a transaction
			// id; blanking the one the order already carries would erase the
			// receipt.
			if strings.TrimSpace(transactionID) != "" {
				updates["transaction_id"] = transactionID
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
			// A promotion is spent when the money arrives, not when the order is
			// written: an abandoned checkout must not eat a limited code. This sits
			// inside the status flip, so a callback NodeLoc sends twice counts once.
			if order.CouponID != nil && *order.CouponID != 0 {
				if err := tx.Model(&models.Coupon{}).Where("id = ?", *order.CouponID).
					UpdateColumn("used_count", gorm.Expr("used_count + 1")).Error; err != nil {
					return err
				}
			}
		}

		return tx.Preload("Product").Preload("Cards").Preload("Records").First(&paid, order.ID).Error
	})
	return &paid, err
}

// MarkOrderDeliveryPending records that the payment is confirmed while delivery
// did not complete, so the buyer sees why the content is missing and the
// background sweep retries. An order that already delivered is never rewritten.
func (s *GormStore) MarkOrderDeliveryPending(ctx context.Context, orderNo, note string) error {
	return s.db.WithContext(ctx).Model(&models.Order{}).
		Where("order_no = ? AND fulfillment_status NOT IN ?", strings.TrimSpace(orderNo), []string{"delivered", "completed"}).
		Updates(map[string]any{"fulfillment_status": "pending", "delivery_note": note}).Error
}

// ListUndeliveredPaidOrders returns paid orders still owed a delivery: those
// whose automatic delivery never ran, and those parked in 等待补货 that a later
// card import should have released. The money is already captured in both, so
// what is retried is the delivery, whatever the order row went on to say.
func (s *GormStore) ListUndeliveredPaidOrders(ctx context.Context, limit int) ([]models.Order, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	orders := make([]models.Order, 0, limit)
	err := s.db.WithContext(ctx).Model(&models.Order{}).
		Where("status IN ? AND fulfillment_status IN ?", []string{"paid", "completed"}, []string{"pending", "waiting_stock"}).
		Where("paid_at IS NOT NULL AND paid_at <= ?", time.Now().UTC().Add(-time.Minute)).
		Preload("Product").
		Order("paid_at ASC, id ASC").
		Limit(limit).
		Find(&orders).Error
	return orders, err
}

// ListUndeliveredPaidOrdersForProduct is one product's waiting queue: the orders
// that paid for keys this shelf did not have when the payment landed. Unlike the
// background sweep it puts no age floor on the order — the buyer already waited
// for the restock, and Fulfill locks the row and refuses anything unpaid, so a
// payment still settling cannot be delivered twice by these two paths.
func (s *GormStore) ListUndeliveredPaidOrdersForProduct(ctx context.Context, productID uint, limit int) ([]models.Order, error) {
	if productID == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	orders := make([]models.Order, 0, limit)
	err := s.db.WithContext(ctx).Model(&models.Order{}).
		Where("product_id = ? AND status IN ? AND fulfillment_status IN ?",
			productID, []string{"paid", "completed"}, []string{"pending", "waiting_stock"}).
		Preload("Product").
		Order("paid_at ASC, id ASC").
		Limit(limit).
		Find(&orders).Error
	return orders, err
}

// CountUndeliveredPaidOrdersByProduct answers "and how many are already waiting
// for this product" in one query, for the restocking queue.
func (s *GormStore) CountUndeliveredPaidOrdersByProduct(ctx context.Context) (map[uint]int64, error) {
	var rows []struct {
		ProductID uint
		Waiting   int64
	}
	err := s.db.WithContext(ctx).Model(&models.Order{}).
		Select("product_id, COUNT(*) AS waiting").
		Where("status IN ? AND fulfillment_status IN ?",
			[]string{"paid", "completed"}, []string{"pending", "waiting_stock"}).
		Group("product_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		if row.ProductID != 0 {
			counts[row.ProductID] = row.Waiting
		}
	}
	return counts, nil
}

// ListReconcilableOrders returns orders the store still calls 待支付 but which do
// carry a NodeLoc transaction id — the set a lost browser redirect can strand,
// and the one 批量查单 can settle.
func (s *GormStore) ListReconcilableOrders(ctx context.Context, limit int, minAge time.Duration) ([]models.Order, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	withTransaction := s.db.Model(&domain.PaymentOrder{}).
		Select("order_id").
		Where("provider_transaction_id IS NOT NULL AND provider_transaction_id <> ''")
	query := s.db.WithContext(ctx).Model(&models.Order{}).
		Where("status = ? AND id IN (?)", "pending", withTransaction)
	if minAge > 0 {
		query = query.Where("created_at <= ?", time.Now().UTC().Add(-minAge))
	}
	orders := make([]models.Order, 0, limit)
	err := query.
		Preload("Product").
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&orders).Error
	return orders, err
}

func (s *GormStore) MarkOrderRefunded(ctx context.Context, orderNo string) error {
	orderNo = strings.TrimSpace(orderNo)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}
		if err := tx.Model(&order).Updates(map[string]any{
			"status": "refunded", "fulfillment_status": "cancelled",
		}).Error; err != nil {
			return err
		}
		// 销量 is a claim about goods the buyer kept. A refund takes it back,
		// and only if the order had ever added to it.
		if isDelivered(order.FulfillmentStatus) && order.ProductID != 0 {
			return tx.Model(&models.Product{}).Where("id = ?", order.ProductID).
				UpdateColumn("sold_count", gorm.Expr("CASE WHEN sold_count >= ? THEN sold_count - ? ELSE 0 END", order.Quantity, order.Quantity)).Error
		}
		return nil
	})
}

func (s *GormStore) ListAllOrders(ctx context.Context, limit, offset int, status, search string, buyerID uint, attention string) ([]models.Order, int64, error) {
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
	if attention == "undelivered" {
		// The money is in; only the goods are missing. manual_pending counts here
		// too, because 人工发货 queued and never done is the shop owner's job.
		query = query.Where("status IN ? AND fulfillment_status IN ?",
			[]string{"paid", "completed"}, []string{"pending", "manual_pending", "waiting_stock"})
	} else if status != "" {
		query = query.Where("status = ?", status)
	}
	if buyerID != 0 {
		query = query.Where("user_id = ?", buyerID)
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

// SetOrderDeliveryContent is a shop owner delivering a manual product by hand.
// It runs in a transaction because the same write decides whether the sale is
// new: content edited a second time must not count the goods as leaving twice.
func (s *GormStore) SetOrderDeliveryContent(ctx context.Context, orderNo string, content string) (*models.Order, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, ErrOrderNotFound
	}
	now := time.Now().UTC()
	var updated models.Order
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}
		if err := tx.Model(&order).Updates(map[string]any{
			"delivery_content":   content,
			"fulfillment_status": "delivered",
			"delivered_at":       now,
			"status":             "completed",
			// The queued-for-manual-handling note would otherwise keep showing
			// next to the delivered content.
			"delivery_note": nil,
		}).Error; err != nil {
			return err
		}
		if !isDelivered(order.FulfillmentStatus) && order.ProductID != 0 {
			if err := tx.Model(&models.Product{}).Where("id = ?", order.ProductID).
				UpdateColumn("sold_count", gorm.Expr("sold_count + ?", order.Quantity)).Error; err != nil {
				return err
			}
		}
		return tx.Preload("Product").Preload("User").Preload("Cards").Preload("Records").First(&updated, order.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func isDelivered(status string) bool {
	return status == "delivered" || status == "completed"
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

		// Each order carries one delivery record per kind of delivery. The retry
		// sweep calls Fulfill every few minutes, so inserting per attempt would
		// bury the order's history in identical rows; update in place instead.
		record := func(deliveryType, status string, note *string, content *string, completedAt *time.Time) error {
			updates := map[string]any{"status": status, "updated_at": time.Now().UTC()}
			if note != nil {
				updates["note"] = *note
			}
			if content != nil {
				updates["content"] = *content
			}
			if completedAt != nil {
				updates["completed_at"] = *completedAt
			}
			result := tx.Model(&models.DeliveryRecord{}).
				Where("order_id = ? AND delivery_type = ?", current.ID, deliveryType).
				Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				return nil
			}
			return tx.Create(&models.DeliveryRecord{
				OrderID:      current.ID,
				Sequence:     1,
				DeliveryType: deliveryType,
				Status:       status,
				Content:      content,
				Note:         note,
				CompletedAt:  completedAt,
			}).Error
		}

		// A missing product row (deleted after checkout) is treated as manual so
		// payment is never dropped on the floor.
		if current.Product == nil || !current.Product.AutoDeliver || current.Product.ProductType != "card" {
			now := time.Now().UTC()
			note := "本单需要商家人工发货，处理结果会同步到订单中。"
			if err := record("manual", "pending", &note, nil, nil); err != nil {
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

		// Cards already bound to this order were taken for it, so a retry hands
		// those back out rather than pulling fresh ones and selling the same
		// order twice over. Only the remainder comes out of available stock, and
		// only that remainder is subtracted from the product's count.
		var cards []models.Card
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_id = ? AND product_id = ?", current.ID, current.ProductID).
			Order("id ASC").Limit(current.Quantity).Find(&cards).Error; err != nil {
			return err
		}
		reused := len(cards)
		if need := current.Quantity - reused; need > 0 {
			var fresh []models.Card
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("product_id = ? AND status = ?", current.ProductID, "available").
				Order("id ASC").Limit(need).Find(&fresh).Error; err != nil {
				return err
			}
			cards = append(cards, fresh...)
		}
		if len(cards) < current.Quantity {
			note := "支付已完成，卡密库存不足，补货后会自动交付。"
			if err := record("card", "waiting_stock", &note, nil, nil); err != nil {
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

		if taken := current.Quantity - reused; taken > 0 {
			if err := tx.Model(&models.Product{}).Where("id = ?", current.ProductID).
				UpdateColumn("stock_count", gorm.Expr("CASE WHEN stock_count >= ? THEN stock_count - ? ELSE 0 END", taken, taken)).Error; err != nil {
				return err
			}
		}
		// 销量 counts what left the shop. It moves with the delivery rather than
		// with the payment, so an order still waiting for stock is not yet a sale
		// the storefront can advertise.
		if err := tx.Model(&models.Product{}).Where("id = ?", current.ProductID).
			UpdateColumn("sold_count", gorm.Expr("sold_count + ?", current.Quantity)).Error; err != nil {
			return err
		}

		if err := record("card", "completed", nil, &deliveryContent, &now); err != nil {
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
