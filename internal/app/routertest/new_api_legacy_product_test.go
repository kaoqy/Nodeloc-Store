package routertest

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/kaoqy/Nodeloc-Store/internal/app/container"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/system"
)

func setupNewAPIShop(t *testing.T, configured bool) (*gin.Engine, *container.Container, string, *models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousWriter := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	cfg := &config.Config{}
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(dir, "store.db")
	cfg.JWT.Secret = "new-api-legacy-secret"

	ctn, err := container.New(cfg, nil)
	if err != nil {
		t.Fatalf("container.New: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := ctn.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	router := gin.New()
	accounts := ctn.Identity.Handler.AccountReader()
	ctn.Identity.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Payment.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Catalog.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Plugin.Handler.RegisterRoutes(router, &cfg.JWT, accounts)

	if configured {
		if err := system.SaveRuntime(ctn.DB, &system.RuntimeConfig{
			NewAPI: system.NewAPIConfig{
				BaseURL: "https://new-api.example.com", AdminAccessToken: "token",
				AdminUserID: "1", NLToUSD: "1",
			},
		}); err != nil {
			t.Fatalf("configure: %v", err)
		}
	}
	buyer := &models.User{Username: "legacy-buyer", Role: "user", IsActive: true}
	if err := ctn.DB.Create(buyer).Error; err != nil {
		t.Fatalf("seed buyer: %v", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": strconv.FormatUint(uint64(buyer.ID), 10),
		"exp": time.Now().Add(time.Hour).Unix(), "role": "user",
	})
	signed, _ := token.SignedString([]byte(cfg.JWT.Secret))
	return router, ctn, signed, buyer
}

// A product can exist as a New-API product without its plugin binding: older
// rows, or a row whose binding sync failed. The storefront then gets no plugin
// descriptor, so the amount input never renders and the buyer cannot order.
// Checkout must still work from the product's own channel config.
func TestNewAPIProductWithoutBindingCanStillBeOrdered(t *testing.T) {
	router, ctn, signed, _ := setupNewAPIShop(t, true)

	// Deliberately create the product with delivery_channel=new_api but WITHOUT
	// the plugin binding the storefront descriptor depends on.
	product := &models.Product{
		Name: "New-API 无绑定", Slug: "topup-no-binding",
		ProductType: "manual", DeliveryChannel: "new_api",
		MinTopupAmount: 10, MaxTopupAmount: 100, IsPublished: true,
	}
	if err := ctn.DB.Create(product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}

	// Even without a stored binding, the storefront descriptor must be repaired
	// on read: it gains the amount field and the rate, so the product page can
	// render the single required input.
	descRecorder := httptest.NewRecorder()
	descRequest := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+strconv.FormatUint(uint64(product.ID), 10)+"/plugin", nil)
	router.ServeHTTP(descRecorder, descRequest)
	var desc struct {
		Data *struct {
			PluginKey  string `json:"plugin_key"`
			FormSchema []struct {
				Key string `json:"key"`
			} `json:"form_schema"`
		} `json:"data"`
	}
	_ = json.Unmarshal(descRecorder.Body.Bytes(), &desc)
	if desc.Data == nil || desc.Data.PluginKey != "new-api-redemption-v1" {
		t.Fatalf("binding-less product did not get a repaired descriptor: %s", descRecorder.Body.String())
	}
	if len(desc.Data.FormSchema) != 1 || desc.Data.FormSchema[0].Key != "nl_amount" {
		t.Fatalf("repaired descriptor lacks the amount field: %s", descRecorder.Body.String())
	}

	body, _ := json.Marshal(map[string]any{
		"product_id": product.ID, "slug": product.Slug, "quantity": 1,
		"form_values": map[string]string{"nl_amount": "50"},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+signed)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("binding-less New-API order answered %d, want 201: %s", recorder.Code, recorder.Body.String())
	}
}

// A disabled binding is the other half of the same failure: the product is a
// New-API product but its binding was switched off. Repair must re-enable it so
// the descriptor and routing work again.
func TestNewAPIProductWithDisabledBindingIsRepaired(t *testing.T) {
	router, ctn, signed, _ := setupNewAPIShop(t, true)

	product := &models.Product{
		Name: "New-API 停用绑定", Slug: "topup-disabled-binding",
		ProductType: "manual", DeliveryChannel: "new_api",
		MinTopupAmount: 10, MaxTopupAmount: 100, IsPublished: true,
	}
	if err := ctn.DB.Create(product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	var provider models.Plugin
	if err := ctn.DB.Where("key = ?", "new-api-redemption-v1").First(&provider).Error; err != nil {
		t.Fatalf("load provider: %v", err)
	}
	if err := ctn.DB.Create(&models.PluginBinding{
		PluginID: provider.ID, ProductID: product.ID, Value: "",
		RemoteName: "New-API 兑换码", RemoteRef: "new-api-redemption-v1",
		IsEnabled: false,
	}).Error; err != nil {
		t.Fatalf("seed disabled binding: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"product_id": product.ID, "slug": product.Slug, "quantity": 1,
		"form_values": map[string]string{"nl_amount": "50"},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+signed)
	router.ServeHTTP(recorder, request)

	// The product declares the New-API channel, so the disabled binding is
	// repaired and the order succeeds — never falling through to another channel.
	if recorder.Code != http.StatusCreated {
		t.Fatalf("disabled-binding New-API order answered %d, want 201: %s", recorder.Code, recorder.Body.String())
	}
	var stored models.PluginBinding
	if err := ctn.DB.Where("product_id = ? AND remote_ref = ?", product.ID, "new-api-redemption-v1").First(&stored).Error; err != nil {
		t.Fatalf("reload binding: %v", err)
	}
	if !stored.IsEnabled {
		t.Fatal("binding was not re-enabled by the repair")
	}
}
