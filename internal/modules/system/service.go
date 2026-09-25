package system

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kaoqy/Nodeloc-Store/internal/authz"
	"github.com/kaoqy/Nodeloc-Store/internal/config"
	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"github.com/kaoqy/Nodeloc-Store/internal/platform/database/gormdb"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Version is reported by the system status endpoint.
const Version = "1.0.0"

// Service owns the install/status/settings lifecycle. In bootstrap mode db is
// nil and only Install/Status are usable; after the container is built, Attach
// hands it the live database and connectivity probes.
type Service struct {
	mu           sync.Mutex
	db           *gorm.DB
	installing   bool
	bootPath     string
	defaultPort  int
	oauthProbe   func() (string, error)
	paymentProbe func(context.Context) error
	rebuild      func() error
	handler      *Handler
}

func NewService(bootPath string, defaultPort int) *Service {
	if defaultPort <= 0 {
		defaultPort = 8080
	}
	s := &Service{bootPath: bootPath, defaultPort: defaultPort}
	s.handler = newHandler(s)
	return s
}

// Handler returns the HTTP handler bound to this service.
func (s *Service) Handler() *Handler {
	return s.handler
}

// Attach binds the live container to the service (called on every rebuild).
func (s *Service) Attach(db *gorm.DB, oauthProbe func() (string, error), paymentProbe func(context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
	s.oauthProbe = oauthProbe
	s.paymentProbe = paymentProbe
}

// SetRebuild installs the callback that rebuilds the running application.
func (s *Service) SetRebuild(fn func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rebuild = fn
}

// Initialized reports whether setup has completed, even if the live
// container has not been attached yet.
func (s *Service) Initialized() bool {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db != nil {
		return true
	}
	_, found, err := config.ReadBootstrap(s.bootPath)
	return err == nil && found
}

// Status is the payload for GET /api/v1/system/status.
func (s *Service) Status() map[string]any {
	initialized := s.Initialized()
	status := map[string]any{
		"initialized": initialized,
		"version":     Version,
	}
	if initialized {
		name := "Nodeloc Store"
		if rt, err := s.currentRuntime(); err == nil && rt != nil && rt.App.Name != "" {
			name = rt.App.Name
		}
		status["app"] = map[string]any{"name": name}
	}
	return status
}

// InstallRequest is the JSON body posted by the setup wizard.
type InstallRequest struct {
	App      AppConfig      `json:"app"`
	Database DatabaseInput  `json:"database"`
	OAuth    OAuthConfig    `json:"oauth"`
	Payment  PaymentConfig  `json:"payment"`
	Admin    AdminInput     `json:"admin"`
	Features FeaturesConfig `json:"features"`
	Theme    ThemeConfig    `json:"theme"`
}

type DatabaseInput struct {
	Driver string `json:"driver"`
	DSN    string `json:"dsn"`
}

type AdminInput struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

var (
	ErrAlreadyInstalled      = errors.New("系统已经初始化完成")
	ErrNotInstalled          = errors.New("系统尚未初始化")
	ErrInvalidInput          = errors.New("输入不合法")
	ErrSavedButRestartFailed = errors.New("配置已保存，但热重启失败")
)

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// Install runs the one-time setup: connect & migrate the database, create the
// admin account, persist runtime settings and the auto-managed bootstrap file,
// then swap the live application in.
func (s *Service) Install(req InstallRequest) error {
	if _, found, err := config.ReadBootstrap(s.bootPath); err == nil && found {
		return ErrAlreadyInstalled
	}
	if s.Initialized() {
		return ErrAlreadyInstalled
	}
	if !s.claimInstalling() {
		return ErrAlreadyInstalled
	}
	defer s.releaseInstalling()

	db, err := s.runInstall(req)
	if err != nil {
		return err
	}

	// Rebuild re-enters Attach on this same service, so run it after releasing
	// the claim; the probe connection is only kept when no rebuild is wired.
	rebuild := s.rebuildFn()
	if rebuild != nil {
		if err := rebuild(); err != nil {
			closeDB(db)
			return fmt.Errorf("初始化完成但应用启动失败: %w", err)
		}
		closeDB(db)
		return nil
	}
	s.Attach(db, nil, nil)
	return nil
}

// runInstall performs the setup steps and returns the live database handle.
func (s *Service) runInstall(req InstallRequest) (*gorm.DB, error) {
	req.App.Name = strings.TrimSpace(req.App.Name)
	req.App.Domain = normalizeDomain(req.App.Domain)
	req.OAuth.ClientID = strings.TrimSpace(req.OAuth.ClientID)
	req.OAuth.ClientSecret = strings.TrimSpace(req.OAuth.ClientSecret)
	req.Payment.PaymentID = strings.TrimSpace(req.Payment.PaymentID)
	req.Payment.SecretKey = strings.TrimSpace(req.Payment.SecretKey)
	req.Admin.Username = strings.TrimSpace(req.Admin.Username)
	req.Admin.Email = strings.ToLower(strings.TrimSpace(req.Admin.Email))
	if req.App.Scheme != "http" && req.App.Scheme != "https" {
		req.App.Scheme = "https"
	}

	switch {
	case req.App.Domain == "":
		return nil, validationError("站点域名不能为空")
	case req.Admin.Username == "" || len(req.Admin.Username) > 64:
		return nil, validationError("管理员用户名不合法")
	case len(req.Admin.Password) < 8:
		return nil, validationError("管理员密码至少需要 8 位")
	case req.OAuth.ClientID == "" || req.OAuth.ClientSecret == "":
		return nil, validationError("请填写 NodeLoc OAuth 的 Client ID 与 Client Secret")
	}

	driver := strings.ToLower(strings.TrimSpace(req.Database.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	dsn := strings.TrimSpace(req.Database.DSN)
	dataDir := filepath.Dir(s.bootPath)
	if driver == "sqlite" {
		if dsn == "" {
			dsn = filepath.Join(dataDir, "store.db")
		}
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
	} else if driver != "mysql" {
		return nil, validationError("不支持的数据库驱动: %s", driver)
	} else if dsn == "" {
		return nil, validationError("MySQL DSN 不能为空")
	}

	rt := &RuntimeConfig{
		App:      req.App,
		OAuth:    req.OAuth,
		Payment:  req.Payment,
		Features: req.Features,
		Theme:    req.Theme,
	}
	rt.OAuth.Enabled = true
	rt.Payment.Enabled = true
	rt.MergeDefaults()
	rt.OAuth.RedirectURI = strings.TrimSpace(rt.OAuth.RedirectURI)
	if rt.OAuth.RedirectURI == "" {
		rt.OAuth.RedirectURI = rt.RedirectURI()
	}

	db, err := gormdb.New(&config.DatabaseConfig{Driver: driver, DSN: dsn})
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}
	if err := models.Migrate(db); err != nil {
		closeDB(db)
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	if err := authz.Init(db); err != nil {
		closeDB(db)
		return nil, fmt.Errorf("权限初始化失败: %w", err)
	}
	if err := authz.SeedDefaults(); err != nil {
		log.Printf("[install] RBAC seed failed: %v", err)
	}

	var existing int64
	if err := db.Model(&models.User{}).Where("username = ?", req.Admin.Username).Count(&existing).Error; err != nil {
		closeDB(db)
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		closeDB(db)
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}
	hashString := string(hash)
	if existing > 0 {
		res := db.Model(&models.User{}).Where("username = ?", req.Admin.Username).
			Updates(map[string]any{"password_hash": &hashString, "is_admin": true, "role": "admin", "is_active": true})
		if res.Error != nil {
			closeDB(db)
			return nil, res.Error
		}
	} else {
		user := models.User{
			Username:     req.Admin.Username,
			PasswordHash: &hashString,
			IsAdmin:      true,
			IsActive:     true,
			Role:         "admin",
		}
		if req.Admin.Email != "" {
			user.Email = &req.Admin.Email
		}
		if err := db.Create(&user).Error; err != nil {
			closeDB(db)
			return nil, fmt.Errorf("创建管理员失败: %w", err)
		}
	}

	if err := SaveRuntime(db, rt); err != nil {
		closeDB(db)
		return nil, fmt.Errorf("保存配置失败: %w", err)
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		closeDB(db)
		return nil, fmt.Errorf("生成 JWT 密钥失败: %w", err)
	}
	boot := config.Bootstrap{
		DBDriver:  driver,
		DBDSN:     dsn,
		Port:      s.defaultPort,
		JWTSecret: hex.EncodeToString(secret),
	}
	if err := boot.WriteTo(s.bootPath); err != nil {
		closeDB(db)
		return nil, fmt.Errorf("写入启动配置失败: %w", err)
	}
	return db, nil
}

func closeDB(db *gorm.DB) {
	if db == nil {
		return
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func (s *Service) claimInstalling() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installing {
		return false
	}
	s.installing = true
	return true
}

func (s *Service) releaseInstalling() {
	s.mu.Lock()
	s.installing = false
	s.mu.Unlock()
}

func (s *Service) rebuildFn() func() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rebuild
}

// currentRuntime loads the stored config (caller holds no lock; db read is safe).
func (s *Service) currentRuntime() (*RuntimeConfig, error) {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil, ErrNotInstalled
	}
	rt, err := LoadRuntime(db)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		rt = Default()
	}
	return rt, nil
}

// GetSettings returns the runtime config with secrets redacted.
func (s *Service) GetSettings() (map[string]any, error) {
	rt, err := s.currentRuntime()
	if err != nil {
		return nil, err
	}
	view := *rt
	view.OAuth.ClientSecret = maskSecret(view.OAuth.ClientSecret)
	view.Payment.SecretKey = maskSecret(view.Payment.SecretKey)
	return map[string]any{"settings": view}, nil
}

// SaveSettings applies an update and rebuilds the application so the change
// takes effect immediately.
func (s *Service) SaveSettings(update RuntimeConfig) error {
	s.mu.Lock()
	db := s.db
	rebuild := s.rebuild
	s.mu.Unlock()
	if db == nil {
		return ErrNotInstalled
	}

	existing, err := LoadRuntime(db)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = Default()
	}

	next := update
	next.App.Name = strings.TrimSpace(next.App.Name)
	next.App.Domain = normalizeDomain(next.App.Domain)
	next.App.Scheme = strings.TrimSpace(next.App.Scheme)
	next.OAuth.ClientID = strings.TrimSpace(next.OAuth.ClientID)
	next.OAuth.BaseURL = strings.TrimSpace(next.OAuth.BaseURL)
	next.OAuth.RedirectURI = strings.TrimSpace(next.OAuth.RedirectURI)
	next.OAuth.Scopes = strings.TrimSpace(next.OAuth.Scopes)
	next.Payment.PaymentID = strings.TrimSpace(next.Payment.PaymentID)

	if next.OAuth.ClientSecret == "" || next.OAuth.ClientSecret == Redacted {
		next.OAuth.ClientSecret = existing.OAuth.ClientSecret
	}
	if next.Payment.SecretKey == "" || next.Payment.SecretKey == Redacted {
		next.Payment.SecretKey = existing.Payment.SecretKey
	}
	next.MergeDefaults()
	if next.App.Domain == "" {
		next.App.Domain = existing.App.Domain
	}
	if next.OAuth.RedirectURI == "" {
		next.OAuth.RedirectURI = existing.OAuth.RedirectURI
	}
	if next.OAuth.ClientID == "" || next.OAuth.ClientSecret == "" {
		return validationError("NodeLoc OAuth Client ID / Secret 不能为空")
	}

	if err := SaveRuntime(db, &next); err != nil {
		return err
	}
	if rebuild != nil {
		if err := rebuild(); err != nil {
			return fmt.Errorf("%w: %v", ErrSavedButRestartFailed, err)
		}
	}
	return nil
}

