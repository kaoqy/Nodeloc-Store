package contract

import (
	"context"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

// AuditRepo defines the persistence operations required by the audit service.
type AuditRepo interface {
	Create(ctx context.Context, log *domain.AuditLog) error
	List(ctx context.Context, filter domain.LogFilter) ([]domain.AuditLog, int64, error)
	// Export walks the same filtered rows without stopping at one page, for the
	// CSV download. limit caps how many rows come back; the second return says
	// the cap was reached, because a shortened file must not look complete.
	Export(ctx context.Context, filter domain.LogFilter, limit int) ([]domain.AuditLog, bool, error)
	// Actions lists the distinct action names already recorded, so the log page
	// can offer the filters the shop has actually used.
	Actions(ctx context.Context) ([]string, error)
}
