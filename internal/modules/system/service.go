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
	"github.com/kaoqy/Nodeloc-Store/internal/shared"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Version is reported by the system status endpoint.
const Version = "1.0.0"

// superAdminRole is the role the wizard gives the account it creates. It is the
// only role that can hand out back-office roles, so the owner has to hold it.
const superAdminRole = "super_admin"

// Service owns the install/status/settings lifecycle. In bootstrap mode db is
// nil and only Install/Status are usable; after the container is built, Attach
// hands it the live database and connectivity probes.
type Service struct {
	mu           sync.Mutex
	db           *gorm.DB
	installing   bool
	bootPath     string
	defaultPort  int
	oauthProbe   OAuthProbeFunc
	paymentProbe PaymentProbeFunc
	rebuild      func() error
	handler      *Handler
}

// OAuthProbe is the identity module's reading of one call to NodeLoc's token
// endpoint. Code is empty when NodeLoc recognised the Client ID / Client Secret
// pair; AuthorizeURL carries the link the settings page can offer either way.
//
// The code, not a sentence, is what 设置 switches on: an English OAuth error name
// is the only part of that answer that does not follow the forum's locale, and a
// settings page re-reading error strings is how a wrong Secret used to be reported
// as 「配置正确」.
type OAuthProbe struct {
	Code         string
	Detail       string
	AuthorizeURL string
}

// OAuthProbeFunc asks the live OAuth provider whether this store's credentials are
// a pair NodeLoc knows.
type OAuthProbeFunc func(context.Context) OAuthProbe

// PaymentProbe is what the payment module reports back from one test call to
// NodeLoc. Code is empty when the provider accepted the call; otherwise it is the
// payment module's own machine-readable reading of the refusal — 设置 switches on
// it instead of re-reading English error strings, so the two modules stay apart
// and can never disagree about what the same refusal means.
type PaymentProbe struct {
	Code      string
	Message   string
	Detail    string
	Retryable bool
	Style     string
}

// PaymentProbeFunc asks the live payment gateway whether this store can reach
// NodeLoc, without touching an order.
type PaymentProbeFunc func(context.Context) PaymentProbe

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
// Both probes answer with the module's own reading of one NodeLoc call: the payment
// probe also carries the signing convention the forum accepted, and the OAuth probe
// carries whether the client pair is known, so 「测试」 buttons can name the failure
// and the field to fix instead of quoting an English error back to the shop owner.
func (s *Service) Attach(db *gorm.DB, oauthProbe OAuthProbeFunc, paymentProbe PaymentProbeFunc) {
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
		app := map[string]any{"name": "Nodeloc Store"}
		if rt, err := s.currentRuntime(); err == nil && rt != nil {
			if rt.App.Name != "" {
				app["name"] = rt.App.Name
			}
			app["slogan"] = rt.App.Slogan
			app["description"] = rt.App.Description
			app["logo"] = rt.App.Logo
			// The storefront renders whatever the shop owner typed into 设置, so
			// the footer and the announcement travel with the site identity.
			app["footer_text"] = rt.App.FooterText
			app["footer_note"] = rt.App.FooterNote
			app["footer_links"] = rt.App.FooterLinks
			app["announcement"] = rt.App.Announcement
			status["features"] = map[string]any{
				"registration": rt.Features.RegistrationEnabled,
				"checkin":      rt.Features.CheckinEnabled(),
				"coupons":      rt.Features.CouponsEnabled(),
			}
			// Both SPAs recolour from this: style.css derives every accent token
			// from --brand, so one hex repaints buttons, focus rings and glow.
			// The locale only labels the storefront, whose own copy stays
			// Simplified in the back office.
			status["theme"] = map[string]any{"primary": rt.Theme.Primary, "locale": rt.Theme.Locale}
		}
		status["app"] = app
	}
	return status
}

// ShellIdentity is the copy an HTML document needs before its JavaScript boots:
// what the tab is called, what a search result or a forum preview says about the
// shop, and which icon the tab wears.
type ShellIdentity struct {
	Name        string
	Description string
	Logo        string
	Locale      string
}

