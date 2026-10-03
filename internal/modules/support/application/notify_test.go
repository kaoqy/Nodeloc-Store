package application

import (
	"context"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// TestRenderTemplateSupportsBothSyntaxes keeps the two placeholder styles in
// sync: the seeded templates use {{x}} and older copy used {x}, and a shop may
// have either in its own text.
func TestRenderTemplateSupportsBothSyntaxes(t *testing.T) {
	variables := map[string]string{"ticket_no": "TK1", "status": "已解决"}
	if got := renderTemplate("工单 {{ticket_no}} 变成 {status}", variables); got != "工单 TK1 变成 已解决" {
		t.Fatalf("render = %q", got)
	}
}

// TestNotificationTypeGroups is what the buyer's inbox groups by.
func TestNotificationTypeGroups(t *testing.T) {
	cases := map[string]string{
		"ticket.created":   "ticket",
		"activity.started": "activity",
		"coupon.claimed":   "coupon",
		"order.exception":  "order",
		"other":            "system",
	}
	for key, want := range cases {
		if got := notificationType(key); got != want {
			t.Errorf("notificationType(%q) = %q, want %q", key, got, want)
		}
	}
}

// ── 提醒事件收件人 ──────────────────────────────────────────────────

type fakeNotifier struct{ users []uint }

func (f *fakeNotifier) NotifyUser(_ context.Context, userID uint, _, _, _, _ string) error {
	f.users = append(f.users, userID)
	return nil
}

type fakeMailer struct {
	users     []uint
	addresses []string
}

func (f *fakeMailer) SendToUser(_ context.Context, userID uint, _, _ string) error {
	f.users = append(f.users, userID)
	return nil
}

func (f *fakeMailer) SendToAddress(_ context.Context, address, _, _ string) error {
	f.addresses = append(f.addresses, address)
	return nil
}

type fakeStaff struct {
	users  []uint
	emails []string
}

func (f fakeStaff) UserIDs(context.Context) ([]uint, error)  { return f.users, nil }
func (f fakeStaff) Emails(context.Context) ([]string, error) { return f.emails, nil }

func newNotifyService(t *testing.T, template domain.NotificationTemplate, deps Deps) *Service {
	t.Helper()
	repo := &stubRepo{templates: []domain.NotificationTemplate{template}}
	deps.Repo = repo
	service, err := NewService(deps)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

// TestNormalizeRecipients 三档映射：空与 buyer 归买家，staff/admin 归员工，
// 其它一律当成自定义邮箱，避免把笔误悄悄发给错误的人。
func TestNormalizeRecipients(t *testing.T) {
	cases := map[string]string{
		"":                 "user",
		"  User ":          "user",
		"buyer":            "user",
		"customer":         "user",
		"staff":            "staff",
		" ADMIN ":          "staff",
		"owner":            "staff",
		"boss@example.com": "custom",
		"a@x.com,b@y.com":  "custom",
	}
	for in, want := range cases {
		if got := normalizeRecipients(in); got != want {
			t.Errorf("normalizeRecipients(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestResolveRecipientsCustomOnlyMails 校验自定义邮箱只发邮件、不发站内信，
// 且顺带去重与去空白。
func TestResolveRecipientsCustomOnlyMails(t *testing.T) {
	service := newNotifyService(t, domain.NotificationTemplate{}, Deps{})
	users, addresses := service.resolveRecipients(context.Background(),
		Notification{Recipients: " boss@example.com , ops@example.com ,BOSS@example.com "}, "custom")
	if len(users) != 0 {
		t.Fatalf("custom should not target in-app users, got %v", users)
	}
	if len(addresses) != 2 || addresses[0] != "boss@example.com" || addresses[1] != "ops@example.com" {
		t.Fatalf("addresses = %v", addresses)
	}
}

// TestNotifyRoutesStaffRecipients 是「订单异常发到老板邮箱」的核心：模板写着
// recipients=staff 时，站内发给员工账号、邮件发给员工邮箱，各走各的通道。
func TestNotifyRoutesStaffRecipients(t *testing.T) {
	notifier := &fakeNotifier{}
	mailer := &fakeMailer{}
	staff := fakeStaff{users: []uint{3, 4}, emails: []string{"boss@example.com", "BOSS@example.com"}}
	service := newNotifyService(t, domain.NotificationTemplate{
		Key: "order.exception", IsEnabled: true, InApp: true, Mail: true,
		TitleTemplate: "订单 {{order_no}} 需要关注", ContentTemplate: "{{detail}}",
		Recipients: "staff",
	}, Deps{Notifier: notifier, Mailer: mailer, Staff: staff})

	service.notify(context.Background(), "order.exception",
		map[string]string{"order_no": "A100", "detail": "库存不足"},
		Notification{Key: "order.exception", Title: "兜底标题", Content: "兜底内容"})

	if len(notifier.users) != 2 || notifier.users[0] != 3 || notifier.users[1] != 4 {
		t.Fatalf("in-app users = %v", notifier.users)
	}
	if len(mailer.users) != 0 {
		t.Fatalf("staff routing must not use SendToUser, got %v", mailer.users)
	}
	// 大小写不同的同一地址只发一次。
	if len(mailer.addresses) != 1 || mailer.addresses[0] != "boss@example.com" {
		t.Fatalf("mail addresses = %v", mailer.addresses)
	}
}

// TestNotifyDisabledTemplateIsSilent 确认管理员停用模板后什么都不发。
func TestNotifyDisabledTemplateIsSilent(t *testing.T) {
	notifier := &fakeNotifier{}
	service := newNotifyService(t, domain.NotificationTemplate{
		Key: "ticket.created", IsEnabled: false, InApp: true,
	}, Deps{Notifier: notifier})
	service.notify(context.Background(), "ticket.created", nil,
		Notification{Key: "ticket.created", UserID: 7})
	if len(notifier.users) != 0 {
		t.Fatalf("disabled template still sent to %v", notifier.users)
	}
}

// TestNotifyDefaultsToBuyer 买家模板（Recipients 为空）仍然按 UserID 投递。
func TestNotifyDefaultsToBuyer(t *testing.T) {
	notifier := &fakeNotifier{}
	mailer := &fakeMailer{}
	service := newNotifyService(t, domain.NotificationTemplate{
		Key: "ticket.agent_reply", IsEnabled: true, InApp: true, Mail: true,
	}, Deps{Notifier: notifier, Mailer: mailer})
	service.notify(context.Background(), "ticket.agent_reply", nil,
		Notification{Key: "ticket.agent_reply", UserID: 9, Title: "回复"})
	if len(notifier.users) != 1 || notifier.users[0] != 9 {
		t.Fatalf("in-app users = %v", notifier.users)
	}
	if len(mailer.users) != 1 || mailer.users[0] != 9 {
		t.Fatalf("mail users = %v", mailer.users)
	}
}

func TestDedupeStrings(t *testing.T) {
	got := dedupeStrings([]string{"a@x.com", " A@X.COM ", "", "b@y.com", "a@x.com"})
	if len(got) != 2 || got[0] != "a@x.com" || got[1] != "b@y.com" {
		t.Fatalf("dedupe = %v", got)
	}
}
