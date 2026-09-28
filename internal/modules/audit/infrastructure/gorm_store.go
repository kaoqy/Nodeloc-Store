package infrastructure

import (
	"context"
	"strings"

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
	query := s.filtered(ctx, filter)

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

// Actions returns the distinct recorded action names.
func (s *GormStore) Actions(ctx context.Context) ([]string, error) {
	actions := make([]string, 0)
	if err := s.db.WithContext(ctx).Model(&domain.AuditLog{}).
		Distinct().Order("action ASC").Pluck("action", &actions).Error; err != nil {
		return nil, err
	}
	return actions, nil
}

func (s *GormStore) filtered(ctx context.Context, filter domain.LogFilter) *gorm.DB {
	query := s.db.WithContext(ctx).Model(&domain.AuditLog{})
	if filter.Action != "" {
		// Prefix match so "order" also finds order.cancel and order.refund.
		query = query.Where("action LIKE ? ESCAPE '"+likeEscape+"'", escapeLike(filter.Action)+likeAny)
	}
	if filter.Search != "" {
		pattern := likeAny + escapeLike(filter.Search) + likeAny
		// The alternatives are parenthesised so AND with the other filters keeps
		// its precedence.
		query = query.Where(
			"(action LIKE ? ESCAPE '"+likeEscape+"' OR target LIKE ? ESCAPE '"+likeEscape+"' OR detail LIKE ? ESCAPE '"+likeEscape+"')",
			pattern, pattern, pattern,
		)
	}
	if filter.ActorID != nil {
		query = query.Where("actor_id = ?", *filter.ActorID)
	}
	if filter.SystemOnly {
		query = query.Where("actor_id IS NULL")
	}
	if filter.Since != nil {
		query = query.Where("created_at >= ?", *filter.Since)
	}
	if filter.Before != nil {
		query = query.Where("created_at < ?", *filter.Before)
	}
	return query
}

// LIKE wildcards in a typed keyword would silently widen the match, so they are
// escaped. '/' is the escape character because a backslash needs different
// quoting in SQLite and MySQL string literals.
const (
	likeEscape = "/"
	likeAny    = "%"
)

func escapeLike(value string) string {
	replacer := strings.NewReplacer(likeEscape, likeEscape+likeEscape, "%", likeEscape+"%", "_", likeEscape+"_")
	return replacer.Replace(value)
}
