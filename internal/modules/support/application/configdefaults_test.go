package application

import (
	"context"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/support/domain"
)

// TestActivityDefaultsComeFromConfigCenter proves the switches a shop saves in
// 配置中心 actually reach the activity defaults, rather than sitting unused.
func TestActivityDefaultsComeFromConfigCenter(t *testing.T) {
	service := newConfigService(t, []domain.SystemConfig{
		{Base: models.Base{ID: 1}, Group: "activity", Key: "default_per_user_limit", Value: "3", ValueType: "int"},
		{Base: models.Base{ID: 2}, Group: "activity", Key: "block_activity_stacking", Value: "true", ValueType: "bool"},
		{Base: models.Base{ID: 3}, Group: "product", Key: "show_sold_count", Value: "false", ValueType: "bool"},
		{Base: models.Base{ID: 4}, Group: "order", Key: "unpaid_cancel_hours", Value: "6", ValueType: "int"},
	})
	ctx := context.Background()
	if got := service.SystemConfigInt(ctx, "activity", "default_per_user_limit", 1); got != 3 {
		t.Fatalf("per-user limit = %d, want 3", got)
	}
	if !service.SystemConfigBool(ctx, "activity", "block_activity_stacking", false) {
		t.Fatal("block_activity_stacking = false, want true")
	}
	if service.SystemConfigBool(ctx, "product", "show_sold_count", true) {
		t.Fatal("show_sold_count = true, want false")
	}
	if got := service.SystemConfigInt(ctx, "order", "unpaid_cancel_hours", 2); got != 6 {
		t.Fatalf("unpaid cancel hours = %d, want 6", got)
	}
}
