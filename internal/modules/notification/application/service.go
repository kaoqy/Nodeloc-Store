package application

import (
	"context"
	"errors"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/notification/contract"
)

var ErrInvalidNotification = errors.New("user_id, type and title are required")

type Service struct {
	repo     contract.NotificationRepo
	notifier contract.Notifier
}

func NewService(repo contract.NotificationRepo, notifier contract.Notifier) *Service {
	return &Service{repo: repo, notifier: notifier}
}

func (s *Service) Send(ctx context.Context, notification *models.Notification) error {
	if notification == nil || notification.UserID == 0 || strings.TrimSpace(notification.Type) == "" || strings.TrimSpace(notification.Title) == "" {
		return ErrInvalidNotification
	}
	notification.Type = strings.TrimSpace(notification.Type)
	notification.Title = strings.TrimSpace(notification.Title)
	notification.IsRead = false
	if err := s.repo.Create(ctx, notification); err != nil {
		return err
	}
	if s.notifier != nil {
		return s.notifier.Notify(ctx, *notification)
	}
	return nil
}

func (s *Service) Broadcast(ctx context.Context, notificationType, title string, content, link *string) (int, error) {
	notificationType = strings.TrimSpace(notificationType)
	title = strings.TrimSpace(title)
	if notificationType == "" || title == "" {
		return 0, ErrInvalidNotification
	}
	userIDs, err := s.repo.ListActiveUserIDs(ctx)
	if err != nil {
		return 0, err
	}
	notifications := make([]*models.Notification, 0, len(userIDs))
	for _, userID := range userIDs {
		notifications = append(notifications, &models.Notification{UserID: userID, Type: notificationType, Title: title, Content: content, Link: link})
	}
	if err := s.repo.CreateBatch(ctx, notifications); err != nil {
		return 0, err
	}
	if s.notifier != nil {
		for _, notification := range notifications {
			if err := s.notifier.Notify(ctx, *notification); err != nil {
				return 0, err
			}
		}
	}
	return len(notifications), nil
}

func (s *Service) List(ctx context.Context, userID uint, page, pageSize int) ([]models.Notification, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.ListByUser(ctx, userID, pageSize, (page-1)*pageSize)
}

func (s *Service) MarkAsRead(ctx context.Context, id, userID uint) error {
	if id == 0 || userID == 0 {
		return ErrInvalidNotification
	}
	return s.repo.MarkAsRead(ctx, id, userID)
}
