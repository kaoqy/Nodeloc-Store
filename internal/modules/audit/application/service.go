package application

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

const (
	defaultPage  = 1
	defaultLimit = 20
	maxLimit     = 100
	// maxSearchLen keeps a keyword from turning into a full-table pattern match
	// on the longest text column the log row carries.
	maxSearchLen = 64
	// maxExportLogs bounds one download. A shop that outgrows it should narrow
	// the date range first instead of pulling its whole history into a file.
	maxExportLogs = 20000
)

var ErrActionRequired = errors.New("audit action is required")

// Service implements audit logging and query use cases.
type Service struct {
	repo contract.AuditRepo
}

func NewService(repo contract.AuditRepo) *Service {
	if repo == nil {
		panic("audit: nil repository")
	}
	return &Service{repo: repo}
}

func (s *Service) LogAction(ctx context.Context, input domain.LogActionInput) (*domain.AuditLog, error) {
	action := strings.TrimSpace(input.Action)
	if action == "" {
		return nil, ErrActionRequired
	}

	log := &domain.AuditLog{
		ActorID: input.ActorID,
		Action:  action,
		Target:  trimOptional(input.Target),
		Detail:  trimOptional(input.Detail),
		IP:      trimOptional(input.IP),
	}
	if err := s.repo.Create(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *Service) QueryLogs(ctx context.Context, filter domain.LogFilter) (domain.Page, error) {
	filter = normalizeFilter(filter)
	if filter.Page < 1 {
		filter.Page = defaultPage
	}
	if filter.Limit < 1 {
		filter.Limit = defaultLimit
	} else if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}

	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return domain.Page{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(filter.Limit) - 1) / int64(filter.Limit))
	}
	return domain.Page{
		Items: items, Total: total, Page: filter.Page,
		Limit: filter.Limit, TotalPages: totalPages,
	}, nil
}

// ExportLogs runs a download through the exact same filter normalisation as the
// page, so the CSV can only ever carry what the owner was looking at.
func (s *Service) ExportLogs(ctx context.Context, filter domain.LogFilter) ([]domain.AuditLog, bool, error) {
	return s.repo.Export(ctx, normalizeFilter(filter), maxExportLogs)
}

func normalizeFilter(filter domain.LogFilter) domain.LogFilter {
	filter.Action = strings.TrimSpace(filter.Action)
	filter.Search = strings.TrimSpace(filter.Search)
	if runes := []rune(filter.Search); len(runes) > maxSearchLen {
		filter.Search = string(runes[:maxSearchLen])
	}
	return filter
}

// ListActions names the actions this shop has recorded, so the log page can
// offer filters that match real history instead of a hand-written guess.
func (s *Service) ListActions(ctx context.Context) ([]string, error) {
	actions, err := s.repo.Actions(ctx)
	if err != nil {
		return nil, err
	}
	sort.Strings(actions)
	return actions, nil
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
