package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TelegramBotToken string
	AdminTelegramIDs []int64
	LogLevel         string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	adminIDsStr := os.Getenv("ADMIN_TELEGRAM_IDS")
	if adminIDsStr == "" {
		return nil, fmt.Errorf("ADMIN_TELEGRAM_IDS environment variable is required")
	}

	// Parse admin IDs (comma-separated)
	adminIDs, err := parseAdminIDs(adminIDsStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ADMIN_TELEGRAM_IDS: %w", err)
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	return &Config{
		TelegramBotToken: token,
		AdminTelegramIDs: adminIDs,
		LogLevel:         logLevel,
	}, nil
}

func parseAdminIDs(s string) ([]int64, error) {
	parts := strings.Split(s, ",")
	ids := make([]int64, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid admin ID '%s': %w", part, err)
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one admin ID is required")
	}

	return ids, nil
}

// IsBootstrapAdmin checks if a user ID is in the bootstrap admin list
func (c *Config) IsBootstrapAdmin(userID int64) bool {
	for _, adminID := range c.AdminTelegramIDs {
		if adminID == userID {
			return true
		}
	}
	return false
}