// TestOAuth builds an authorization URL from the current settings.
func (s *Service) TestOAuth() (string, error) {
	s.mu.Lock()
	probe := s.oauthProbe
	s.mu.Unlock()
	if probe == nil {
		return "", ErrNotInstalled
	}
	return probe()
}

// TestPayment probes the NodeLoc payment gateway with the current settings.
func (s *Service) TestPayment(ctx context.Context) (bool, string) {
	s.mu.Lock()
	probe := s.paymentProbe
	s.mu.Unlock()
	if probe == nil {
		return false, "系统尚未初始化"
	}
	err := probe(ctx)
	if err == nil {
		return true, "支付网关连通正常"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not fully configured"):
		return false, "支付参数未配置完整（Payment ID / Secret Key）"
	case strings.Contains(msg, "NodeLoc returned HTTP"):
		return true, "支付网关可达（NodeLoc 返回: " + msg + "）"
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "connection refused"), strings.Contains(msg, "timeout"), strings.Contains(msg, "TLS"):
		return false, "无法连接支付网关: " + msg
	default:
		return false, msg
	}
}

func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	return Redacted
}

// normalizeDomain strips scheme and trailing slash from a host setting.
func normalizeDomain(in string) string {
	in = strings.TrimSpace(in)
	in = strings.TrimPrefix(in, "https://")
	in = strings.TrimPrefix(in, "http://")
	in = strings.TrimRight(in, "/")
	return strings.ToLower(in)
}
