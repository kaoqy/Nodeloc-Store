package infrastructure

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/domain"
)

// GormStore is the plugin module's persistence adapter.
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) ListPlugins(ctx context.Context) ([]domain.Plugin, error) {
	var plugins []domain.Plugin
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&plugins).Error; err != nil {
		return nil, err
	}
	return plugins, nil
}

func (s *GormStore) CountPlugins(ctx context.Context) (int64, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Plugin{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *GormStore) FindPlugin(ctx context.Context, id uint) (*domain.Plugin, error) {
	var plugin domain.Plugin
	if err := s.db.WithContext(ctx).First(&plugin, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPluginNotFound
		}
		return nil, err
	}
	return &plugin, nil
}

func (s *GormStore) FindPluginByKey(ctx context.Context, key string) (*domain.Plugin, error) {
	var plugin domain.Plugin
	if err := s.db.WithContext(ctx).Where("key = ?", key).First(&plugin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &plugin, nil
}

func (s *GormStore) CreatePlugin(ctx context.Context, plugin *domain.Plugin) error {
	return s.db.WithContext(ctx).Create(plugin).Error
}

func (s *GormStore) UpdatePlugin(ctx context.Context, plugin *domain.Plugin) error {
	return s.db.WithContext(ctx).Save(plugin).Error
}

func (s *GormStore) DeletePlugin(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("plugin_id = ?", id).Delete(&models.PluginBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Plugin{}, id).Error
	})
}

func (s *GormStore) ListBindings(ctx context.Context, pluginID uint) ([]domain.PluginBinding, error) {
	query := s.db.WithContext(ctx).Model(&models.PluginBinding{})
	if pluginID != 0 {
		query = query.Where("plugin_id = ?", pluginID)
	}
	var bindings []domain.PluginBinding
	if err := query.Order("sort_order ASC, value ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

func (s *GormStore) ListBindingsForProduct(ctx context.Context, productID uint) ([]domain.PluginBinding, error) {
	var bindings []domain.PluginBinding
	if err := s.db.WithContext(ctx).
		Where("product_id = ?", productID).
		Order("sort_order ASC, value ASC").
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

func (s *GormStore) ResolveBinding(ctx context.Context, productID uint, value string) (*domain.PluginBinding, error) {
	var binding domain.PluginBinding
	if err := s.db.WithContext(ctx).
		Where("product_id = ? AND value = ? AND is_enabled = ?", productID, value, true).
		First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrBindingNotFound
		}
		return nil, err
	}
	return &binding, nil
}

func (s *GormStore) FindBinding(ctx context.Context, id uint) (*domain.PluginBinding, error) {
	var binding domain.PluginBinding
	if err := s.db.WithContext(ctx).First(&binding, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrBindingNotFound
		}
		return nil, err
	}
	return &binding, nil
}

func (s *GormStore) CreateBinding(ctx context.Context, binding *domain.PluginBinding) error {
	return s.db.WithContext(ctx).Create(binding).Error
}

func (s *GormStore) UpdateBinding(ctx context.Context, binding *domain.PluginBinding) error {
	return s.db.WithContext(ctx).Save(binding).Error
}

func (s *GormStore) DeleteBinding(ctx context.Context, id uint) error {
	result := s.db.WithContext(ctx).Delete(&models.PluginBinding{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrBindingNotFound
	}
	return nil
}

func (s *GormStore) ProductExists(ctx context.Context, productID uint) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Product{}).Where("id = ?", productID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// OrderContext joins the names an order is delivered with. A missing product or
// buyer is not an error: the order may have been placed by an account that was
// since removed, and the plugin can still deliver.
func (s *GormStore) OrderContext(ctx context.Context, orderID uint) (*contract.OrderContext, error) {
	var order models.Order
	if err := s.db.WithContext(ctx).First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrInvalidInput
		}
		return nil, err
	}
	info := &contract.OrderContext{}
	var product models.Product
	if err := s.db.WithContext(ctx).First(&product, order.ProductID).Error; err == nil {
		info.ProductTitle = product.Name
	}
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, order.UserID).Error; err == nil {
		info.Username = user.Username
	}
	return info, nil
}
