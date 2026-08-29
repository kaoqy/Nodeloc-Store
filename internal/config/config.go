package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig      `mapstructure:"app"`
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
	PaymentID    string `mapstructure:"payment_id"`
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
	v.SetDefault("nodeloc.scopes", "openid profile email")
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
