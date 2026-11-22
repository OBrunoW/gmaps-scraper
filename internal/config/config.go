package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultDatabaseURL = "postgres://postgres:postgres@localhost:5432/gmaps_scraper?sslmode=disable" //nolint:gosec
	DefaultHashIDSalt  = "gmaps-scraper-pro-default-salt"
	DefaultAppEnv      = "development"
	DefaultPort        = "8080"

	DefaultGeocoderURL  = "https://nominatim.openstreetmap.org"
	DefaultAPIBrasilURL = "https://gateway.apibrasil.io/api/v2"

	EnvAddr        = "ADDR"
	EnvPort        = "PORT"
	EnvDatabaseURL = "DATABASE_URL"
	EnvAppEnv      = "APP_ENV"
	EnvServiceKeys = "SERVICE_KEYS"
	EnvAPIKey      = "API_KEY"
	EnvHashIDSalt  = "HASHID_SALT"
	EnvDBHost      = "DB_HOST"
	EnvDBPort      = "DB_PORT"
	EnvDBUser      = "DB_USER"
	EnvDBPassword  = "DB_PASSWORD"
	EnvDBName      = "DB_NAME"
	EnvDBSSLMode   = "DB_SSLMODE"

	EnvGeocoderURL      = "PROSPECTOR_GEOCODER_URL"
	EnvAPIBrasilToken   = "APIBRASIL_TOKEN"
	EnvAPIBrasilURL     = "APIBRASIL_BASE_URL"
	EnvAPIBrasilHomolog = "APIBRASIL_HOMOLOG"
	developmentName     = "development"
)

var ErrAuthRequired = errors.New("SERVICE_KEYS or API_KEY is required outside development")

type Config struct {
	Addr        string
	AppEnv      string
	DatabaseURL string
	ServiceKeys map[string]string
	APIKey      string
	HashIDSalt  string

	GeocoderURL      string
	APIBrasilToken   string
	APIBrasilURL     string
	APIBrasilHomolog bool
}

func Load() (*Config, error) {
	cfg := &Config{
		Addr:        resolveAddr(),
		AppEnv:      resolveAppEnv(),
		DatabaseURL: resolveDatabaseURL(),
		ServiceKeys: parseServiceKeys(os.Getenv(EnvServiceKeys)),
		APIKey:      strings.TrimSpace(os.Getenv(EnvAPIKey)),
		HashIDSalt:  envOr(EnvHashIDSalt, DefaultHashIDSalt),

		GeocoderURL:      trimmedURL(EnvGeocoderURL, DefaultGeocoderURL),
		APIBrasilToken:   strings.TrimSpace(os.Getenv(EnvAPIBrasilToken)),
		APIBrasilURL:     trimmedURL(EnvAPIBrasilURL, DefaultAPIBrasilURL),
		APIBrasilHomolog: parseBool(os.Getenv(EnvAPIBrasilHomolog)),
	}

	return cfg, nil
}

func (c *Config) ValidateAuth() error {
	if !c.IsDevelopment() && !c.HasAuthKeys() {
		return ErrAuthRequired
	}

	return nil
}

func (c *Config) IsDevelopment() bool {
	return c.AppEnv == developmentName
}

func (c *Config) HasAuthKeys() bool {
	return len(c.ServiceKeys) > 0 || c.APIKey != ""
}

func (c *Config) AllowNoopAuth() bool {
	return c.IsDevelopment() && !c.HasAuthKeys()
}

func (c *Config) HasAPIBrasil() bool {
	return c.APIBrasilToken != ""
}

func IsEnvSet(name string) bool {
	_, ok := os.LookupEnv(name)
	return ok
}

func resolveAddr() string {
	if addr := os.Getenv(EnvAddr); addr != "" {
		return addr
	}
	return ":" + envOr(EnvPort, DefaultPort)
}

func resolveAppEnv() string {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv(EnvAppEnv)))
	if appEnv == "" {
		return DefaultAppEnv
	}
	return appEnv
}

func resolveDatabaseURL() string {
	if url := os.Getenv(EnvDatabaseURL); url != "" {
		return url
	}
	if url := buildDatabaseURL(); url != "" {
		return url
	}
	return DefaultDatabaseURL
}

func buildDatabaseURL() string {
	host := os.Getenv(EnvDBHost)
	if host == "" {
		return ""
	}
	port := envOr(EnvDBPort, "5432")
	user := os.Getenv(EnvDBUser)
	password := os.Getenv(EnvDBPassword)
	name := os.Getenv(EnvDBName)
	sslMode := envOr(EnvDBSSLMode, "disable")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, name, sslMode)
}

func parseServiceKeys(raw string) map[string]string {
	keys := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		secret := strings.TrimSpace(parts[1])
		if name != "" && secret != "" {
			keys[secret] = name
		}
	}
	return keys
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func trimmedURL(key, fallback string) string {
	value := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/")
	if value == "" {
		return fallback
	}
	return value
}

func parseBool(raw string) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	return err == nil && value
}
