package config

import (
	"errors"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		EnvAddr, EnvPort, EnvDatabaseURL, EnvAppEnv, EnvServiceKeys, EnvAPIKey, EnvHashIDSalt,
		EnvDBHost, EnvDBPort, EnvDBUser, EnvDBPassword, EnvDBName, EnvDBSSLMode,
		EnvGeocoderURL, EnvAPIBrasilToken, EnvAPIBrasilURL, EnvAPIBrasilHomolog,
	} {
		t.Setenv(name, "")
	}
}

func TestLoadSucceedsOutsideDevelopmentWithoutKeys(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAppEnv, "production")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("config should not be nil")
	}
}

func TestValidateAuthFailsOutsideDevelopmentWithoutKeys(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAppEnv, "production")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err = cfg.ValidateAuth(); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("error = %v, want ErrAuthRequired", err)
	}
}

func TestValidateAuthPassesInDevelopmentWithoutKeys(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err = cfg.ValidateAuth(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadOutsideDevelopmentWithKeys(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"service keys", map[string]string{EnvServiceKeys: "crm:secret"}},
		{"api key", map[string]string{EnvAPIKey: "secret"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(EnvAppEnv, "production")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err = cfg.ValidateAuth(); err != nil {
				t.Fatalf("unexpected validate error: %v", err)
			}
			if cfg.AllowNoopAuth() {
				t.Error("noop auth must not be allowed with keys")
			}
		})
	}
}

func TestLoadDevelopmentDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppEnv != "development" {
		t.Errorf("app env = %q, want development", cfg.AppEnv)
	}
	if !cfg.AllowNoopAuth() {
		t.Error("noop auth should be allowed in development without keys")
	}
	if cfg.Addr != ":8080" {
		t.Errorf("addr = %q, want :8080", cfg.Addr)
	}
}

func TestLoadAddr(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvPort, "9000")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" {
		t.Errorf("addr = %q, want :9000", cfg.Addr)
	}

	t.Setenv(EnvAddr, "127.0.0.1:7000")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:7000" {
		t.Errorf("addr = %q, want 127.0.0.1:7000", cfg.Addr)
	}
}

func TestLoadServiceKeys(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvServiceKeys, " crm : secret-a , bad, :x, y:, other:secret:b ")
	t.Setenv(EnvAPIKey, "  legacy  ")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ServiceKeys) != 2 {
		t.Fatalf("service keys = %v, want 2 entries", cfg.ServiceKeys)
	}
	if cfg.ServiceKeys["secret-a"] != "crm" {
		t.Errorf("secret-a maps to %q, want crm", cfg.ServiceKeys["secret-a"])
	}
	if cfg.ServiceKeys["secret:b"] != "other" {
		t.Errorf("secret:b maps to %q, want other", cfg.ServiceKeys["secret:b"])
	}
	if cfg.APIKey != "legacy" {
		t.Errorf("api key = %q, want legacy", cfg.APIKey)
	}
}

func TestLoadDatabaseURL(t *testing.T) {
	t.Run("from DATABASE_URL", func(t *testing.T) {
		clearEnv(t)
		t.Setenv(EnvDatabaseURL, "postgres://u:p@h:1/d")
		t.Setenv(EnvDBHost, "ignored")

		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DatabaseURL != "postgres://u:p@h:1/d" {
			t.Errorf("dsn = %q", cfg.DatabaseURL)
		}
	})

	t.Run("from DB parts", func(t *testing.T) {
		clearEnv(t)
		t.Setenv(EnvDBHost, "db")
		t.Setenv(EnvDBPort, "5433")
		t.Setenv(EnvDBUser, "user")
		t.Setenv(EnvDBPassword, "pass")
		t.Setenv(EnvDBName, "name")
		t.Setenv(EnvDBSSLMode, "require")

		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		want := "postgres://user:pass@db:5433/name?sslmode=require"
		if cfg.DatabaseURL != want {
			t.Errorf("dsn = %q, want %q", cfg.DatabaseURL, want)
		}
	})

	t.Run("DB parts defaults", func(t *testing.T) {
		clearEnv(t)
		t.Setenv(EnvDBHost, "db")

		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		want := "postgres://:@db:5432/?sslmode=disable"
		if cfg.DatabaseURL != want {
			t.Errorf("dsn = %q, want %q", cfg.DatabaseURL, want)
		}
	})

	t.Run("default", func(t *testing.T) {
		clearEnv(t)

		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DatabaseURL != DefaultDatabaseURL {
			t.Errorf("dsn = %q, want default", cfg.DatabaseURL)
		}
	})
}

func TestLoadHashIDSalt(t *testing.T) {
	clearEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HashIDSalt != "gmaps-scraper-pro-default-salt" {
		t.Errorf("salt = %q, want default", cfg.HashIDSalt)
	}

	t.Setenv(EnvHashIDSalt, "custom-salt")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HashIDSalt != "custom-salt" {
		t.Errorf("salt = %q, want custom-salt", cfg.HashIDSalt)
	}
}

func TestIsEnvSet(t *testing.T) {
	t.Setenv("GMAPS_TEST_PRESENT", "")
	if !IsEnvSet("GMAPS_TEST_PRESENT") {
		t.Error("variable set to empty should count as set")
	}
	if IsEnvSet("GMAPS_TEST_ABSENT_VARIABLE") {
		t.Error("absent variable should not count as set")
	}
}

func loadConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return cfg
}

func TestGeocoderURLDefault(t *testing.T) {
	clearEnv(t)
	if got := loadConfig(t).GeocoderURL; got != DefaultGeocoderURL {
		t.Fatalf("GeocoderURL = %q", got)
	}
}

func TestGeocoderURLTrimsTrailingSlash(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvGeocoderURL, "http://localhost:9000/")
	if got := loadConfig(t).GeocoderURL; got != "http://localhost:9000" {
		t.Fatalf("GeocoderURL = %q", got)
	}
}

func TestAPIBrasilTokenUnsetMeansNotConfigured(t *testing.T) {
	clearEnv(t)
	cfg := loadConfig(t)
	if cfg.APIBrasilToken != "" || cfg.HasAPIBrasil() {
		t.Fatalf("token = %q, configured = %v", cfg.APIBrasilToken, cfg.HasAPIBrasil())
	}
}

func TestAPIBrasilTokenIsTrimmed(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAPIBrasilToken, "  secret  ")
	cfg := loadConfig(t)
	if cfg.APIBrasilToken != "secret" || !cfg.HasAPIBrasil() {
		t.Fatalf("token = %q, configured = %v", cfg.APIBrasilToken, cfg.HasAPIBrasil())
	}
}

func TestAPIBrasilURLDefault(t *testing.T) {
	clearEnv(t)
	if got := loadConfig(t).APIBrasilURL; got != DefaultAPIBrasilURL {
		t.Fatalf("APIBrasilURL = %q", got)
	}
}

func TestAPIBrasilURLOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAPIBrasilURL, "http://localhost:9100/")
	if got := loadConfig(t).APIBrasilURL; got != "http://localhost:9100" {
		t.Fatalf("APIBrasilURL = %q", got)
	}
}

func TestAPIBrasilHomologParsing(t *testing.T) {
	cases := map[string]bool{"": false, "false": false, "true": true, "1": true, "garbage": false}
	for raw, want := range cases {
		clearEnv(t)
		t.Setenv(EnvAPIBrasilHomolog, raw)
		if got := loadConfig(t).APIBrasilHomolog; got != want {
			t.Errorf("APIBRASIL_HOMOLOG=%q: got %v, want %v", raw, got, want)
		}
	}
}