// GetShellIdentity reads the same settings document the status API answers from,
// so the served shell and the app that repaints it cannot disagree about what
// the shop is called. Until setup finishes there is nothing stored, and the
// built defaults are returned — the same words the bundle ships with.
func (s *Service) GetShellIdentity() ShellIdentity {
	d := Default()
	shell := ShellIdentity{Name: d.App.Name, Locale: d.Theme.Locale}
	rt, err := s.currentRuntime()
	if err != nil || rt == nil {
		return shell
	}
	if rt.App.Name != "" {
		shell.Name = rt.App.Name
	}
	shell.Description = rt.App.Description
	shell.Logo = rt.App.Logo
	if rt.Theme.Locale != "" {
		shell.Locale = rt.Theme.Locale
	}
	return shell
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
	req.Payment.Token = strings.TrimSpace(req.Payment.Token)
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
	// The wizard's payment block is complete when the application is named and the
	// owner has pasted its secret — whichever of the two boxes NodeLoc handed them.
	// Requiring both made a shop that could take money switch itself off at install.
	rt.Payment.Enabled = strings.TrimSpace(req.Payment.PaymentID) != "" &&
		(strings.TrimSpace(req.Payment.Token) != "" || strings.TrimSpace(req.Payment.SecretKey) != "")
	rt.MergeDefaults()
	// The wizard's fields are as user-typed as the settings page's, so they go
	// through the same bounds and colour checks before they are stored.
	rt.Normalize()
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
			Updates(map[string]any{"password_hash": &hashString, "is_admin": true, "role": superAdminRole, "is_active": true})
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
			Role:         superAdminRole,
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

// EnsureOwnerRole leaves the store with exactly one super_admin.
//
// The wizard used to hand the owner 管理员, and this application deliberately
// lets only a super_admin grant 管理员 or move a staff account's role: an
// install from before the change had no account that could ever add a second
// admin. Promoting the oldest 管理员 closes that hole; as long as some
// super_admin is still around the check only reads the count and leaves every
// role alone, so it never undoes a deliberate change while the shop has an owner.
func EnsureOwnerRole(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	var owners int64
	if err := db.Model(&models.User{}).
		Where("role = ?", superAdminRole).
		Count(&owners).Error; err != nil {
		return err
	}
	if owners > 0 {
		return nil
	}

	var candidate models.User
	err := db.Where("(is_admin = ? OR role = ?) AND role <> ? AND is_active = ?",
		true, "admin", superAdminRole, true).
		Order("id ASC").
		First(&candidate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Nothing to promote: either the shop has no 管理员 account left, or the
		// owner deactivated it. Both are states someone has to fix by hand, and
		// silently elevating a 客服 to owner would be the worse answer.
		return nil
	}
	if err != nil {
		return err
	}
	if err := db.Model(&candidate).
		Updates(map[string]any{"role": superAdminRole, "is_admin": true}).Error; err != nil {
		return err
	}
	log.Printf("[authz] no super_admin in store; promoted user %d (%s) to owner", candidate.ID, candidate.Username)
	return nil
}

// RoleHeadcounts is how many accounts sit in each role. The role editor shows
// it before someone saves a matrix: 客服 has 4 accounts, so switching off
// orders:view is a change that reaches four people on their next click.
func (s *Service) RoleHeadcounts() (map[string]int64, error) {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil, nil
	}
	var rows []struct {
		Role  string
		Total int64
	}
	if err := db.Model(&models.User{}).
		Select("role, COUNT(*) AS total").
		Group("role").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, item := range rows {
		role := strings.TrimSpace(item.Role)
		if role == "" {
			// Accounts seeded before roles existed are plain buyers.
			role = "user"
		}
		counts[role] += item.Total
	}
	return counts, nil
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
	view.Payment.Token = maskSecret(view.Payment.Token)
	view.Payment.SecretKey = maskSecret(view.Payment.SecretKey)
	// The owner's switch says what they want; this says whether the shop can act
	// on it. Without it a storefront with an empty Payment Token advertises
	// 「支付已启用」 while every buyer is refused at 下单.
	missing := rt.Payment.MissingCredentials()
	return map[string]any{
		"settings":         view,
		"payment_ready":    rt.Payment.Enabled && len(missing) == 0,
		"payment_missing":  missing,
		"payment_warnings": rt.PaymentWarnings(),
	}, nil
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
	next.OAuth.BaseURL = strings.TrimSpace(next.OAuth.BaseURL)
	next.OAuth.RedirectURI = strings.TrimSpace(next.OAuth.RedirectURI)
	next.OAuth.Scopes = strings.TrimSpace(next.OAuth.Scopes)
	// Credentials are trimmed of the paste artefacts before anything compares
	// them, so a quoted `"tk_xxx"` and a bare tk_xxx are the same setting here —
	// and the mask placeholder is still recognised as the mask placeholder.
	next.OAuth.ClientID = shared.TrimCredential(next.OAuth.ClientID)
	next.OAuth.ClientSecret = shared.TrimCredential(next.OAuth.ClientSecret)
	next.Payment.PaymentID = shared.TrimCredential(next.Payment.PaymentID)
	next.Payment.Token = shared.TrimCredential(next.Payment.Token)
	next.Payment.SecretKey = shared.TrimCredential(next.Payment.SecretKey)

	// The SPA echoes the mask placeholder for a secret it did not touch; only that
	// placeholder preserves the stored value. An emptied field is a real clearing.
	if next.OAuth.ClientSecret == Redacted {
		next.OAuth.ClientSecret = existing.OAuth.ClientSecret
	}
	if next.Payment.Token == Redacted {
		next.Payment.Token = existing.Payment.Token
	}
	if next.Payment.SecretKey == Redacted {
		next.Payment.SecretKey = existing.Payment.SecretKey
	}
	// The owner's switch stays off only when there is no application named or no
	// secret at all to sign with. Either box counts: which one NodeLoc hands out
	// depends on the payment application's release, and a store that can sign 下单
	// must not be switched off because the other box is empty.
	hasSecret := next.Payment.Token != "" || next.Payment.SecretKey != ""
	next.Payment.Enabled = next.Payment.Enabled && next.Payment.PaymentID != "" && hasSecret
	// A settings document that does not mention a switch at all is an older one,
	// not an instruction to turn it on, so the stored value carries over first.
	if next.Features.Checkin == nil {
		next.Features.Checkin = existing.Features.Checkin
	}
	if next.Features.Coupons == nil {
		next.Features.Coupons = existing.Features.Coupons
	}
	if next.Features.StockAlertThreshold == nil {
		next.Features.StockAlertThreshold = existing.Features.StockAlertThreshold
	}
	next.MergeDefaults()
	next.Normalize()
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

// TestOAuth answers the one question the settings page cannot answer by looking at
// its own fields: does NodeLoc recognise this Client ID / Client Secret pair?
//
// Building the authorization URL used to be the whole test, and it proved nothing —
// the live forum redirects /oauth-provider/authorize to its own login page even for a
// client_id that has never existed, so a shop whose Secret was reset or mistyped got a
// green light there and every buyer then fell off after logging into NodeLoc. The
// probe therefore speaks to the token endpoint (with a code that cannot exist, which
// touches nobody) and reads OAuth's own error name off it.
func (s *Service) TestOAuth(ctx context.Context) (bool, string, string) {
	s.mu.Lock()
	probe := s.oauthProbe
	s.mu.Unlock()
	if probe == nil {
		return false, "", ErrNotInstalled.Error()
	}
	outcome := probe(ctx)
	link := ""
	if outcome.AuthorizeURL != "" {
		link = "（授权链接已生成，可直接在浏览器里打开试一次登录）"
	}
	ok := false
	reason := ""
	switch outcome.Code {
	case "":
		ok = true
		reason = "NodeLoc 认这组 Client ID 与 Client Secret，买家点「NodeLoc 登录」后商店能换到令牌"
	case "not_configured":
		reason = "NodeLoc 登录还没配置完整：" + describeOAuthMissing(outcome.Detail)
	case "unreachable":
		reason = "连不上 NodeLoc 的 OAuth 接口：" + describeOAuthUnreachable(outcome.Detail) +
			"。请检查「NodeLoc 站点地址」拼写，以及这台商店服务器能不能出网"
	case "route_missing":
		reason = "这个地址上没有 NodeLoc 的 OAuth 接口：" + describeOAuthUnreachable(outcome.Detail) +
			"。NodeLoc 的 OAuth 挂在论坛域名下的 /oauth-provider 上，请把「NodeLoc 站点地址」填成论坛本体（不是 API 网关、也不是登录镜像域）"
	case "client_rejected":
		reason = "NodeLoc 不认这组凭据：Client ID 与 Client Secret 对不上这个 OAuth 应用。" +
			"请到论坛的 OAuth 应用页重新复制这两串（应用被重建、Secret 被重置、或把支付应用的 Payment ID 填进 Client ID 都会这样）：" +
			describeOAuthUnreachable(outcome.Detail)
	case "redirect_mismatch":
		reason = "凭据是对的，但 NodeLoc 不接受商店报的回调地址：" + describeOAuthUnreachable(outcome.Detail) +
			"。回调必须是后台「站点域名」推出来的那一条 /api/v1/auth/oauth/callback，并且要和 OAuth 应用里登记的回调一字不差（协议、域名、结尾斜杠都算）"
	case "grant_unsupported":
		reason = "这个 OAuth 应用不允许商店走的授权方式（授权码模式）：" + describeOAuthUnreachable(outcome.Detail) +
			"。请确认应用勾选的是 authorization code flow"
	case "scope_rejected":
		reason = "这个 OAuth 应用没有通过商店申请的 scope（至少要有 openid，取邮箱还要 email）：" +
			describeOAuthUnreachable(outcome.Detail)
	default:
		reason = "测试未能完成：" + describeOAuthUnreachable(outcome.Detail) + "（code=" + outcome.Code + "）"
	}
	return ok, outcome.AuthorizeURL, reason + link
}

// describeOAuthMissing turns the provider's 「缺少 X」 into the 设置 page's own field
// names, because the shop owner reads this sentence with the page in front of them.
func describeOAuthMissing(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return "请填好「NodeLoc 站点地址」「Client ID」「Client Secret」"
	}
	for _, marker := range []string{"缺少 ", "missing "} {
		if index := strings.Index(strings.ToLower(detail), marker); index >= 0 {
			return "请补齐后台设置里的 " + strings.TrimSpace(detail[index+len(marker):])
		}
	}
	return detail
}

