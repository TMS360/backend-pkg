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
	AppURL            string `mapstructure:"APP_URL"`
	FrontendURL       string `mapstructure:"FRONTEND_URL"`
	MobileAppURL      string `mapstructure:"MOBILE_APP_URL"`
	SigningKey        string `mapstructure:"SIGNING_KEY"`
	CleanupPassword   string `mapstructure:"CLEANUP_PASSWORD"`
	GRPCPort          string `mapstructure:"GRPC_PORT"`
	HTTPServer        `mapstructure:"HTTP"`
	PostgresSQLConfig `mapstructure:"DB"`
	KafkaConfig       `mapstructure:"KAFKA"`
	RedisConfig       `mapstructure:"REDIS"`
	JWTConfig         `mapstructure:"JWT"`
	MailConfig        `mapstructure:"MAIL"`
	SamsaraConfig     `mapstructure:"SAMSARA"`
	HereConfig        `mapstructure:"HERE"`
	RelayConfig       `mapstructure:"RELAY"`
	UspsConfig        `mapstructure:"USPS"`
	FactoringConfig   `mapstructure:"FACTORING"`
	ClickHouseConfig  `mapstructure:"CLICKHOUSE"`
	AwsConfig         `mapstructure:"AWS"`
	ServiceURLs       `mapstructure:"SERVICES"`
}

// IsProduction reports whether the app is running in production. The codebase
// uses both "prod" and "production" historically, so check both.
func (c *Config) IsProduction() bool {
	return c.AppEnv == "prod" || c.AppEnv == "production"
}

// ServiceURLs holds base URLs for peer services. Used by the tms-auth tenant-cleaner
// orchestrator to fan deletes out to each PostgreSQL service. Set via env vars
// SERVICES_AUTH_URL, SERVICES_BROKERS_URL, etc.
type ServiceURLs struct {
	AuthURL     string `mapstructure:"AUTH_URL"`
	BrokersURL  string `mapstructure:"BROKERS_URL"`
	LoadsURL    string `mapstructure:"LOADS_URL"`
	TeamsURL    string `mapstructure:"TEAMS_URL"`
	FilesURL    string `mapstructure:"FILES_URL"`
	MediatorURL string `mapstructure:"MEDIATOR_URL"`
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
	Host    string   `mapstructure:"HOST"`
	Port    string   `mapstructure:"PORT"`
	Brokers []string `mapstructure:"BROKERS"`
	GroupID string   `mapstructure:"GROUP_ID"`
	Topics  []string `mapstructure:"TOPICS"`
}

type RedisConfig struct {
	Host     string `mapstructure:"HOST"`
	Port     string `mapstructure:"PORT"`
	Password string `mapstructure:"PASSWORD"`
}

type JWTConfig struct {
	PrivateKey     string        `mapstructure:"PRIVATE_KEY"`
	PrivateKeyPath string        `mapstructure:"PRIVATE_KEY_PATH"`
	PublicKey      string        `mapstructure:"PUBLIC_KEY"`
	PublicKeyPath  string        `mapstructure:"PUBLIC_KEY_PATH"`
	AccessTTL      time.Duration `mapstructure:"ACCESS_TTL"`
	RefreshTTL     time.Duration `mapstructure:"REFRESH_TTL"`
	CookieDomain   string        `mapstructure:"COOKIE_DOMAIN"`
	CookieSecure   bool          `mapstructure:"COOKIE_SECURE"`
	CookieLaxMode  int           `mapstructure:"COOKIE_LAX_MODE"`
}

type MailConfig struct {
	// Provider selects the delivery backend: "smtp" (default) or "resend".
	Provider string `mapstructure:"PROVIDER"`
	// APIKey is used by HTTP-based providers (e.g. Resend).
	APIKey   string `mapstructure:"API_KEY"`
	Host     string `mapstructure:"HOST"`
	Port     string `mapstructure:"PORT"`
	Username string `mapstructure:"USERNAME"`
	Password string `mapstructure:"PASSWORD"`
	From     string `mapstructure:"FROM"`
}

type SamsaraConfig struct {
	Host string `mapstructure:"HOST"`
}

type HereConfig struct {
	RouterHost  string `mapstructure:"ROUTER_HOST"`
	GeocodeHost string `mapstructure:"GEOCODE_HOST"`
	LookupHost  string `mapstructure:"LOOKUP_HOST"`
}

type RelayConfig struct {
	Host string `mapstructure:"HOST"`
}

// UspsConfig holds non-secret USPS API hosts. The OAuth2 Consumer Key/Secret
// are NOT stored here — they live per-company in Redis at
// {company_id}:setting:usps_credentials as a JSON object, set by tms360-backend.
// Hosts default in the client (apis.usps.com); override via USPS_BASE_URL /
// USPS_OAUTH_HOST (e.g. the CAT/TEM sandbox apis-tem.usps.com).
type UspsConfig struct {
	BaseURL   string `mapstructure:"BASE_URL"`
	OAuthHost string `mapstructure:"OAUTH_HOST"`
}

// FactoringConfig holds per-provider defaults that callers can override via env.
// Credentials are NOT stored here — they live per-company in Redis at
// {company_id}:setting:{provider_type}_credentials as a JSON blob, set by
// tms360-backend.
type FactoringConfig struct {
	TriumphSFTPHost string `mapstructure:"TRIUMPH_SFTP_HOST"`
	TriumphSFTPPort int    `mapstructure:"TRIUMPH_SFTP_PORT"`
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
	EndpointURL     string `mapstructure:"ENDPOINT_URL"`
}

var Prefixes = []string{"http", "db", "kafka", "redis", "jwt", "redis", "mail", "samsara", "here", "relay", "usps", "factoring", "clickhouse", "aws", "services"}

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
