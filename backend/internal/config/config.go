package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds process configuration loaded from the environment.
type Config struct {
	App        AppConfig
	Encryption EncryptionConfig
	HTTP       HTTPConfig
	DB         DBConfig
	Redis      RedisConfig
	Storage    StorageConfig
	CORS       CORSConfig
	SMTP       SMTPConfig
	Queue      QueueConfig
	Bulk       BulkConfig
	Centrifugo CentrifugoConfig
	Auth       AuthConfig
	JWT        JWTConfig
	Log        LogConfig
	VAPID      VAPIDConfig
	Expo       ExpoConfig
	Search     SearchConfig
	Sentry     SentryConfig
	Tenancy    TenancyConfig
}

// TenancyConfig controls organizations (the multi-tenant shell). See
// docs/design/tenant-core.md.
type TenancyConfig struct {
	// Enabled mounts the /v1/tenant routes. Off = single-scope app.
	Enabled bool
	// SelfServe lets signed-in users create organizations themselves.
	SelfServe bool
	// CacheTTL is how long a resolved organization + membership stays in
	// Redis. Changes invalidate it immediately; the TTL only bounds memory.
	CacheTTL time.Duration
}

// SentryConfig configures error reporting to a Sentry-protocol endpoint.
// An empty DSN disables reporting entirely.
type SentryConfig struct {
	DSN         string
	Environment string
	// Release defaults to the SDK's own detection (SENTRY_RELEASE, build info).
	Release string
	// TracesSampleRate is the share of HTTP requests sent as performance
	// transactions (0 = tracing off).
	TracesSampleRate float64
}

// AuthConfig holds NextAuth adapter integration settings.
type AuthConfig struct {
	AdapterSecret string
	WebAuthnRPID  string
	FrontendURL   string
	// ReviewAccounts maps a lower-cased email to a fixed sign-in code for
	// App Store / Play review (AUTH_REVIEW_ACCOUNTS=email:code,...). Empty = off.
	ReviewAccounts map[string]string
	// AppleNativeClientIDs are accepted id_token audiences for native Apple
	// sign-in (iOS bundle id). GoogleNativeClientIDs are the iOS/Android OAuth
	// client ids; the web client id from OAuth settings is accepted as well.
	AppleNativeClientIDs  []string
	GoogleNativeClientIDs []string
	// Sign in with Apple key (.p8) used to exchange native authorization codes
	// and revoke refresh tokens on account deletion. Optional.
	AppleTeamID     string
	AppleKeyID      string
	ApplePrivateKey string
	// QR sign-in location lookup. QRGeoIPDB is an optional MaxMind
	// GeoLite2-City .mmdb path (AUTH_QR_GEOIP_DB). QRTrustGeoHeaders trusts
	// edge geo headers (CF-IPCountry / CF-IPCity / CF-Region) and must only be
	// on when the edge (Cloudflare) overwrites them (AUTH_QR_TRUST_GEO_HEADERS).
	QRGeoIPDB         string
	QRTrustGeoHeaders bool
}

// VAPIDConfig holds Web Push keys (empty = push disabled).
type VAPIDConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// ExpoConfig holds the Expo Push Service settings (mobile push). The access
// token is optional: only needed with Expo "enhanced push security".
type ExpoConfig struct {
	AccessToken string
}

// JWTConfig holds access JWT and opaque refresh token settings.
type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

type AppConfig struct {
	Name string
	Env  string
}

// EncryptionConfig holds at-rest secret encryption material.
type EncryptionConfig struct {
	Key string
}

type HTTPConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int32
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Name,
		c.SSLMode,
	)
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type StorageConfig struct {
	Driver    string // minio | s3 | local
	LocalPath string
	MaxBytes  int64
	MinIO     ObjectStoreConfig
	S3        ObjectStoreConfig
}

// ObjectStoreConfig holds S3-compatible credentials (MinIO or S3).
type ObjectStoreConfig struct {
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	UsePathStyle  bool
	PublicBaseURL string
}

type CORSConfig struct {
	AllowedOrigins []string
}

// SMTPConfig configures outbound email (MailHog in development).
type SMTPConfig struct {
	Driver   string
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
}

type LogConfig struct {
	Level  string
	Format string
}

// QueueConfig controls Asynq background jobs.
type QueueConfig struct {
	Enabled         bool
	Concurrency     int
	WorkerInProcess bool
}

// BulkConfig controls bulk action engine thresholds.
type BulkConfig struct {
	SyncMax       int
	RollbackHours int
}