// describeOAuthUnreachable keeps NodeLoc's own words after the Chinese lead: which of
// DNS, TLS, a 500 or a 404 web page it was decides whether the owner fixes the address
// box or the host's network.
func describeOAuthUnreachable(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return "NodeLoc 没有给出原因"
	}
	if runes := []rune(detail); len(runes) > 200 {
		return string(runes[:200]) + "…"
	}
	return detail
}

// TestPayment answers the settings page's 「测试支付网关」 from the payment module's
// own reading of one probe call.
//
// The probe replays 下单 for one of this shop's settled orders — the only server-side
// payment call that proves the signing credentials without putting a charge on anybody —
// and then asks 查单 about a transaction id that does not exist. An unreachable host, an
// empty field, a rejected signature, an unknown Payment ID, a calling IP the application
// does not allow, a drifted clock, a route NodeLoc only lets a browser call and a store
// with no order to replay are eight different fixes, and reporting them all as 「支付网关
// 可达」 is how a wrong Payment Token survived a settings page.
//
// The payment module already sorts those apart and hands back a code; this reads the
// code. Matching on the English wording of a sentinel was how the two modules used to
// disagree about the same failure — a reworded payment error silently turned 「凭据不对」
// into 「网关可达」 here.
func (s *Service) TestPayment(ctx context.Context) (bool, string) {
	s.mu.Lock()
	probe := s.paymentProbe
	s.mu.Unlock()
	if probe == nil {
		return false, "系统尚未初始化"
	}
	outcome := probe(ctx)
	note := ""
	if outcome.Style != "" {
		note = "（本次实测：" + outcome.Style + "）"
	}
	if outcome.Code == "" {
		return true, "支付网关连通正常，NodeLoc 已接受商店的签名" + note
	}
	switch outcome.Code {
	case "not_configured":
		// Every branch ends with the style note: a store that once reached NodeLoc
		// and then lost a credential still needs to see what the last accepted call
		// was signed with.
		return false, "支付参数未配置完整，缺少 " + describePaymentMissing(outcome.Detail) + note
	// The payment application answered and does not know this Payment ID. Nothing
	// a signing key fixes: it is one field on this page.
	case "payment_id_unknown":
		return false, "支付网关可达，但 NodeLoc 在这个地址上找不到后台填写的 Payment ID。请到 NodeLoc 的支付应用页复制 pay_ 开头的那串 ID（OAuth 登录用的是另一个 Client ID，两者不能混填），并确认「支付 API 地址」指向挂着这个支付应用的域名" + note
	// 查单 is Discourse's own browser route: it answers 「["BAD CSRF"]」 to every
	// server-side call, so it proves the store cannot use it — and proves nothing
	// about whether the shop can take money. Saying 「不可达」 here would send the
	// owner off to fix a working payment setup.
	case "provider_guarded":
		return true, "支付网关可达，但 NodeLoc 的查单接口只接受论坛后台的浏览器会话，服务器端调不动它。商店改用下单回执与支付回调核实到账，收款与发货不受影响" + note
	// Nothing in the shop's own history could be replayed at 下单 without opening a
	// fresh payment, so the button has no evidence either way. Reporting the guarded
	// 查单 as an all-clear here is what let a wrong Payment Token look configured.
	case "not_verified":
		return false, "还没有测出下单凭据是否可用：" + outcome.Message + "请完成并支付一笔真实订单，再按这个按钮——商店会复用那一单去核实签名，不会重复收款" + note
	// Every payment call carries a 10-digit second timestamp and NodeLoc refuses
	// one outside its window, so a server whose clock has drifted fails 下单/查单/
	// 转账 at once. Nothing in the credentials is wrong in that case.
	case "provider_clock":
		return false, "NodeLoc 拒绝了商店的请求时间戳：这台服务器的系统时间与标准时间相差过大，请在宿主机上同步时钟（NTP），支付凭据本身没有问题" + note
	case "provider_rejected":
		return false, "网关可达，但 NodeLoc 拒绝了商店的签名：Payment Token 与 Secret Key 都和这个 Payment ID 配不上（商店已依次试过文档写的几种签名方式），请到 NodeLoc 后台重新核对后复制粘贴" + note
	// The probe's transaction id does not exist by design, so 「not found」 is the
	// sound of NodeLoc reading the request and answering it.
	case "not_found":
		return true, "支付网关连通正常，签名已被 NodeLoc 接受（探测用的交易号本就不存在）" + note
	case "provider_unreachable":
		return false, "无法连接支付网关: " + describePaymentUnreachable(outcome.Detail) + note
	default:
		return false, outcome.Message + "（code=" + outcome.Code + "）" + note
	}
}

