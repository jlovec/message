package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host              string
	Port              int
	DatabaseURL       string
	DatabaseHost      string
	DatabasePort      int
	DatabaseName      string
	DatabaseUser      string
	DatabasePassword  string
	DatabaseSSLMode   string
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Host:              getenv("HOST", "0.0.0.0"),
		Port:              getenvInt("PORT", 8080),
		DatabaseURL:       strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DatabaseHost:      getenv("DATABASE_HOST", "postgresql"),
		DatabasePort:      getenvInt("DATABASE_PORT", 5432),
		DatabaseName:      getenv("DATABASE_NAME", "smsforwarder_messages"),
		DatabaseUser:      getenv("DATABASE_USER", "smsforwarder_webhook"),
		DatabasePassword:  strings.TrimSpace(os.Getenv("DATABASE_PASSWORD")),
		DatabaseSSLMode:   getenv("DATABASE_SSLMODE", "disable"),
		ShutdownTimeout:   getenvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		ReadHeaderTimeout: getenvDuration("READ_HEADER_TIMEOUT", 5*time.Second),
	}

	if cfg.Port <= 0 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("PORT must be between 1 and 65535")
	}
	if cfg.DatabaseURL == "" && cfg.DatabasePassword == "" {
		return Config{}, fmt.Errorf("DATABASE_PASSWORD is required when DATABASE_URL is not set")
	}

	return cfg, nil
}

func (cfg Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

func (cfg Config) PostgresDSN() string {
	if cfg.DatabaseURL != "" {
		return cfg.DatabaseURL
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.DatabaseUser, cfg.DatabasePassword),
		Host:   fmt.Sprintf("%s:%d", cfg.DatabaseHost, cfg.DatabasePort),
		Path:   cfg.DatabaseName,
	}
	q := u.Query()
	q.Set("sslmode", cfg.DatabaseSSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func getenv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
