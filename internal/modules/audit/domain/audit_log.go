package domain

import "github.com/kaoqy/Nodeloc-Store/internal/models"

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
type LogFilter struct {
	Action string
	Page   int
	Limit  int
}

// Page is a paginated collection of audit logs.
type Page struct {
	Items      []AuditLog `json:"items"`
	Total      int64      `json:"total"`
	Page       int        `json:"page"`
	Limit      int        `json:"limit"`
	TotalPages int        `json:"total_pages"`
}