// describePaymentUnreachable keeps NodeLoc's or the transport's own reason
// (dial tcp: no such host, TLS handshake, HTTP 500) after the Chinese lead,
// because which of those it is decides whether the owner fixes DNS or the host.
func describePaymentUnreachable(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return "NodeLoc 没有应答"
	}
	return detail
}

// paymentFieldLabels name the payment settings the way the 设置 page labels them,
// so a diagnostic points at a field the owner can see.
var paymentFieldLabels = map[string]string{
	"base_url":   "支付 API 地址",
	"payment_id": "Payment ID",
	"token":      "Payment Token",
	"secret_key": "Secret Key",
}

func describePaymentMissing(msg string) string {
	// Both boxes empty is one instruction, not two: NodeLoc hands some payment
	// applications a single secret, and either field is enough to sign with.
	if strings.Contains(msg, "token") && strings.Contains(msg, "secret_key") {
		return "Payment Token 与 Secret Key（NodeLoc 的支付应用只给一串时，填进任意一个即可）"
	}
	tail := msg
	if idx := strings.Index(msg, "缺少 "); idx >= 0 {
		tail = msg[idx+len("缺少 "):]
	} else if idx := strings.LastIndex(msg, "："); idx >= 0 {
		tail = msg[idx+len("："):]
	}
	labels := make([]string, 0, 4)
	for _, name := range strings.Split(strings.TrimSpace(tail), "、") {
		if label, ok := paymentFieldLabels[name]; ok {
			labels = append(labels, label)
			continue
		}
		labels = append(labels, name)
	}
	return strings.Join(labels, "、")
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
