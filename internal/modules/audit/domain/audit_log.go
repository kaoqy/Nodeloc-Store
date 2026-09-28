package domain

import (
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

// AuditLog is the audit module's domain entity. The canonical persistence
// model is shared with the rest of the application through internal/models.
type AuditLog = models.AuditLog

// LogActionInput contains the data required to record an auditable action.
type LogActionInput struct {
	ActorID *uint
	Action  string
	Target  *string
	Detail  *string
	IP      *string
}

// LogFilter controls audit-log queries.
//
// Search matches a substring of the action, target or detail. Since is an
// inclusive lower bound and Before an exclusive upper bound, so an "until
// today" pick still keeps today's entries — that is the pairing the log page's
// date range relies on.
type LogFilter struct {
	Action     string
	Search     string
	ActorID    *uint
	SystemOnly bool
	Since      *time.Time
	Before     *time.Time
	Page       int
	Limit      int
}

// Page is a paginated collection of audit logs.
type Page struct {
	Items      []AuditLog `json:"items"`
	Total      int64      `json:"total"`
	Page       int        `json:"page"`
	Limit      int        `json:"limit"`
	TotalPages int        `json:"total_pages"`
}
