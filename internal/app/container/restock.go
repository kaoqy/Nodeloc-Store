package container

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/app/stockwatch"
	"github.com/kaoqy/Nodeloc-Store/internal/authz"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	notificationapp "github.com/kaoqy/Nodeloc-Store/internal/modules/notification/application"
)

// restockWarningType is the kind the back office inbox labels 库存预警.
const restockWarningType = "stock"

// restockOutbox hands a watcher's warning to the inbox it shares with buyers.
// The wording stays here rather than in the notification module because the
// shelf, not the message transport, is what decides it is worth sending.
type restockOutbox struct {
	service *notificationapp.Service
}

func (o restockOutbox) Publish(ctx context.Context, userID uint, warning stockwatch.Warning, since time.Time) (bool, error) {
	content := warning.Content
	link := warning.Link
	notification := &models.Notification{
		UserID:  userID,
		Type:    restockWarningType,
		Title:   warning.Title,
		Content: &content,
		Link:    &link,
	}
	return o.service.SendOnce(ctx, notification, since)
}

// restockRoster answers "who refills the shelves": active accounts holding a role
// that may open the cards pages. It reads the same grant the routes check, so a
// warning never lands in an inbox that cannot act on it, and a role the shop
// demotes stops being addressed on the next pass.
func restockRoster(db *gorm.DB) func(context.Context) ([]uint, error) {
	return func(ctx context.Context) ([]uint, error) {
		var holders []struct {
			ID   uint
			Role string
		}
		if err := db.WithContext(ctx).Model(&models.User{}).
			Where("is_active = ? AND role <> ? AND role <> ?", true, "", "user").
			Find(&holders).Error; err != nil {
			return nil, err
		}
		ids := make([]uint, 0, len(holders))
		for _, holder := range holders {
			if authz.Can(holder.Role, "cards", "view") {
				ids = append(ids, holder.ID)
			}
		}
		return ids, nil
	}
}
