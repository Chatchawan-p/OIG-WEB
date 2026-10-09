// Package config loads and validates runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains all process configuration. Secret values must never be logged.
type Config struct {
	AppEnv              string
	AppPort             string
	MongoURI            string
	MongoDatabase       string
	JWTSecret           string
	JWTExpiration       time.Duration
	DiscordClientID     string
	DiscordClientSecret string
	DiscordRedirectURI  string
	CORSAllowedOrigins  []string
	SwaggerEnabled      bool
	EvidenceStoragePath string
	EvidenceMaxBytes    int64
}

// Load reads configuration from the environment and returns every validation error together.
func Load() (Config, error) {
	cfg := Config{
		AppEnv:              valueOrDefault("APP_ENV", "development"),
		AppPort:             strings.TrimPrefix(valueOrDefault("APP_PORT", "8080"), ":"),
		MongoURI:            strings.TrimSpace(os.Getenv("MONGO_URI")),
		MongoDatabase:       strings.TrimSpace(os.Getenv("MONGO_DATABASE")),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		DiscordClientID:     strings.TrimSpace(os.Getenv("DISCORD_CLIENT_ID")),
		DiscordClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
		DiscordRedirectURI:  strings.TrimSpace(os.Getenv("DISCORD_REDIRECT_URI")),
		CORSAllowedOrigins:  splitCSV(os.Getenv("CORS_ALLOWED_ORIGINS")),
		EvidenceStoragePath: valueOrDefault("EVIDENCE_STORAGE_PATH", "data/evidence"),
	}

	if cfg.AppEnv == "local" {
		cfg.AppEnv = "development"
	}

	var errs []error
	var err error
	cfg.JWTExpiration, err = time.ParseDuration(valueOrDefault("JWT_EXPIRATION", "15m"))
	if err != nil || cfg.JWTExpiration <= 0 {
		errs = append(errs, errors.New("JWT_EXPIRATION must be a positive Go duration such as 15m"))
	}
	cfg.SwaggerEnabled, err = strconv.ParseBool(valueOrDefault("SWAGGER_ENABLED", "false"))
	if err != nil {
		errs = append(errs, errors.New("SWAGGER_ENABLED must be true or false"))
	}
	cfg.EvidenceMaxBytes, err = strconv.ParseInt(valueOrDefault("EVIDENCE_MAX_BYTES", "5242880"), 10, 64)
	if err != nil || cfg.EvidenceMaxBytes < 1024 || cfg.EvidenceMaxBytes > 20*1024*1024 {
		errs = append(errs, errors.New("EVIDENCE_MAX_BYTES must be between 1024 and 20971520"))
	}
	if strings.TrimSpace(cfg.EvidenceStoragePath) == "" {
		errs = append(errs, errors.New("EVIDENCE_STORAGE_PATH is required"))
	}

	if cfg.AppPort == "" {
		errs = append(errs, errors.New("APP_PORT is required"))
	} else if port, portErr := strconv.Atoi(cfg.AppPort); portErr != nil || port < 1 || port > 65535 {
		errs = append(errs, errors.New("APP_PORT must be an integer between 1 and 65535"))
	}
	if !oneOf(cfg.AppEnv, "development", "test", "staging", "production") {
		errs = append(errs, errors.New("APP_ENV must be development, test, staging, or production"))
	}
	if cfg.MongoURI == "" {
		errs = append(errs, errors.New("MONGO_URI is required"))
	}
	if cfg.MongoDatabase == "" {
		errs = append(errs, errors.New("MONGO_DATABASE is required"))
	}
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must contain at least 32 characters"))
	} else if isPlaceholder(cfg.JWTSecret) {
		errs = append(errs, errors.New("JWT_SECRET must not use the example placeholder"))
	}
	if cfg.DiscordClientID == "" || isPlaceholder(cfg.DiscordClientID) {
		errs = append(errs, errors.New("DISCORD_CLIENT_ID is required"))
	}
	if cfg.DiscordClientSecret == "" || isPlaceholder(cfg.DiscordClientSecret) {
		errs = append(errs, errors.New("DISCORD_CLIENT_SECRET is required"))
	}
	if cfg.DiscordRedirectURI == "" {
		errs = append(errs, errors.New("DISCORD_REDIRECT_URI is required"))
	} else if parsed, parseErr := url.ParseRequestURI(cfg.DiscordRedirectURI); parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		errs = append(errs, errors.New("DISCORD_REDIRECT_URI must be an absolute URL"))
	}
	if len(cfg.CORSAllowedOrigins) == 0 {
		errs = append(errs, errors.New("CORS_ALLOWED_ORIGINS must contain at least one exact origin"))
	}
	for _, origin := range cfg.CORSAllowedOrigins {
		if origin == "*" {
			errs = append(errs, errors.New("CORS_ALLOWED_ORIGINS must not contain a wildcard"))
			continue
		}
		parsed, parseErr := url.ParseRequestURI(origin)
		if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			errs = append(errs, fmt.Errorf("CORS_ALLOWED_ORIGINS contains invalid origin %q", origin))
		}
	}

	return cfg, errors.Join(errs...)
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func isPlaceholder(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "replace-with-")
}

func valueOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func splitCSV(value string) []string {
	seen := make(map[string]struct{})
	var values []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	return values
}

// LoadEnvFiles reads environment files in order and sets any variables that are not already set.
func LoadEnvFiles(paths ...string) error {
	for _, path := range paths {
		if err := loadEnvFile(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}
