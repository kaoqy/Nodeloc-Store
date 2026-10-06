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
	"github.com/kaoqy/Nodeloc-Store/internal/modules/catalog/domain"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/system"
)

// TestNewAPIOrderRouteAcceptsTheStorefrontPayload is end-to-end, network-level
// evidence for the storefront's create-order call: it signs a real JWT, POSTs
// the exact payload CreateOrderPayload builds (numeric product_id and
// form_values.nl_amount) to POST /api/v1/payment/orders through the registered
// router, and reads the created order back from the database.
func TestNewAPIOrderRouteAcceptsTheStorefrontPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
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
	cfg.JWT.Secret = "new-api-contract-test-secret"

	ctn, err := container.New(cfg, nil)
	if err != nil {
		t.Fatalf("container.New: %v", err)
	}
	defer func() {
		if sqlDB, err := ctn.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	router := gin.New()
	accounts := ctn.Identity.Handler.AccountReader()
	ctn.Identity.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Payment.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Catalog.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Plugin.Handler.RegisterRoutes(router, &cfg.JWT, accounts)

	// Seed an active buyer and a published New-API product, so a correctly wired
	// request must create a real pending order carrying the buyer's amount and
	// the product's own id.
	buyer := &models.User{Username: "buyer", Role: "user", IsActive: true}
	if err := ctn.DB.Create(buyer).Error; err != nil {
		t.Fatalf("seed buyer: %v", err)
	}
	product := &models.Product{
		Name:            "New-API 充值",
		Slug:            "topup-contract",
		Price:           5,
		ProductType:     domain.ProductTypeManual,
		DeliveryChannel: domain.DeliveryChannelNewAPI,
		MinTopupAmount:  10,
		MaxTopupAmount:  100,
		IsPublished:     true,
	}
	if err := ctn.DB.Create(product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	// A product bound to the New-API provider needs an enabled binding; create
	// it directly so the checkout path is exercised without the plugin UI.
	var provider models.Plugin
	if err := ctn.DB.Where("key = ?", "new-api-redemption-v1").First(&provider).Error; err != nil {
		t.Fatalf("load builtin provider: %v", err)
	}
	binding := &models.PluginBinding{
		PluginID:   provider.ID,
		ProductID:  product.ID,
		Value:      "",
		RemoteName: "New-API 兑换码",
		RemoteRef:  "new-api-redemption-v1",
		IsEnabled:  true,
	}
	if err := ctn.DB.Create(binding).Error; err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	if err := system.SaveRuntime(ctn.DB, &system.RuntimeConfig{
		NewAPI: system.NewAPIConfig{
			BaseURL: "https://new-api.example.com", AdminAccessToken: "token",
			AdminUserID: "1", NLToUSD: "1",
		},
	}); err != nil {
		t.Fatalf("configure New-API: %v", err)
	}

	// The storefront's amount input comes from this public descriptor; it must
	// expose exactly one field (nl_amount) and no account/username field.
	describeRecorder := httptest.NewRecorder()
	describeRequest := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+strconv.FormatUint(uint64(product.ID), 10)+"/plugin", nil)
	router.ServeHTTP(describeRecorder, describeRequest)
	if describeRecorder.Code != http.StatusOK {
		t.Fatalf("plugin descriptor answered %d: %s", describeRecorder.Code, describeRecorder.Body.String())
	}
	var descriptor struct {
		Data struct {
			PluginKey  string `json:"plugin_key"`
			FormSchema []struct {
				Key      string `json:"key"`
				Required bool   `json:"required"`
			} `json:"form_schema"`
		} `json:"data"`
	}
	if err := json.Unmarshal(describeRecorder.Body.Bytes(), &descriptor); err != nil {
		t.Fatalf("decode descriptor: %v", err)
	}
	if descriptor.Data.PluginKey != "new-api-redemption-v1" {
		t.Fatalf("descriptor plugin = %q, want new-api-redemption-v1", descriptor.Data.PluginKey)
	}
	if len(descriptor.Data.FormSchema) != 1 || descriptor.Data.FormSchema[0].Key != "nl_amount" {
		t.Fatalf("descriptor form schema = %+v, want only nl_amount", descriptor.Data.FormSchema)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  strconv.FormatUint(uint64(buyer.ID), 10),
		"exp":  time.Now().Add(time.Hour).Unix(),
		"role": "user",
	})
	signed, err := token.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"product_id": product.ID,
		"slug":       product.Slug,
		"quantity":   1,
		"form_values": map[string]string{
			"nl_amount": "50",
		},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+signed)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/payment/orders answered %d, want 201: %s", recorder.Code, recorder.Body.String())
	}

	var created struct {
		Order models.Order `json:"order"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if created.Order.ProductID != product.ID {
		t.Fatalf("order product = %d, want %d", created.Order.ProductID, product.ID)
	}
	if created.Order.TopupAmount != 50 {
		t.Fatalf("order topup = %d, want 50", created.Order.TopupAmount)
	}
	if created.Order.Status != "pending" {
		t.Fatalf("order status = %q, want pending", created.Order.Status)
	}

	var stored models.Order
	if err := ctn.DB.Where("order_no = ?", created.Order.OrderNo).First(&stored).Error; err != nil {
		t.Fatalf("reload persisted order: %v", err)
	}
	if stored.ProductID != product.ID || stored.TopupAmount != 50 {
		t.Fatalf("persisted order = %+v", stored)
	}

	// The server, not the client, owns the amount range: an out-of-range amount
	// over the very same route must be refused, and no second order created.
	for _, amount := range []string{"9", "101", "abc", "0"} {
		badBody, _ := json.Marshal(map[string]any{
			"product_id":  product.ID,
			"slug":        product.Slug,
			"quantity":    1,
			"form_values": map[string]string{"nl_amount": amount},
		})
		badRecorder := httptest.NewRecorder()
		badRequest := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", bytes.NewReader(badBody))
		badRequest.Header.Set("Content-Type", "application/json")
		badRequest.Header.Set("Authorization", "Bearer "+signed)
		router.ServeHTTP(badRecorder, badRequest)
		if badRecorder.Code != http.StatusBadRequest {
			t.Fatalf("amount %q answered %d, want 400: %s", amount, badRecorder.Code, badRecorder.Body.String())
		}
	}
	var orderCount int64
	if err := ctn.DB.Model(&models.Order{}).Where("product_id = ?", product.ID).Count(&orderCount).Error; err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("orders = %d, want exactly the one valid order", orderCount)
	}
}

// TestNewAPIOrderWithoutProductPriceSucceeds proves the amount-type rule end to
// end: a New-API product has no product price, and the order must still be
// created, charged at exactly the NL amount the buyer entered.
func TestNewAPIOrderWithoutProductPriceSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// The payment layer logs the technical reason for a 5xx; keep the test tidy.
	previousWriter := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
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
	cfg.JWT.Secret = "new-api-pricing-repro-secret"

	ctn, err := container.New(cfg, nil)
	if err != nil {
		t.Fatalf("container.New: %v", err)
	}
	defer func() {
		if sqlDB, err := ctn.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	router := gin.New()
	accounts := ctn.Identity.Handler.AccountReader()
	ctn.Identity.Handler.RegisterRoutes(router, &cfg.JWT)
	ctn.Payment.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Catalog.Handler.RegisterRoutes(router, &cfg.JWT, accounts)
	ctn.Plugin.Handler.RegisterRoutes(router, &cfg.JWT, accounts)

	buyer := &models.User{Username: "pricing-buyer", Role: "user", IsActive: true}
	if err := ctn.DB.Create(buyer).Error; err != nil {
		t.Fatalf("seed buyer: %v", err)
	}
	// Exactly the channel's allowed configuration: two amount bounds, no price.
	product := &models.Product{
		Name:            "New-API 无价充值",
		Slug:            "topup-no-price",
		Price:           0,
		ProductType:     domain.ProductTypeManual,
		DeliveryChannel: domain.DeliveryChannelNewAPI,
		MinTopupAmount:  10,
		MaxTopupAmount:  100,
		IsPublished:     true,
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
		RemoteName: "New-API 兑换码", RemoteRef: "new-api-redemption-v1", IsEnabled: true,
	}).Error; err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	if err := system.SaveRuntime(ctn.DB, &system.RuntimeConfig{
		NewAPI: system.NewAPIConfig{
			BaseURL: "https://new-api.example.com", AdminAccessToken: "token",
			AdminUserID: "1", NLToUSD: "1",
		},
	}); err != nil {
		t.Fatalf("configure New-API: %v", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  strconv.FormatUint(uint64(buyer.ID), 10),
		"exp":  time.Now().Add(time.Hour).Unix(),
		"role": "user",
	})
	signed, err := token.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"product_id":  product.ID,
		"slug":        product.Slug,
		"quantity":    1,
		"form_values": map[string]string{"nl_amount": "50"},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+signed)
	router.ServeHTTP(recorder, request)

	// A New-API product with no product price must order successfully: the buyer
	// pays exactly the NL amount they entered.
	if recorder.Code != http.StatusCreated {
		t.Fatalf("price-less New-API order answered %d, want 201: %s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		Order models.Order `json:"order"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if created.Order.TotalAmount != 50 || created.Order.UnitPrice != 50 {
		t.Fatalf("order priced %d x %d, want 50 x 1", created.Order.UnitPrice, created.Order.Quantity)
	}
	if created.Order.TopupAmount != 50 {
		t.Fatalf("order topup = %d, want 50", created.Order.TopupAmount)
	}
}
