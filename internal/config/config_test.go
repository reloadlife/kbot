package config

import (
	"os"
	"testing"
)

func TestLoad_Success(t *testing.T) {
	// Set environment variables
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token-123")
	os.Setenv("ADMIN_TELEGRAM_IDS", "123456789,987654321")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")
	defer os.Unsetenv("ADMIN_TELEGRAM_IDS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.TelegramBotToken != "test-token-123" {
		t.Errorf("Expected token 'test-token-123', got '%s'", cfg.TelegramBotToken)
	}

	if len(cfg.AdminTelegramIDs) != 2 {
		t.Errorf("Expected 2 admin IDs, got %d", len(cfg.AdminTelegramIDs))
	}

	if cfg.AdminTelegramIDs[0] != 123456789 {
		t.Errorf("Expected first admin ID to be 123456789, got %d", cfg.AdminTelegramIDs[0])
	}

	if cfg.AdminTelegramIDs[1] != 987654321 {
		t.Errorf("Expected second admin ID to be 987654321, got %d", cfg.AdminTelegramIDs[1])
	}

	if cfg.LogLevel != "info" {
		t.Errorf("Expected default log level 'info', got '%s'", cfg.LogLevel)
	}

	// Check default bot namespace and deployment name
	if cfg.BotNamespace != "default" {
		t.Errorf("Expected default bot namespace 'default', got '%s'", cfg.BotNamespace)
	}

	if cfg.BotDeploymentName != "telegram-bot" {
		t.Errorf("Expected default bot deployment name 'telegram-bot', got '%s'", cfg.BotDeploymentName)
	}
}

func TestLoad_CustomLogLevel(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("ADMIN_TELEGRAM_IDS", "123456789")
	os.Setenv("LOG_LEVEL", "debug")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")
	defer os.Unsetenv("ADMIN_TELEGRAM_IDS")
	defer os.Unsetenv("LOG_LEVEL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", cfg.LogLevel)
	}
}

func TestLoad_CustomBotConfig(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("ADMIN_TELEGRAM_IDS", "123456789")
	os.Setenv("BOT_NAMESPACE", "kbot-system")
	os.Setenv("BOT_DEPLOYMENT_NAME", "kubectl-bot")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")
	defer os.Unsetenv("ADMIN_TELEGRAM_IDS")
	defer os.Unsetenv("BOT_NAMESPACE")
	defer os.Unsetenv("BOT_DEPLOYMENT_NAME")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.BotNamespace != "kbot-system" {
		t.Errorf("Expected bot namespace 'kbot-system', got '%s'", cfg.BotNamespace)
	}

	if cfg.BotDeploymentName != "kubectl-bot" {
		t.Errorf("Expected bot deployment name 'kubectl-bot', got '%s'", cfg.BotDeploymentName)
	}
}

func TestLoad_MissingToken(t *testing.T) {
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
	os.Setenv("ADMIN_TELEGRAM_IDS", "123456789")
	defer os.Unsetenv("ADMIN_TELEGRAM_IDS")

	_, err := Load()
	if err == nil {
		t.Error("Expected error for missing token, got nil")
	}
}

func TestLoad_MissingAdminIDs(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Unsetenv("ADMIN_TELEGRAM_IDS")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")

	_, err := Load()
	if err == nil {
		t.Error("Expected error for missing admin IDs, got nil")
	}
}

func TestLoad_InvalidAdminID(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("ADMIN_TELEGRAM_IDS", "123456789,invalid,987654321")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")
	defer os.Unsetenv("ADMIN_TELEGRAM_IDS")

	_, err := Load()
	if err == nil {
		t.Error("Expected error for invalid admin ID, got nil")
	}
}

func TestLoad_EmptyAdminIDs(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("ADMIN_TELEGRAM_IDS", "")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")
	defer os.Unsetenv("ADMIN_TELEGRAM_IDS")

	_, err := Load()
	if err == nil {
		t.Error("Expected error for empty admin IDs, got nil")
	}
}

func TestLoad_WhitespaceInAdminIDs(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("ADMIN_TELEGRAM_IDS", "123456789 , 987654321 , 111111111")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")
	defer os.Unsetenv("ADMIN_TELEGRAM_IDS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(cfg.AdminTelegramIDs) != 3 {
		t.Errorf("Expected 3 admin IDs, got %d", len(cfg.AdminTelegramIDs))
	}
}

func TestIsBootstrapAdmin(t *testing.T) {
	cfg := &Config{
		AdminTelegramIDs: []int64{123456789, 987654321},
	}

	tests := []struct {
		userID   int64
		expected bool
	}{
		{123456789, true},
		{987654321, true},
		{111111111, false},
		{0, false},
	}

	for _, tt := range tests {
		result := cfg.IsBootstrapAdmin(tt.userID)
		if result != tt.expected {
			t.Errorf("IsBootstrapAdmin(%d) = %v, expected %v", tt.userID, result, tt.expected)
		}
	}
}

func TestParseAdminIDs_Success(t *testing.T) {
	tests := []struct {
		input    string
		expected []int64
	}{
		{"123456789", []int64{123456789}},
		{"123456789,987654321", []int64{123456789, 987654321}},
		{"123, 456, 789", []int64{123, 456, 789}},
		{"  123  ,  456  ", []int64{123, 456}},
	}

	for _, tt := range tests {
		result, err := parseAdminIDs(tt.input)
		if err != nil {
			t.Errorf("parseAdminIDs(%q) returned error: %v", tt.input, err)
			continue
		}

		if len(result) != len(tt.expected) {
			t.Errorf("parseAdminIDs(%q) = %v, expected %v", tt.input, result, tt.expected)
			continue
		}

		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("parseAdminIDs(%q)[%d] = %d, expected %d", tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestParseAdminIDs_Errors(t *testing.T) {
	tests := []string{
		"",
		"   ",
		"abc",
		"123,abc,456",
		"123.456",
		",,,",
	}

	for _, input := range tests {
		_, err := parseAdminIDs(input)
		if err == nil {
			t.Errorf("parseAdminIDs(%q) expected error, got nil", input)
		}
	}
}
