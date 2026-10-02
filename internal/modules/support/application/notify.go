package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// Notification 是渲染好的一条通知，模板开关与渠道开关都已经判断完毕。
type Notification struct {
	Key      string
	Title    string
	Content  string
	Link     string
	InApp    bool
	Mail     bool
	UserID   uint
	TicketID uint
	OrderID  uint
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
			break
		}
	}

	if notification.InApp && s.notifier != nil && notification.UserID > 0 {
		link := notification.Link
		if strings.TrimSpace(link) == "" && notification.TicketID > 0 {
			link = fmt.Sprintf("/tickets/%d", notification.TicketID)
		}
		if err := s.notifier.NotifyUser(ctx, notification.UserID, notificationType(key), notification.Title, notification.Content, link); err != nil {
			s.logNotification(ctx, notification, "in_app", err)
		} else {
			s.logNotification(ctx, notification, "in_app", nil)
		}
	}
	if notification.Mail && s.mailer != nil && notification.UserID > 0 {
		if err := s.mailer.SendToUser(ctx, notification.UserID, notification.Title, notification.Content); err != nil {
			s.logNotification(ctx, notification, "mail", err)
		} else {
			s.logNotification(ctx, notification, "mail", nil)
		}
	}
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
