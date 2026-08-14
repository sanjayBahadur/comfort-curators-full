package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort int
	LogLevel string
	DBSkip   bool
	DBHost   string
	DBPort   int
	DBUser   string
	DBPass   string
	DBName   string
	DBSSL    string
}

func (c Config) DBDSN() string {
	ssl := c.DBSSL
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPass, c.DBName, ssl,
	)
}

func LoadFromEnv() Config {
	return Config{
		HTTPPort: envInt("CC_HTTP_PORT", 8080),
		LogLevel: envStr("CC_LOG_LEVEL", "info"),
		DBSkip:   envBool("CC_SKIP_DB", false),
		DBHost:   envStr("CC_DB_HOST", "localhost"),
		DBPort:   envInt("CC_DB_PORT", 5432),
		DBUser:   envStr("CC_DB_USER", ""),
		DBPass:   envStr("CC_DB_PASS", ""),
		DBName:   envStr("CC_DB_NAME", ""),
		DBSSL:    envStr("CC_DB_SSLMODE", "disable"),
	}
}

func (c Config) Validate() error {
	if !c.DBSkip {
		var missing []string
		if c.DBUser == "" {
			missing = append(missing, "CC_DB_USER")
		}
		if c.DBPass == "" {
			missing = append(missing, "CC_DB_PASS")
		}
		if c.DBName == "" {
			missing = append(missing, "CC_DB_NAME")
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
		}
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return fmt.Errorf("invalid CC_HTTP_PORT: %d", c.HTTPPort)
	}
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.LogLevel)] {
		return fmt.Errorf("invalid CC_LOG_LEVEL: %s (must be debug, info, warn, or error)", c.LogLevel)
	}
	return nil
}

func (c Config) String() string {
	return fmt.Sprintf(
		"HTTPPort=%d LogLevel=%s DBHost=%s DBPort=%d DBUser=%s DBPass=[redacted] DBName=%s DBSSL=%s",
		c.HTTPPort, c.LogLevel, c.DBHost, c.DBPort, c.DBUser, c.DBName, c.DBSSL,
	)
}

func (c Config) SafeFields() map[string]any {
	return map[string]any{
		"http_port": c.HTTPPort,
		"log_level": c.LogLevel,
		"db_host":   c.DBHost,
		"db_port":   c.DBPort,
		"db_user":   c.DBUser,
		"db_name":   c.DBName,
		"db_ssl":    c.DBSSL,
	}
}

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return def
		}
		return n
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return def
		}
		return b
	}
	return def
}
