package rbac

import (
	"testing"
)

func TestMatchesNamespace(t *testing.T) {
	tests := []struct {
		permNamespace    string
		requestNamespace string
		expected         bool
	}{
		{"*", "production", true},
		{"*", "staging", true},
		{"production", "production", true},
		{"production", "staging", false},
		{"staging", "production", false},
	}

	for _, tt := range tests {
		result := matchesNamespace(tt.permNamespace, tt.requestNamespace)
		if result != tt.expected {
			t.Errorf("matchesNamespace(%q, %q) = %v, expected %v",
				tt.permNamespace, tt.requestNamespace, result, tt.expected)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		slice    []string
		item     string
		expected bool
	}{
		{[]string{"pods", "deployments"}, "pods", true},
		{[]string{"pods", "deployments"}, "services", false},
		{[]string{"*"}, "anything", true},
		{[]string{"get", "list"}, "get", true},
		{[]string{"get", "list"}, "delete", false},
		{[]string{}, "anything", false},
	}

	for _, tt := range tests {
		result := contains(tt.slice, tt.item)
		if result != tt.expected {
			t.Errorf("contains(%v, %q) = %v, expected %v",
				tt.slice, tt.item, result, tt.expected)
		}
	}
}

func TestNormalizeNamespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "default"},
		{"  ", "default"},
		{"production", "production"},
		{"  staging  ", "staging"},
		{"dev", "dev"},
	}

	for _, tt := range tests {
		result := NormalizeNamespace(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeNamespace(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatPermissionDenied(t *testing.T) {
	tests := []struct {
		reason   string
		expected string
	}{
		{"", "Permission denied"},
		{"Missing access", "❌ Missing access"},
		{"Not found", "❌ Not found"},
	}

	for _, tt := range tests {
		result := FormatPermissionDenied(tt.reason)
		if result != tt.expected {
			t.Errorf("FormatPermissionDenied(%q) = %q, expected %q", tt.reason, result, tt.expected)
		}
	}
}

func TestPermissionCheck_Structure(t *testing.T) {
	// Test PermissionCheck structure
	check := PermissionCheck{
		TelegramUserID: 123456789,
		Namespace:      "production",
		Resource:       "pods",
		Verb:           "logs",
		ResourceName:   "frontend-pod",
		Selector:       "app=frontend",
	}

	if check.TelegramUserID != 123456789 {
		t.Error("UserID mismatch")
	}
	if check.Namespace != "production" {
		t.Error("Namespace mismatch")
	}
	if check.Resource != "pods" {
		t.Error("Resource mismatch")
	}
	if check.Verb != "logs" {
		t.Error("Verb mismatch")
	}
	if check.ResourceName != "frontend-pod" {
		t.Error("ResourceName mismatch")
	}
	if check.Selector != "app=frontend" {
		t.Error("Selector mismatch")
	}
}
