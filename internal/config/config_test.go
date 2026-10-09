package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PORT", "8081")
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")
	t.Setenv("MONGO_DATABASE", "oig_test")
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("JWT_EXPIRATION", "30m")
	t.Setenv("DISCORD_CLIENT_ID", "client-id")
	t.Setenv("DISCORD_CLIENT_SECRET", "client-secret")
	t.Setenv("DISCORD_REDIRECT_URI", "http://localhost:8081/api/v1/auth/discord/callback")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000, http://localhost:5173")
	t.Setenv("SWAGGER_ENABLED", "true")
}

func TestLoadValidConfiguration(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppPort != "8081" || cfg.JWTExpiration != 30*time.Minute || !cfg.SwaggerEnabled {
		t.Fatalf("Load() returned unexpected config: %#v", cfg)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("expected 2 CORS origins, got %d", len(cfg.CORSAllowedOrigins))
	}
}

func TestLoadRejectsUnsafeConfiguration(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("JWT_SECRET", "short")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected an error")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET") || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsExampleSecrets(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("JWT_SECRET", "replace-with-a-random-secret-at-least-32-characters")
	t.Setenv("DISCORD_CLIENT_SECRET", "replace-with-discord-client-secret")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("Load() error = %v, want placeholder error", err)
	}
}

func TestLoadNormalizesLocalEnv(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("APP_ENV", "local")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppEnv != "development" {
		t.Fatalf("expected AppEnv to be normalized to development, got %q", cfg.AppEnv)
	}
}

func TestLoadEnvFilesPrecedence(t *testing.T) {
	dir := t.TempDir()
	envLocal := dir + "/.env.local"
	envDefault := dir + "/.env"

	if err := os.WriteFile(envDefault, []byte("TEST_VAR_A=default_a\nTEST_VAR_B=default_b\nTEST_VAR_C=default_c\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envLocal, []byte("TEST_VAR_B=local_b\nTEST_VAR_C=local_c\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Explicit process environment variable must take highest precedence
	t.Setenv("TEST_VAR_C", "process_c")

	if err := LoadEnvFiles(envLocal, envDefault); err != nil {
		t.Fatalf("LoadEnvFiles() error = %v", err)
	}

	// TEST_VAR_A was only in .env -> should be default_a
	if got := os.Getenv("TEST_VAR_A"); got != "default_a" {
		t.Fatalf("expected TEST_VAR_A=default_a, got %q", got)
	}
	// TEST_VAR_B was in .env.local and .env -> .env.local must win over .env
	if got := os.Getenv("TEST_VAR_B"); got != "local_b" {
		t.Fatalf("expected TEST_VAR_B=local_b, got %q", got)
	}
	// TEST_VAR_C was in process env, .env.local, and .env -> process env must win
	if got := os.Getenv("TEST_VAR_C"); got != "process_c" {
		t.Fatalf("expected TEST_VAR_C=process_c, got %q", got)
	}
}
