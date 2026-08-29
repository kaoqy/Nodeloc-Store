package contract

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

// AuditRepo defines the persistence operations required by the audit service.
type AuditRepo interface {
	Create(ctx context.Context, log *domain.AuditLog) error
	List(ctx context.Context, filter domain.LogFilter) ([]domain.AuditLog, int64, error)
}
