package application

import (
	"context"
	"errors"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

const (
	defaultPage  = 1
	defaultLimit = 20
	maxLimit     = 100
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
	filter.Action = strings.TrimSpace(filter.Action)
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
