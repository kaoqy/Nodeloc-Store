package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// Notification 是渲染好的一条通知，模板开关与渠道开关都已经判断完毕。
//
// Recipients 来自模板配置：空表示按 UserID 发给关联买家，staff 表示发给
// 管理端员工，其它值按逗号分隔的自定义邮箱地址处理。
type Notification struct {
	Key        string
	Title      string
	Content    string
	Link       string
	InApp      bool
	Mail       bool
	UserID     uint
	TicketID   uint
	OrderID    uint
	Recipients string
}

// notify 用可配置模板发一条通知：模板缺失时退回调用方给的兜底文案。
//
// 顺序是刻意安排的：先读模板（管理员可停用整条通知），再渲染变量，
// 然后写站内信与邮件，最后把这一次尝试记进 notification_logs。
// 任何一步失败都只写日志，不让已经发生的业务回滚。
func (s *Service) notify(ctx context.Context, key string, variables map[string]string, fallback Notification) {
	notification := fallback
	notification.Key = key

	templates, err := s.NotificationTemplates(ctx, "")
	if err != nil {
		logf("load notification templates: %v", err)
	} else {
		for _, template := range templates {
			if template.Key != key {
				continue
			}
			if !template.IsEnabled {
				return
			}
			if title := renderTemplate(template.TitleTemplate, variables); title != "" {
				notification.Title = title
			}
			if content := renderTemplate(template.ContentTemplate, variables); content != "" {
				notification.Content = content
			}
			notification.InApp = template.InApp
			notification.Mail = template.Mail
			notification.Recipients = template.Recipients
			break
		}
	}

	recipients := normalizeRecipients(notification.Recipients)
	inAppUsers, mailAddresses := s.resolveRecipients(ctx, notification, recipients)

	if notification.InApp && s.notifier != nil {
		link := notification.Link
		if strings.TrimSpace(link) == "" && notification.TicketID > 0 {
			link = fmt.Sprintf("/tickets/%d", notification.TicketID)
		}
		for _, userID := range inAppUsers {
			target := notification
			target.UserID = userID
			if err := s.notifier.NotifyUser(ctx, userID, notificationType(key), notification.Title, notification.Content, link); err != nil {
				s.logNotification(ctx, target, "in_app", err)
			} else {
				s.logNotification(ctx, target, "in_app", nil)
			}
		}
	}
	if notification.Mail && s.mailer != nil {
		if recipients == "user" {
			// 买家邮件沿用按账号查地址的路径：账号不存在或未绑邮箱时静默跳过。
			for _, userID := range inAppUsers {
				if err := s.mailer.SendToUser(ctx, userID, notification.Title, notification.Content); err != nil {
					s.logNotification(ctx, notification, "mail", err)
				} else {
					s.logNotification(ctx, notification, "mail", nil)
				}
			}
		} else {
			for _, address := range mailAddresses {
				if err := s.mailer.SendToAddress(ctx, address, notification.Title, notification.Content); err != nil {
					s.logNotification(ctx, notification, "mail", err)
				} else {
					s.logNotification(ctx, notification, "mail", nil)
				}
			}
		}
	}
}

// recipientsKind 把模板里的收件人配置分成三档：买家、员工、自定义邮箱。
func normalizeRecipients(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "user", "buyer", "customer":
		return "user"
	case "staff", "admin", "admins", "owner":
		return "staff"
	default:
		return "custom"
	}
}

// resolveRecipients 计算站内收件人与邮件地址。
//   - user：买家本人，站内按账号投递，邮件走 SendToUser 查地址。
//   - staff：管理端员工。站内按员工账号投递，邮件按员工邮箱投递。
//   - custom：逗号分隔的邮箱地址，只发邮件。
func (s *Service) resolveRecipients(ctx context.Context, notification Notification, kind string) ([]uint, []string) {
	switch kind {
	case "staff":
		var users []uint
		var addresses []string
		if s.staff != nil {
			if ids, err := s.staff.UserIDs(ctx); err != nil {
				logf("resolve staff recipients: %v", err)
			} else {
				users = ids
			}
			if mails, err := s.staff.Emails(ctx); err != nil {
				logf("resolve staff emails: %v", err)
			} else {
				addresses = mails
			}
		}
		return users, dedupeStrings(addresses)
	case "custom":
		parts := strings.Split(notification.Recipients, ",")
		addresses := make([]string, 0, len(parts))
		for _, part := range parts {
			if address := strings.TrimSpace(part); address != "" {
				addresses = append(addresses, address)
			}
		}
		return nil, dedupeStrings(addresses)
	default:
		if notification.UserID == 0 {
			return nil, nil
		}
		return []uint{notification.UserID}, nil
	}
}

// dedupeStrings 去掉重复项并保持原顺序，避免同一封邮件发两遍。
func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

// renderTemplate 支持 {{variable}} 与 {variable} 两种写法，未知变量保留空串。
func renderTemplate(template string, variables map[string]string) string {
	out := template
	for key, value := range variables {
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
		out = strings.ReplaceAll(out, "{"+key+"}", value)
	}
	return strings.TrimSpace(out)
}

// notificationType 把模板 key 映射成通知的类别，前台按类别分组。
func notificationType(key string) string {
	switch {
	case strings.HasPrefix(key, "ticket."):
		return "ticket"
	case strings.HasPrefix(key, "activity."):
		return "activity"
	case strings.HasPrefix(key, "coupon."):
		return "coupon"
	case strings.HasPrefix(key, "order."):
		return "order"
	default:
		return "system"
	}
}

// logNotification 记录一次发送尝试，含失败原因，便于管理员排查与重试。
func (s *Service) logNotification(ctx context.Context, notification Notification, channel string, sendErr error) {
	entry := &models.NotificationLog{
		TemplateKey: notification.Key,
		Channel:     channel,
		Title:       truncate(notification.Title, 200),
		Content:     truncate(notification.Content, 1000),
		Status:      "sent",
		Attempts:    1,
	}
	if notification.UserID > 0 {
		userID := notification.UserID
		entry.UserID = &userID
	}
	if notification.TicketID > 0 {
		ticketID := notification.TicketID
		entry.TicketID = &ticketID
	}
	if notification.OrderID > 0 {
		orderID := notification.OrderID
		entry.OrderID = &orderID
	}
	if sendErr != nil {
		entry.Status = "failed"
		entry.Error = truncate(sendErr.Error(), 500)
	}
	if err := s.repo.CreateNotificationLog(ctx, entry); err != nil {
		logf("notification log write failed: %v", err)
	}
}

// NotificationLogs 是后台查看发送记录用的读取接口。
func (s *Service) NotificationLogs(ctx context.Context, status string, limit, offset int) ([]domain.NotificationLog, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListNotificationLogs(ctx, truncate(status, 24), limit, offset)
}