// CentrifugoConfig controls realtime pub/sub via Centrifugo.
type CentrifugoConfig struct {
	Enabled   bool
	APIURL    string
	APIKey    string
	TokenHMAC string
	WSURL     string
	TokenTTL  time.Duration
}

// SearchConfig controls Meilisearch-backed command palette indexing.
type SearchConfig struct {
	Enabled     bool
	Driver      string
	MeiliHost   string
	MeiliKey    string
	IndexPrefix string
}

// Load reads optional .env then environment variables into Config.
func Load() (Config, error) {
	_ = godotenv.Load("../.env", ".env")

	frontendURL := getEnv("PUBLIC_FRONTEND_URL", "http://localhost:3000")

	cfg := Config{
		App: AppConfig{
			Name: getEnv("APP_NAME", "api"),
			Env:  getEnv("APP_ENV", "development"),
		},
		Encryption: EncryptionConfig{
			Key: getEnv("APP_ENCRYPTION_KEY", defaultEncryptionKey),
		},
		HTTP: HTTPConfig{
			Addr:         getEnv("APP_HTTP_ADDR", ":8080"),
			ReadTimeout:  getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		},
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "127.0.0.1"),
			Port:     getInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "app"),
			Password: getEnv("DB_PASSWORD", "change_me"),
			Name:     getEnv("DB_NAME", "app"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			MaxConns: int32(getInt("DB_MAX_CONNS", 20)),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		Storage: loadStorageConfig(),
		CORS: CORSConfig{
			AllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		},
		SMTP: SMTPConfig{
			Driver:   strings.ToLower(getEnv("MAIL_DRIVER", "smtp")),
			Host:     getEnv("SMTP_HOST", "127.0.0.1"),
			Port:     getInt("SMTP_PORT", 1025),
			Username: getEnv("SMTP_USERNAME", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			From:     firstNonEmpty(getEnv("MAIL_FROM", ""), getEnv("SMTP_FROM", "noreply@localhost")),
			FromName: getEnv("MAIL_FROM_NAME", "App"),
		},
		Queue: QueueConfig{
			Enabled:         getBool("QUEUE_ENABLED", true),
			Concurrency:     getInt("QUEUE_CONCURRENCY", 10),
			WorkerInProcess: getBool("QUEUE_WORKER_INPROCESS", true),
		},
		Bulk: BulkConfig{
			SyncMax:       getInt("BULK_SYNC_MAX", 50),
			RollbackHours: getInt("BULK_ROLLBACK_HOURS", 24),
		},
		Centrifugo: CentrifugoConfig{
			Enabled:   getBool("CENTRIFUGO_ENABLED", true),
			APIURL:    getEnv("CENTRIFUGO_API_URL", "http://127.0.0.1:8000"),
			APIKey:    getEnv("CENTRIFUGO_API_KEY", defaultCentrifugoAPIKey),
			TokenHMAC: getEnv("CENTRIFUGO_TOKEN_HMAC_SECRET", defaultCentrifugoTokenHMAC),
			WSURL:     getEnv("CENTRIFUGO_WS_URL", "ws://127.0.0.1:8000/connection/websocket"),
			TokenTTL:  getDuration("CENTRIFUGO_TOKEN_TTL", time.Hour),
		},
		Auth: AuthConfig{
			AdapterSecret: getEnv("AUTH_ADAPTER_SECRET", defaultAdapterSecret),
			WebAuthnRPID:  getEnv("AUTH_WEBAUTHN_RP_ID", hostOf(frontendURL, "localhost")),
			FrontendURL:   frontendURL,

			ReviewAccounts:        parseReviewAccounts(getEnv("AUTH_REVIEW_ACCOUNTS", "")),
			AppleNativeClientIDs:  splitCSV(getEnv("AUTH_APPLE_NATIVE_CLIENT_IDS", "")),
			GoogleNativeClientIDs: splitCSV(getEnv("AUTH_GOOGLE_NATIVE_CLIENT_IDS", "")),
			AppleTeamID:           getEnv("AUTH_APPLE_TEAM_ID", ""),
			AppleKeyID:            getEnv("AUTH_APPLE_KEY_ID", ""),
			ApplePrivateKey:       getEnv("AUTH_APPLE_PRIVATE_KEY", ""),
			QRGeoIPDB:             getEnv("AUTH_QR_GEOIP_DB", ""),
			QRTrustGeoHeaders:     getBool("AUTH_QR_TRUST_GEO_HEADERS", false),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", "app-dev-access-secret-change-me-32b"),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", "app-dev-refresh-secret-change-me-32b"),
			AccessTTL:     getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:    getDuration("JWT_REFRESH_TTL", 168*time.Hour),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "debug"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		VAPID: VAPIDConfig{
			PublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
			PrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
			Subject:    getEnv("VAPID_SUBJECT", "mailto:noreply@example.com"),
		},
		Expo: ExpoConfig{
			AccessToken: getEnv("EXPO_ACCESS_TOKEN", ""),
		},
		Search: SearchConfig{
			Enabled:     getBool("SEARCH_ENABLED", true),
			Driver:      strings.ToLower(getEnv("SEARCH_DRIVER", "meilisearch")),
			MeiliHost:   getEnv("MEILI_HOST", "http://127.0.0.1:7700"),
			MeiliKey:    getEnv("MEILI_MASTER_KEY", "app-dev-meili-master-key-change-me"),
			IndexPrefix: getEnv("MEILI_INDEX_PREFIX", "app"),
		},
		Sentry: SentryConfig{
			DSN:              strings.TrimSpace(getEnv("SENTRY_DSN", "")),
			Environment:      getEnv("SENTRY_ENVIRONMENT", getEnv("APP_ENV", "development")),
			Release:          getEnv("SENTRY_RELEASE", ""),
			TracesSampleRate: clampRate(getFloat("SENTRY_TRACES_SAMPLE_RATE", 0)),
		},
		Tenancy: TenancyConfig{
			Enabled:   getBool("TENANCY_ENABLED", false),
			SelfServe: getBool("TENANCY_SELF_SERVE", false),
			CacheTTL:  getDuration("TENANCY_CACHE_TTL", 60*time.Second),
		},
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Development-only fallbacks. validate() rejects them outside development.
const (
	defaultEncryptionKey       = "app-dev-encryption-key-32bytes!!"
	defaultAdapterSecret       = "app-dev-auth-adapter-secret-change-me"
	defaultCentrifugoAPIKey    = "app-centrifugo-api-key-change-me"
	defaultCentrifugoTokenHMAC = "app-centrifugo-token-hmac-change-me"
)

func (c Config) validate() error {
	if c.App.Name == "" {
		return fmt.Errorf("config: APP_NAME is required")
	}
	if c.HTTP.Addr == "" {
		return fmt.Errorf("config: APP_HTTP_ADDR is required")
	}
	if c.DB.Host == "" || c.DB.User == "" || c.DB.Name == "" {
		return fmt.Errorf("config: database settings are incomplete")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("config: REDIS_ADDR is required")
	}
	if c.Queue.Concurrency <= 0 {
		return fmt.Errorf("config: QUEUE_CONCURRENCY must be positive")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.RefreshTTL <= 0 {
		return fmt.Errorf("config: JWT TTLs must be positive")
	}
	if c.App.Env != "development" && c.App.Env != "test" {
		if c.JWT.AccessSecret == "" || c.JWT.RefreshSecret == "" {
			return fmt.Errorf("config: JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required outside development")
		}
		if c.JWT.AccessSecret == "app-dev-access-secret-change-me-32b" ||
			c.JWT.RefreshSecret == "app-dev-refresh-secret-change-me-32b" {
			return fmt.Errorf("config: replace default JWT secrets outside development")
		}
		if len(c.JWT.AccessSecret) < 32 {
			return fmt.Errorf("config: JWT_ACCESS_SECRET must be at least 32 characters outside development")
		}
		// Fallbacks below are public (.env.example / source); refuse them so
		// production never encrypts secrets or signs tokens with known keys.
		if c.Encryption.Key == "" || c.Encryption.Key == defaultEncryptionKey {
			return fmt.Errorf("config: set a unique APP_ENCRYPTION_KEY outside development")
		}
		if c.Auth.AdapterSecret == defaultAdapterSecret {
			return fmt.Errorf("config: replace default AUTH_ADAPTER_SECRET outside development")
		}
		if c.Centrifugo.Enabled && (c.Centrifugo.TokenHMAC == defaultCentrifugoTokenHMAC ||
			c.Centrifugo.APIKey == defaultCentrifugoAPIKey) {
			return fmt.Errorf("config: replace default CENTRIFUGO_TOKEN_HMAC_SECRET / CENTRIFUGO_API_KEY outside development")
		}
	}
	if c.JWT.AccessSecret == "" {
		return fmt.Errorf("config: JWT_ACCESS_SECRET is required")
	}
	if c.Search.Enabled {
		if c.Search.MeiliHost == "" {
			return fmt.Errorf("config: MEILI_HOST is required when SEARCH_ENABLED=true")
		}
		if c.Search.IndexPrefix == "" {
			return fmt.Errorf("config: MEILI_INDEX_PREFIX is required when SEARCH_ENABLED=true")
		}
		if c.App.Env != "development" && c.App.Env != "test" {
			if c.Search.MeiliKey == "" {
				return fmt.Errorf("config: MEILI_MASTER_KEY is required when SEARCH_ENABLED=true")
			}
			if c.Search.MeiliKey == "app-dev-meili-master-key-change-me" {
				return fmt.Errorf("config: replace default MEILI_MASTER_KEY outside development")
			}
		}
	}
	return nil
}

func loadStorageConfig() StorageConfig {
	driver := strings.ToLower(getEnv("STORAGE_DRIVER", "minio"))
	s3Endpoint := getEnv("S3_ENDPOINT", "http://127.0.0.1:9000")
	s3Bucket := getEnv("S3_BUCKET", "app")
	s3Access := getEnv("S3_ACCESS_KEY", "minioadmin")
	s3Secret := getEnv("S3_SECRET_KEY", "minioadmin")
	s3Region := getEnv("S3_REGION", "us-east-1")
	s3PathStyle := getBool("S3_USE_PATH_STYLE", true)
	s3Public := getEnv("S3_PUBLIC_BASE_URL", "")

	minioEndpoint := firstNonEmpty(getEnv("MINIO_ENDPOINT", ""), s3Endpoint)
	minioBucket := firstNonEmpty(getEnv("MINIO_BUCKET", ""), s3Bucket)
	minioAccess := firstNonEmpty(getEnv("MINIO_ACCESS_KEY", ""), s3Access)
	minioSecret := firstNonEmpty(getEnv("MINIO_SECRET_KEY", ""), s3Secret)
	minioRegion := firstNonEmpty(getEnv("MINIO_REGION", ""), s3Region)
	minioPathStyle := s3PathStyle
	if os.Getenv("MINIO_USE_PATH_STYLE") != "" {
		minioPathStyle = getBool("MINIO_USE_PATH_STYLE", true)
	}
	minioPublic := firstNonEmpty(getEnv("MINIO_PUBLIC_BASE_URL", ""), s3Public)
	if minioPublic == "" {
		minioPublic = strings.TrimRight(minioEndpoint, "/") + "/" + minioBucket
	}
	if s3Public == "" {
		s3Public = strings.TrimRight(s3Endpoint, "/") + "/" + s3Bucket
	}

	return StorageConfig{
		Driver:    driver,
		LocalPath: getEnv("STORAGE_LOCAL_PATH", "./storage/objects"),
		MaxBytes:  getInt64("STORAGE_MAX_BYTES", 10<<20),
		MinIO: ObjectStoreConfig{
			Endpoint:      minioEndpoint,
			Region:        minioRegion,
			Bucket:        minioBucket,
			AccessKey:     minioAccess,
			SecretKey:     minioSecret,
			UsePathStyle:  minioPathStyle,
			PublicBaseURL: minioPublic,
		},
		S3: ObjectStoreConfig{
			Endpoint:      s3Endpoint,
			Region:        s3Region,
			Bucket:        s3Bucket,
			AccessKey:     s3Access,
			SecretKey:     s3Secret,
			UsePathStyle:  s3PathStyle,
			PublicBaseURL: s3Public,
		},
	}
}

// hostOf returns the hostname of rawURL (no port), or fallback when it has none.
func hostOf(rawURL, fallback string) string {
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return fallback
	}
	return f
}

func clampRate(v float64) float64 {
	if v != v || v < 0 { // NaN or negative
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseReviewAccounts parses "a@x.com:123456,b@y.com:654321". Entries without
// an email or a code of at least 6 characters are ignored.
func parseReviewAccounts(raw string) map[string]string {
	out := map[string]string{}
	for _, item := range splitCSV(raw) {
		email, code, ok := strings.Cut(item, ":")
		email = strings.ToLower(strings.TrimSpace(email))
		code = strings.TrimSpace(code)
		if !ok || email == "" || !strings.Contains(email, "@") || len(code) < 6 {
			continue
		}
		out[email] = code
	}
	return out
}
