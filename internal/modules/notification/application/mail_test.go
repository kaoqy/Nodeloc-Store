package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/contract"
)

// mailRepo is the one half of the notification store a mail test touches.
type mailRepo struct {
	contract.NotificationRepo
	created []models.Notification
	failOn  error
}

func (r *mailRepo) Create(_ context.Context, notification *models.Notification) error {
	if r.failOn != nil {
		return r.failOn
	}
	r.created = append(r.created, *notification)
	return nil
}

type stubAddresses struct {
	email string
	err   error
	calls int
}

func (a *stubAddresses) EmailFor(context.Context, uint) (string, error) {
	a.calls++
	return a.email, a.err
}

type stubMailConfig struct {
	config contract.MailConfig
	on     bool
	calls  int
}

func (m *stubMailConfig) MailConfig(context.Context) (contract.MailConfig, bool) {
	m.calls++
	return m.config, m.on
}

type stubSender struct {
	sent    []sentMail
	err     error
	noCalls bool
}

type sentMail struct {
	config            contract.MailConfig
	to, subject, body string
}

func (s *stubSender) SendMail(config contract.MailConfig, to, subject, body string) error {
	if s.noCalls {
		panic("the sender should not have been called")
	}
	s.sent = append(s.sent, sentMail{config: config, to: to, subject: subject, body: body})
	return s.err
}

func mailService(repo *mailRepo) *Service {
	return NewService(repo, nil)
}

func link(value string) *string { return &value }

// A written notification is mirrored to the account's address with the shop's
// name in the subject and a clickable link in the body.
func TestSendMirrorsToEmailWhenConfigured(t *testing.T) {
	repo := &mailRepo{}
	service := mailService(repo)
	addresses := &stubAddresses{email: "buyer@example.com"}
	config := &stubMailConfig{on: true, config: contract.MailConfig{
		Host: "smtp.example.com", Port: 587, From: "shop@example.com",
		SiteName: "我的商店", BaseURL: "https://shop.example.com",
	}}
	sender := &stubSender{}
	service.EnableMail(addresses, config, sender)

	content := "订单 NL1001 的 180 NL 已退回你的 NodeLoc 账户"
	err := service.Send(context.Background(), &models.Notification{
		UserID: 7, Type: "order", Title: "订单已退款", Content: &content, Link: link("/orders/NL1001"),
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("站内 notifications written = %d, want one", len(repo.created))
	}
	if len(sender.sent) != 1 {
		t.Fatalf("emails sent = %d, want one", len(sender.sent))
	}
	mail := sender.sent[0]
	if mail.to != "buyer@example.com" {
		t.Fatalf("email went to %q", mail.to)
	}
	if !strings.Contains(mail.subject, "我的商店") || !strings.Contains(mail.subject, "订单已退款") {
		t.Fatalf("subject = %q, want the shop name and the title", mail.subject)
	}
	if !strings.Contains(mail.body, "180 NL") {
		t.Fatalf("body lost the content: %q", mail.body)
	}
	if !strings.Contains(mail.body, "https://shop.example.com/orders/NL1001") {
		t.Fatalf("body did not make the link absolute: %q", mail.body)
	}
}

// The 站内 message is what the badge counts, so an SMTP problem must never turn a
// delivered notification into a failed one.
func TestSendSurvivesAFailingMailServer(t *testing.T) {
	repo := &mailRepo{}
	service := mailService(repo)
	service.EnableMail(
		&stubAddresses{email: "buyer@example.com"},
		&stubMailConfig{on: true, config: contract.MailConfig{Host: "smtp.example.com", From: "shop@example.com"}},
		&stubSender{err: errors.New("connection refused")},
	)

	if err := service.Send(context.Background(), &models.Notification{UserID: 7, Type: "order", Title: "订单已发货"}); err != nil {
		t.Fatalf("a failing mail server failed the notification: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatal("the 站内 notification was not written")
	}
}

// No SMTP, no address, or SMTP switched off means no mail — and no error either.
func TestSendSkipsMailWhenItCannotBeSent(t *testing.T) {
	cases := map[string]struct {
		address  string
		on       bool
		wantMail bool
	}{
		"no address":               {address: "", on: true},
		"switched off":             {address: "buyer@example.com", on: false},
		"configured and addressed": {address: "buyer@example.com", on: true, wantMail: true},
	}
	for name, tc := range cases {
		repo := &mailRepo{}
		service := mailService(repo)
		sender := &stubSender{}
		service.EnableMail(
			&stubAddresses{email: tc.address},
			&stubMailConfig{on: tc.on, config: contract.MailConfig{Host: "smtp.example.com", From: "shop@example.com"}},
			sender,
		)
		if err := service.Send(context.Background(), &models.Notification{UserID: 7, Type: "order", Title: "订单动态"}); err != nil {
			t.Fatalf("%s: Send: %v", name, err)
		}
		if got := len(sender.sent) > 0; got != tc.wantMail {
			t.Fatalf("%s: mail sent = %v, want %v", name, got, tc.wantMail)
		}
	}
}

// A shop with no mail wiring at all is the pre-existing behaviour: the in-app
// notification still lands.
func TestSendWorksWithoutAnyMailWiring(t *testing.T) {
	repo := &mailRepo{}
	service := mailService(repo)
	if err := service.Send(context.Background(), &models.Notification{UserID: 7, Type: "system", Title: "公告"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatal("the 站内 notification was not written")
	}
}
