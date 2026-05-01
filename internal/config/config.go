package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Addr                   string
	DataDir                string
	DatabasePath           string
	SessionCookieName      string
	SessionTTL             time.Duration
	DiscoveryRefresh       time.Duration
	DockerHost             string
	LabelNamespace         string
	BootstrapAdminUsername string
	BootstrapAdminPassword string
}

func LoadFromEnv() Config {
	dataDir := stringDefault(os.Getenv("MCPORTAL_DATA_DIR"), filepath.Join(".", "data"))

	return Config{
		Addr:                   stringDefault(os.Getenv("MCPORTAL_ADDR"), ":8080"),
		DataDir:                dataDir,
		DatabasePath:           stringDefault(os.Getenv("MCPORTAL_DATABASE_PATH"), filepath.Join(dataDir, "mcportal.db")),
		SessionCookieName:      stringDefault(os.Getenv("MCPORTAL_SESSION_COOKIE_NAME"), "mcportal_session"),
		SessionTTL:             durationDefault(os.Getenv("MCPORTAL_SESSION_TTL"), 7*24*time.Hour),
		DiscoveryRefresh:       durationDefault(os.Getenv("MCPORTAL_DISCOVERY_REFRESH"), 30*time.Second),
		DockerHost:             os.Getenv("DOCKER_HOST"),
		LabelNamespace:         stringDefault(os.Getenv("MCPORTAL_LABEL_NAMESPACE"), "mcportal"),
		BootstrapAdminUsername: os.Getenv("MCPORTAL_BOOTSTRAP_ADMIN_USERNAME"),
		BootstrapAdminPassword: os.Getenv("MCPORTAL_BOOTSTRAP_ADMIN_PASSWORD"),
	}
}

func stringDefault(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func durationDefault(v string, fallback time.Duration) time.Duration {
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func BoolFromEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
