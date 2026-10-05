package application

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	catalogapp "github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/application"
	cataloginfra "github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/infrastructure"
	paymentcontract "github.com/kaoqy/Nodeloc-Store/internal/modules/payment/contract"
	paymentinfra "github.com/kaoqy/Nodeloc-Store/internal/modules/payment/infrastructure"
	pluginapp "github.com/kaoqy/Nodeloc-Store/internal/modules/plugin"
	pluginaapp "github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/application"
	plugininfra "github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/infrastructure"
)

type newAPIRuntimeStub struct{}

func (newAPIRuntimeStub) NewAPIConfig(context.Context) (map[string]string, map[string]string, error) {
	return map[string]string{
			"base_url":      "https://new-api.example.com",
			"admin_user_id": "1",
			"quota_per_nl":  "100",
		}, map[string]string{
			"admin_access_token": "token",
		}, nil
}

type integrationUserLookup struct{}

func (integrationUserLookup) FindByID(context.Context, uint) (*paymentcontract.UserInfo, error) {
	return &paymentcontract.UserInfo{ID: 7, Username: "buyer", IsActive: true}, nil
}

func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := models.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestNewAPIProductPipelineWithRealModules wires the real catalog, plugin and
// payment modules against SQLite. It exercises product creation, binding sync,
// the plugin form field, and checkout, so a break anywhere in that chain fails
// here rather than only in the browser.
func TestNewAPIProductPipelineWithRealModules(t *testing.T) {
	db := openIntegrationDB(t)
	runtime := newAPIRuntimeStub{}

	registry := plugininfra.NewRegistry(
		plugininfra.NewManualDelivery(),
		plugininfra.NewNewAPIRedemption(runtime),
	)
	pluginService, err := pluginaapp.NewService(plugininfra.NewGormStore(db), registry, runtime)
	if err != nil {
		t.Fatalf("plugin service: %v", err)
	}
	if err := pluginService.EnsureBuiltinProvider(context.Background(), plugininfra.NewAPIRedemptionKey); err != nil {
		t.Fatalf("ensure provider: %v", err)
	}
	bridge := pluginapp.NewDeliveryBridge(pluginService)

	catalogService := catalogapp.NewService(
		cataloginfra.NewProductRepo(db),
		cataloginfra.NewCardRepo(db),
		cataloginfra.NewCategoryRepo(db),
		cataloginfra.NewCouponRepo(db),
		config.FeaturesConfig{},
	)
	catalogService.SetDeliveryChannelSync(bridge)

	product := &models.Product{
		Name:            "New-API 充值",
		Slug:            "topup",
		Price:           5,
		DeliveryChannel: "new_api",
		MinTopupAmount:  10,
		MaxTopupAmount:  100,
		IsPublished:     true,
	}
	if err := catalogService.CreateProduct(context.Background(), product); err != nil {
		t.Fatalf("create product: %v", err)
	}

	describe, err := pluginService.DescribeProduct(context.Background(), product.ID)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if describe == nil || len(describe.FormSchema) != 1 || describe.FormSchema[0].Key != "nl_amount" {
		t.Fatalf("descriptor = %+v, want only nl_amount", describe)
	}

	paymentService := NewService(
		paymentinfra.NewGormStore(db),
		integrationGateway{},
		integrationFulfillment{},
		integrationUserLookup{},
		catalogService,
		nil,
		"pay-id",
		config.FeaturesConfig{},
	)
	paymentService.SetPluginDeliverer(bridge)

	order, err := paymentService.CreateOrder(context.Background(), CreateOrderInput{
		UserID:     7,
		ProductID:  product.ID,
		Slug:       product.Slug,
		Quantity:   1,
		FormValues: map[string]string{"nl_amount": "50"},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.TopupAmount != 50 || order.ProductID != product.ID {
		t.Fatalf("order = %+v, want product %d topup 50", order, product.ID)
	}

	// The channel must accept only the two amount bounds and must not require any
	// New-API account/username field.
	for _, amount := range []string{"9", "101", "0", "-1", "abc", "5.5"} {
		if _, err := paymentService.CreateOrder(context.Background(), CreateOrderInput{
			UserID:     7,
			ProductID:  product.ID,
			Quantity:   1,
			FormValues: map[string]string{"nl_amount": amount},
		}); err == nil {
			t.Fatalf("amount %q was accepted outside 10..100", amount)
		}
	}

	// A New-API product has no card rows; ordering must not consult card stock.
	var cardCount int64
	if err := db.Model(&models.Card{}).Where("product_id = ?", product.ID).Count(&cardCount).Error; err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if cardCount != 0 {
		t.Fatalf("New-API product unexpectedly has %d card rows", cardCount)
	}
}

type integrationGateway struct{ paymentcontract.PaymentGateway }

func (integrationGateway) VerifyCallback(map[string]string) bool { return false }

type integrationFulfillment struct{}

func (integrationFulfillment) Fulfill(context.Context, *models.Order) error { return nil }
