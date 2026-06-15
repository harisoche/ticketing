package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv            string
	AppPort           string
	DatabaseURL       string
	JWTSecret         string
	JWTIssuer         string
	JWTAccessTokenTTL time.Duration

	UploadStorageDriver  string
	UploadLocalDirectory string
	UploadMaxSizeBytes   int64
}

const (
	minJWTSecretLength        = 32
	defaultIssuer             = "ticketing-api"
	defaultUploadDriver       = "local"
	defaultUploadDirectory    = "./storage/uploads"
	defaultUploadMaxSizeBytes = int64(5 * 1024 * 1024) // 5 MiB
)

// Load reads configuration from environment variables. If a .env file is
// present in the current working directory it is loaded into the process
// environment first (without overwriting existing variables).
func Load() (*Config, error) {
	_ = loadDotEnv(".env")

	cfg := &Config{
		AppEnv:      strings.TrimSpace(os.Getenv("APP_ENV")),
		AppPort:     strings.TrimSpace(os.Getenv("APP_PORT")),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		JWTIssuer:   strings.TrimSpace(os.Getenv("JWT_ISSUER")),
	}

	if cfg.AppEnv == "" {
		cfg.AppEnv = "local"
	}
	if cfg.AppPort == "" {
		cfg.AppPort = "8080"
	}
	if cfg.JWTIssuer == "" {
		cfg.JWTIssuer = defaultIssuer
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, errors.New("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < minJWTSecretLength {
		return nil, fmt.Errorf("JWT_SECRET must be at least %d characters long", minJWTSecretLength)
	}

	cfg.UploadStorageDriver = strings.TrimSpace(os.Getenv("UPLOAD_STORAGE_DRIVER"))
	if cfg.UploadStorageDriver == "" {
		cfg.UploadStorageDriver = defaultUploadDriver
	}
	if cfg.UploadStorageDriver != "local" {
		return nil, fmt.Errorf("unsupported UPLOAD_STORAGE_DRIVER %q (only \"local\" is supported in Phase 5)", cfg.UploadStorageDriver)
	}
	cfg.UploadLocalDirectory = strings.TrimSpace(os.Getenv("UPLOAD_LOCAL_DIRECTORY"))
	if cfg.UploadLocalDirectory == "" {
		cfg.UploadLocalDirectory = defaultUploadDirectory
	}
	maxRaw := strings.TrimSpace(os.Getenv("UPLOAD_MAX_SIZE_BYTES"))
	if maxRaw == "" {
		cfg.UploadMaxSizeBytes = defaultUploadMaxSizeBytes
	} else {
		v, err := strconv.ParseInt(maxRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("UPLOAD_MAX_SIZE_BYTES is invalid: %w", err)
		}
		if v <= 0 {
			return nil, errors.New("UPLOAD_MAX_SIZE_BYTES must be positive")
		}
		cfg.UploadMaxSizeBytes = v
	}

	ttlRaw := strings.TrimSpace(os.Getenv("JWT_ACCESS_TOKEN_TTL"))
	if ttlRaw == "" {
		ttlRaw = "24h"
	}
	ttl, err := time.ParseDuration(ttlRaw)
	if err != nil {
		return nil, fmt.Errorf("JWT_ACCESS_TOKEN_TTL is invalid: %w", err)
	}
	if ttl <= 0 {
		return nil, errors.New("JWT_ACCESS_TOKEN_TTL must be positive")
	}
	cfg.JWTAccessTokenTTL = ttl

	return cfg, nil
}

// loadDotEnv is a tiny KEY=VALUE parser. Existing env vars win.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		_ = os.Setenv(key, value)
	}
	return scanner.Err()
}
