package infrastructure

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
	"gorm.io/gorm"
)

var _ contract.AuditRepo = (*GormStore)(nil)

// GormStore persists audit logs using GORM.
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	if db == nil {
		panic("audit: nil gorm database")
	}
	return &GormStore{db: db}
}

func (s *GormStore) Create(ctx context.Context, log *domain.AuditLog) error {
	return s.db.WithContext(ctx).Create(log).Error
}

func (s *GormStore) List(ctx context.Context, filter domain.LogFilter) ([]domain.AuditLog, int64, error) {
	query := s.db.WithContext(ctx).Model(&domain.AuditLog{})
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	logs := make([]domain.AuditLog, 0)
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(filter.Limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
