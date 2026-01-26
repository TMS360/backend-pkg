package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	AppName           string `mapstructure:"APP_NAME"`
	AppEnv            string `mapstructure:"APP_ENV"`
	AppDebug          bool   `mapstructure:"APP_DEBUG"`
	AppPort           string `mapstructure:"APP_PORT"`
	SigningKey        string `mapstructure:"SIGNING_KEY"`
	GRPCPort          string `mapstructure:"GRPC_PORT"`
	HTTPServer        `mapstructure:"HTTP"`
	PostgresSQLConfig `mapstructure:"DB"`
	KafkaConfig       `mapstructure:"KAFKA"`
	RedisConfig       `mapstructure:"REDIS"`
	JWTConfig         `mapstructure:"JWT"`
	SamsaraConfig     `mapstructure:"SAMSARA"`
	HereConfig        `mapstructure:"HERE"`
	ClickHouseConfig  `mapstructure:"CLICKHOUSE"`
	AwsConfig         `mapstructure:"AWS"`
}

type HTTPServer struct {
	Timeout        time.Duration `mapstructure:"TIMEOUT"`
	IdleTimeout    time.Duration `mapstructure:"IDLE_TIMEOUT"`
	AllowedOrigins []string      `mapstructure:"ALLOWED_ORIGINS"`
}

type PostgresSQLConfig struct {
	Host       string `mapstructure:"HOST"`
	Port       string `mapstructure:"PORT"`
	DBName     string `mapstructure:"DATABASE"`
	DBNameTest string `mapstructure:"DATABASE_TEST"`
	User       string `mapstructure:"USERNAME"`
	Password   string `mapstructure:"PASSWORD"`
	SSLMode    string `mapstructure:"SSLMODE"`
	TimeZone   string `mapstructure:"TIMEZONE"`
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

type HereConfig struct {
	RouterHost  string `mapstructure:"ROUTER_HOST"`
	GeocodeHost string `mapstructure:"GEOCODE_HOST"`
	LookupHost  string `mapstructure:"LOOKUP_HOST"`
}

type ClickHouseConfig struct {
	Host     string `mapstructure:"HOST"`
	Port     string `mapstructure:"PORT"`
	DBName   string `mapstructure:"DATABASE"`
	User     string `mapstructure:"USERNAME"`
	Password string `mapstructure:"PASSWORD"`
}

type AwsConfig struct {
	AccessKeyID     string `mapstructure:"ACCESS_KEY_ID"`
	SecretAccessKey string `mapstructure:"SECRET_ACCESS_KEY"`
	Region          string `mapstructure:"REGION"`
	BucketName      string `mapstructure:"BUCKET_NAME"`
}

var Prefixes = []string{"http", "db", "kafka", "redis", "jwt", "redis", "samsara", "here", "clickhouse", "aws"}

func MapConfig() {
	for _, key := range viper.AllKeys() {
		for _, prefix := range Prefixes {
			target := prefix + "_"

			if strings.HasPrefix(key, target) {
				newKey := strings.Replace(key, target, prefix+".", 1)
				viper.Set(newKey, viper.Get(key))
				break
			}
		}
	}
}
