package application

import (
	"context"
	"testing"
	"time"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// ticketStateRepo keeps one ticket in memory so the close path can be checked
// end to end without a database.
type ticketStateRepo struct {
	stubRepo
	ticket *domain.Ticket
	logs   []domain.TicketLog
}

func (r *ticketStateRepo) GetTicket(context.Context, uint) (*domain.Ticket, error) {
	if r.ticket == nil {
		return nil, domain.ErrTicketNotFound
	}
	copy := *r.ticket
	return &copy, nil
}

func (r *ticketStateRepo) UpdateTicket(_ context.Context, ticket *domain.Ticket) error {
	copy := *ticket
	r.ticket = &copy
	return nil
}

func (r *ticketStateRepo) AppendTicketLog(_ context.Context, entry *domain.TicketLog) error {
	r.logs = append(r.logs, *entry)
	return nil
}

func TestBuyerClosePersistsTicketStatus(t *testing.T) {
	repo := &ticketStateRepo{ticket: &domain.Ticket{
		Base:     models.Base{ID: 7},
		TicketNo: "TK0007",
		UserID:   42,
		Status:   models.TicketStatusPendingHuman,
		Handler:  models.TicketHandlerHuman,
	}}
	service, err := NewService(Deps{Repo: repo})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	fixed := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }

	if _, err := service.SetTicketStatus(context.Background(), 7, models.TicketStatusCancelled, 42, "user", "用户关闭工单"); err != nil {
		t.Fatalf("close ticket: %v", err)
	}
	fresh, err := repo.GetTicket(context.Background(), 7)
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if fresh.Status != models.TicketStatusCancelled {
		t.Fatalf("status = %q, want %q", fresh.Status, models.TicketStatusCancelled)
	}
	if len(repo.logs) != 1 || repo.logs[0].Action != "ticket.status" {
		t.Fatalf("close log = %+v, want one ticket.status entry", repo.logs)
	}
}

