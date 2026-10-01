package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Features FeaturesConfig `mapstructure:"features"`
	Database DatabaseConfig `mapstructure:"database"`
	Server   ServerConfig   `mapstructure:"server"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	NodeLoc  NodeLocConfig  `mapstructure:"nodeloc"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Log      LogConfig      `mapstructure:"log"`
}

type AppConfig struct {
	Name    string `mapstructure:"name"`
	Slogan  string `mapstructure:"slogan"`
	Scheme  string `mapstructure:"scheme"`
	Domain  string `mapstructure:"domain"`
	BaseURL string `mapstructure:"-"`
}

// FeaturesConfig toggles the buyer-facing capabilities the shop owner can turn
// off from 设置. They are stored in the runtime settings and overlaid by
// ApplyTo on every rebuild. The flags are inverted so that a zero value, which
// is what an older config file or a test unmarshals to, leaves every feature on.
type FeaturesConfig struct {
	CheckinDisabled     bool `mapstructure:"checkin_disabled"`
	CouponsDisabled     bool `mapstructure:"coupons_disabled"`
	StockAlertThreshold int  `mapstructure:"stock_alert_threshold"`
	// The two NodeLoc 启用 switches on 设置 live here too. They used to be stored
	// and never read, so turning NodeLoc 登录 or 收款 off changed nothing but the
	// badge on the settings card.
	OAuthDisabled        bool `mapstructure:"oauth_disabled"`
	PaymentsDisabled     bool `mapstructure:"payments_disabled"`
	RegistrationDisabled bool `mapstructure:"registration_disabled"`
}

func (f FeaturesConfig) CheckinOn() bool { return !f.CheckinDisabled }

func (f FeaturesConfig) CouponsOn() bool { return !f.CouponsDisabled }

func (f FeaturesConfig) RegistrationOn() bool { return !f.RegistrationDisabled }

func (f FeaturesConfig) OAuthOn() bool { return !f.OAuthDisabled }

func (f FeaturesConfig) PaymentsOn() bool { return !f.PaymentsDisabled }

// AlertThreshold is how little stock still counts as 库存告急.
func (f FeaturesConfig) AlertThreshold() int {
	if f.StockAlertThreshold > 0 {
		return f.StockAlertThreshold
	}
	return 5
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	AccessTTL  int    `mapstructure:"access_ttl"`
	RefreshTTL int    `mapstructure:"refresh_ttl"`
}

type NodeLocConfig struct {
	BaseURL      string `mapstructure:"base_url"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURI  string `mapstructure:"redirect_uri"`
	Scopes       string `mapstructure:"scopes"`
	// PaymentBaseURL is where 下单/查单/转账 are sent. It defaults to BaseURL
	// because most shops have only one NodeLoc host, but a forum mirrored on a
	// second domain must not have its money requests dragged along with it.
	PaymentBaseURL string `mapstructure:"payment_base_url"`
	PaymentID      string `mapstructure:"payment_id"`
	// PaymentToken (tk_xxx) signs outbound create-payment and transfer calls.
	PaymentToken string `mapstructure:"payment_token"`
	// PaymentSecret is the merchant key: it signs 查单 and verifies callbacks.
	PaymentSecret string `mapstructure:"payment_secret"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func (c *Config) GetBaseURL() string {
	if c.App.BaseURL != "" {
		return c.App.BaseURL
	}
	scheme := c.App.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.App.Domain)
}

func (c *Config) GetRedirectURI() string {
	if c.NodeLoc.RedirectURI != "" {
		return c.NodeLoc.RedirectURI
	}
	return fmt.Sprintf("%s/api/v1/auth/oauth/callback", c.GetBaseURL())
}

func Load(path string) (*Config, error) {
	v := viper.New()
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
	}

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Defaults
	v.SetDefault("app.scheme", "https")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("jwt.access_ttl", 7200)
	v.SetDefault("jwt.refresh_ttl", 604800)
	v.SetDefault("nodeloc.base_url", "https://www.nodeloc.com")
	// openid is mandatory on NodeLoc; email is opt-in because the scope needs
	// staff approval on the application.
	v.SetDefault("nodeloc.scopes", "openid profile")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
