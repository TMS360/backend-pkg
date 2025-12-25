package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	AppEnv            string `mapstructure:"APP_ENV"`
	AppDebug          bool   `mapstructure:"APP_DEBUG"`
	AppPort           string `mapstructure:"APP_PORT"`
	SigningKey        string `mapstructure:"SIGNING_KEY"`
	HTTPServer        `mapstructure:"HTTP"`
	PostgresSQLConfig `mapstructure:"DB"`
	KafkaConfig       `mapstructure:"KAFKA"`
	RedisConfig       `mapstructure:"REDIS"`
	JWTConfig         `mapstructure:"JWT"`
	SamsaraConfig     `mapstructure:"SAMSARA"`
	ClickHouseConfig  `mapstructure:"CLICKHOUSE"`
}

type HTTPServer struct {
	Timeout        time.Duration `mapstructure:"TIMEOUT"`
	IdleTimeout    time.Duration `mapstructure:"IDLE_TIMEOUT"`
	AllowedOrigins []string      `mapstructure:"ALLOWED_ORIGINS"`
}

type PostgresSQLConfig struct {
	Host     string `mapstructure:"HOST"`
	Port     string `mapstructure:"PORT"`
	DBName   string `mapstructure:"DATABASE"`
	User     string `mapstructure:"USERNAME"`
	Password string `mapstructure:"PASSWORD"`
	SSLMode  string `mapstructure:"SSLMODE"`
	TimeZone string `mapstructure:"TIMEZONE"`
}

type KafkaConfig struct {
	Host string `mapstructure:"HOST"`
	Port string `mapstructure:"PORT"`
}

type RedisConfig struct {
	Host     string `mapstructure:"HOST"`
	Port     string `mapstructure:"PORT"`
	Password string `mapstructure:"PASSWORD"`
}

type JWTConfig struct {
	PrivateKeyPath string        `mapstructure:"PRIVATE_KEY_PATH"`
	PublicKeyPath  string        `mapstructure:"PUBLIC_KEY_PATH"`
	AccessTTL      time.Duration `mapstructure:"ACCESS_TTL"`
	RefreshTTL     time.Duration `mapstructure:"REFRESH_TTL"`
	CookieDomain   string        `mapstructure:"COOKIE_DOMAIN"`
	CookieSecure   bool          `mapstructure:"COOKIE_SECURE"`
	CookieLaxMode  int           `mapstructure:"COOKIE_LAX_MODE"`
}

type SamsaraConfig struct {
	Host string `mapstructure:"HOST"`
}

type ClickHouseConfig struct {
	Host     string `mapstructure:"HOST"`
	Port     string `mapstructure:"PORT"`
	DBName   string `mapstructure:"DATABASE"`
	User     string `mapstructure:"USERNAME"`
	Password string `mapstructure:"PASSWORD"`
}

var Prefixes = []string{"http", "db", "kafka", "redis", "jwt"}

func MapConfig(prefixes []string) {
	if len(prefixes) == 0 {
		prefixes = Prefixes
	}

	for _, key := range viper.AllKeys() {
		for _, prefix := range prefixes {
			target := prefix + "_"

			if strings.HasPrefix(key, target) {
				newKey := strings.Replace(key, target, prefix+".", 1)
				viper.Set(newKey, viper.Get(key))
				break
			}
		}
	}
}
