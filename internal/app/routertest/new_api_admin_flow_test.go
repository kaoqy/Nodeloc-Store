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
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/kaoqy/Nodeloc-Store/internal/app/container"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/system"
)

// TestNewAPIProductCreatedThroughAdminAPICanBeOrdered walks the exact path the
// live shop uses: an administrator creates the New-API product through the
// admin HTTP API (not a hand-written DB row), the storefront reads it back, and
// a buyer creates an order. It exists because a hand-seeded test can silently
// bypass the create/read contract the UI actually depends on.
func TestNewAPIProductCreatedThroughAdminAPICanBeOrdered(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
	cfg.JWT.Secret = "new-api-admin-flow-secret"

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

	admin := &models.User{Username: "owner", Role: "super_admin", IsAdmin: true, IsActive: true}
	if err := ctn.DB.Create(admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	buyer := &models.User{Username: "buyer", Role: "user", IsActive: true}
	if err := ctn.DB.Create(buyer).Error; err != nil {
		t.Fatalf("seed buyer: %v", err)
	}
	// Configure the New-API channel the way the admin settings page would, so the
	// channel is ready and checkout can succeed.
	if err := system.SaveRuntime(ctn.DB, &system.RuntimeConfig{
		NewAPI: system.NewAPIConfig{
			BaseURL:          "https://new-api.example.com",
			AdminAccessToken: "token",
			AdminUserID:      "1",
			NLToUSD:          "1",
		},
	}); err != nil {
		t.Fatalf("configure New-API: %v", err)
	}
	tokenFor := func(u *models.User) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":      strconv.FormatUint(uint64(u.ID), 10),
			"exp":      time.Now().Add(time.Hour).Unix(),
			"role":     u.Role,
			"is_admin": u.IsAdmin,
		})
		signed, err := token.SignedString([]byte(cfg.JWT.Secret))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		return signed
	}

	// 1. The administrator creates the New-API product exactly as the form does.
	createBody, _ := json.Marshal(map[string]any{
		"name":             "New-API 充值",
		"slug":             "topup-admin-flow",
		"delivery_channel": "new_api",
		"min_topup_amount": 10,
		"max_topup_amount": 100,
		"is_published":     true,
		"price":            0,
		"product_type":     "manual",
	})
	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products", bytes.NewReader(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Authorization", "Bearer "+tokenFor(admin))
	router.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("admin create answered %d, want 201: %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		Data models.Product `json:"data"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created product: %v", err)
	}
	if created.Data.DeliveryChannel != "new_api" || created.Data.MinTopupAmount != 10 || created.Data.MaxTopupAmount != 100 {
		t.Fatalf("created product lost its channel config: %+v", created.Data)
	}

	// 2. The storefront must read the same channel config back.
	readRecorder := httptest.NewRecorder()
	readRequest := httptest.NewRequest(http.MethodGet, "/api/v1/store/products/"+created.Data.Slug, nil)
	router.ServeHTTP(readRecorder, readRequest)
	if readRecorder.Code != http.StatusOK {
		t.Fatalf("storefront product answered %d: %s", readRecorder.Code, readRecorder.Body.String())
	}
	var read struct {
		Data models.Product `json:"data"`
	}
	if err := json.Unmarshal(readRecorder.Body.Bytes(), &read); err != nil {
		t.Fatalf("decode storefront product: %v", err)
	}
	if read.Data.DeliveryChannel != "new_api" || read.Data.MinTopupAmount != 10 || read.Data.MaxTopupAmount != 100 {
		t.Fatalf("storefront product hid the channel config: %+v", read.Data)
	}

	// 3. A buyer creates the order.
	orderBody, _ := json.Marshal(map[string]any{
		"product_id":  created.Data.ID,
		"slug":        created.Data.Slug,
		"quantity":    1,
		"form_values": map[string]string{"nl_amount": "50"},
	})
	orderRecorder := httptest.NewRecorder()
	orderRequest := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", bytes.NewReader(orderBody))
	orderRequest.Header.Set("Content-Type", "application/json")
	orderRequest.Header.Set("Authorization", "Bearer "+tokenFor(buyer))
	router.ServeHTTP(orderRecorder, orderRequest)
	if orderRecorder.Code != http.StatusCreated {
		t.Fatalf("buyer order answered %d, want 201: %s", orderRecorder.Code, orderRecorder.Body.String())
	}
	var ordered struct {
		Order models.Order `json:"order"`
	}
	if err := json.Unmarshal(orderRecorder.Body.Bytes(), &ordered); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if ordered.Order.TotalAmount != 50 || ordered.Order.TopupAmount != 50 {
		t.Fatalf("order = total %d, topup %d; want 50/50", ordered.Order.TotalAmount, ordered.Order.TopupAmount)
	}
}

// TestNewAPIOrderWithoutChannelConfigIsRefusedSafely proves the buyer is told
// before paying when the shop has not finished configuring New-API, rather than
// paying for an order the shop cannot fulfil.
func TestNewAPIOrderWithoutChannelConfigIsRefusedSafely(t *testing.T) {
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
	cfg.JWT.Secret = "new-api-unconfigured-secret"

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

	buyer := &models.User{Username: "buyer-unconf", Role: "user", IsActive: true}
	if err := ctn.DB.Create(buyer).Error; err != nil {
		t.Fatalf("seed buyer: %v", err)
	}
	product := &models.Product{
		Name: "New-API 未配置", Slug: "topup-unconfigured",
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
		RemoteName: "New-API 兑换码", RemoteRef: "new-api-redemption-v1", IsEnabled: true,
	}).Error; err != nil {
		t.Fatalf("seed binding: %v", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": strconv.FormatUint(uint64(buyer.ID), 10),
		"exp": time.Now().Add(time.Hour).Unix(), "role": "user",
	})
	signed, _ := token.SignedString([]byte(cfg.JWT.Secret))
	body, _ := json.Marshal(map[string]any{
		"product_id": product.ID, "slug": product.Slug, "quantity": 1,
		"form_values": map[string]string{"nl_amount": "50"},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+signed)
	router.ServeHTTP(recorder, request)

	var payload map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &payload)
	if recorder.Code != http.StatusServiceUnavailable || payload["code"] != "channel_not_ready" {
		t.Fatalf("unconfigured channel answered %d %v, want 503 channel_not_ready", recorder.Code, payload)
	}
	// The buyer must never see which credential is missing.
	if text, _ := payload["error"].(string); strings.Contains(text, "AccessToken") || strings.Contains(text, "token") {
		t.Fatalf("buyer-facing error leaked credential detail: %q", text)
	}
}
