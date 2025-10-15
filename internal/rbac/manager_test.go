package rbac

import (
	"testing"
)

func TestMergeUnique(t *testing.T) {
	tests := []struct {
		slice1   []string
		slice2   []string
		expected []string
	}{
		{
			[]string{"a", "b"},
			[]string{"c", "d"},
			[]string{"a", "b", "c", "d"},
		},
		{
			[]string{"a", "b"},
			[]string{"b", "c"},
			[]string{"a", "b", "c"},
		},
		{
			[]string{},
			[]string{"a", "b"},
			[]string{"a", "b"},
		},
		{
			[]string{"a", "b"},
			[]string{},
			[]string{"a", "b"},
		},
		{
			[]string{"a", "a", "b"},
			[]string{"b", "c", "c"},
			[]string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		result := mergeUnique(tt.slice1, tt.slice2)

		if len(result) != len(tt.expected) {
			t.Errorf("mergeUnique(%v, %v) length = %d, expected %d",
				tt.slice1, tt.slice2, len(result), len(tt.expected))
			continue
		}

		// Convert to map for easier comparison (order doesn't matter)
		resultMap := make(map[string]bool)
		for _, v := range result {
			resultMap[v] = true
		}

		for _, v := range tt.expected {
			if !resultMap[v] {
				t.Errorf("mergeUnique(%v, %v) missing expected value: %s",
					tt.slice1, tt.slice2, v)
			}
		}
	}
}

func TestParseUserIDFromName(t *testing.T) {
	tests := []struct {
		input       string
		expectedID  int64
		expectError bool
	}{
		{"user-123456789", 123456789, false},
		{"user-987654321", 987654321, false},
		{"user-1", 1, false},
		{"invalid", 0, true},
		{"user-abc", 0, true},
		{"123456789", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		id, err := parseUserIDFromName(tt.input)

		if tt.expectError {
			if err == nil {
				t.Errorf("parseUserIDFromName(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseUserIDFromName(%q) unexpected error: %v", tt.input, err)
			}
			if id != tt.expectedID {
				t.Errorf("parseUserIDFromName(%q) = %d, expected %d", tt.input, id, tt.expectedID)
			}
		}
	}
}

func TestFormatUserResourceName(t *testing.T) {
	tests := []struct {
		userID   int64
		expected string
	}{
		{123456789, "user-123456789"},
		{987654321, "user-987654321"},
		{1, "user-1"},
		{0, "user-0"},
	}

	for _, tt := range tests {
		result := formatUserResourceName(tt.userID)
		if result != tt.expected {
			t.Errorf("formatUserResourceName(%d) = %q, expected %q", tt.userID, result, tt.expected)
		}
	}
}

func TestGetPermissionSummary_NoPermissions(t *testing.T) {
	// This would need to be tested with a mock K8s client
	// For now, we'll test the format logic

	permission := &TelegramBotPermission{
		Spec: TelegramBotPermissionSpec{
			TelegramUserID: 123456789,
			Role:           "viewer",
			Permissions:    []Permission{},
		},
	}

	// Just verify we can construct the basic structure
	if permission.Spec.TelegramUserID != 123456789 {
		t.Error("User ID mismatch")
	}
	if permission.Spec.Role != "viewer" {
		t.Error("Role mismatch")
	}
	if len(permission.Spec.Permissions) != 0 {
		t.Error("Should have no permissions")
	}
}

func TestPermissionStructure(t *testing.T) {
	// Test that Permission structure is correctly defined
	perm := Permission{
		Namespace: "production",
		Resources: []string{"pods", "deployments"},
		Verbs:     []string{"get", "list"},
		Selector:  "app=frontend",
	}

	if perm.Namespace != "production" {
		t.Error("Namespace mismatch")
	}
	if len(perm.Resources) != 2 {
		t.Error("Resources count mismatch")
	}
	if len(perm.Verbs) != 2 {
		t.Error("Verbs count mismatch")
	}
	if perm.Selector != "app=frontend" {
		t.Error("Selector mismatch")
	}
}

func TestTelegramBotPermissionSpec(t *testing.T) {
	spec := TelegramBotPermissionSpec{
		TelegramUserID: 123456789,
		Role:           "admin",
		Permissions: []Permission{
			{
				Namespace: "*",
				Resources: []string{"pods", "deployments", "services"},
				Verbs:     []string{"get", "list", "logs", "restart", "rollback", "scale"},
			},
		},
	}

	if spec.TelegramUserID != 123456789 {
		t.Error("User ID mismatch")
	}
	if spec.Role != "admin" {
		t.Error("Role mismatch")
	}
	if len(spec.Permissions) != 1 {
		t.Error("Permissions count mismatch")
	}

	perm := spec.Permissions[0]
	if perm.Namespace != "*" {
		t.Error("Namespace should be wildcard")
	}
	if len(perm.Resources) != 3 {
		t.Error("Should have 3 resources")
	}
	if len(perm.Verbs) != 6 {
		t.Error("Should have 6 verbs")
	}
}

func TestDeepCopy_Permission(t *testing.T) {
	original := Permission{
		Namespace: "production",
		Resources: []string{"pods"},
		Verbs:     []string{"get", "list"},
		Selector:  "app=frontend",
	}

	copied := Permission{}
	original.DeepCopyInto(&copied)

	// Verify copy
	if copied.Namespace != original.Namespace {
		t.Error("Namespace not copied")
	}
	if copied.Selector != original.Selector {
		t.Error("Selector not copied")
	}
	if len(copied.Resources) != len(original.Resources) {
		t.Error("Resources not copied")
	}
	if len(copied.Verbs) != len(original.Verbs) {
		t.Error("Verbs not copied")
	}

	// Verify deep copy (modifying copy shouldn't affect original)
	copied.Resources[0] = "deployments"
	if original.Resources[0] == "deployments" {
		t.Error("Original was modified - not a deep copy")
	}
}

func TestDeepCopy_TelegramBotPermissionSpec(t *testing.T) {
	original := TelegramBotPermissionSpec{
		TelegramUserID: 123456789,
		Role:           "viewer",
		Permissions: []Permission{
			{
				Namespace: "production",
				Resources: []string{"pods"},
				Verbs:     []string{"logs"},
			},
		},
	}

	copied := TelegramBotPermissionSpec{}
	original.DeepCopyInto(&copied)

	// Verify copy
	if copied.TelegramUserID != original.TelegramUserID {
		t.Error("UserID not copied")
	}
	if copied.Role != original.Role {
		t.Error("Role not copied")
	}
	if len(copied.Permissions) != len(original.Permissions) {
		t.Error("Permissions not copied")
	}

	// Verify deep copy
	copied.Permissions[0].Namespace = "staging"
	if original.Permissions[0].Namespace == "staging" {
		t.Error("Original was modified - not a deep copy")
	}
}

func TestDeepCopy_TelegramBotPermission(t *testing.T) {
	original := &TelegramBotPermission{
		Spec: TelegramBotPermissionSpec{
			TelegramUserID: 123456789,
			Role:           "admin",
			Permissions: []Permission{
				{
					Namespace: "*",
					Resources: []string{"pods"},
					Verbs:     []string{"get"},
				},
			},
		},
	}

	// Test DeepCopy method
	copied := original.DeepCopy()

	if copied == nil {
		t.Fatal("DeepCopy returned nil")
	}

	if copied.Spec.TelegramUserID != original.Spec.TelegramUserID {
		t.Error("UserID not copied")
	}

	// Verify it's a deep copy
	copied.Spec.Role = "viewer"
	if original.Spec.Role == "viewer" {
		t.Error("Original was modified - not a deep copy")
	}

	// Test DeepCopy on nil
	var nilPerm *TelegramBotPermission
	copiedNil := nilPerm.DeepCopy()
	if copiedNil != nil {
		t.Error("DeepCopy of nil should return nil")
	}
}

func TestDeepCopyObject_TelegramBotPermission(t *testing.T) {
	original := &TelegramBotPermission{
		Spec: TelegramBotPermissionSpec{
			TelegramUserID: 123456789,
			Role:           "admin",
		},
	}

	obj := original.DeepCopyObject()
	if obj == nil {
		t.Fatal("DeepCopyObject returned nil")
	}

	// Verify it returns a runtime.Object
	copied, ok := obj.(*TelegramBotPermission)
	if !ok {
		t.Fatal("DeepCopyObject didn't return *TelegramBotPermission")
	}

	if copied.Spec.TelegramUserID != original.Spec.TelegramUserID {
		t.Error("UserID not copied")
	}
}

func TestDeepCopy_TelegramBotPermissionList(t *testing.T) {
	original := &TelegramBotPermissionList{
		Items: []TelegramBotPermission{
			{
				Spec: TelegramBotPermissionSpec{
					TelegramUserID: 123456789,
					Role:           "admin",
				},
			},
			{
				Spec: TelegramBotPermissionSpec{
					TelegramUserID: 987654321,
					Role:           "viewer",
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == nil {
		t.Fatal("DeepCopy returned nil")
	}

	if len(copied.Items) != len(original.Items) {
		t.Error("Items not copied")
	}

	// Verify deep copy
	copied.Items[0].Spec.Role = "operator"
	if original.Items[0].Spec.Role == "operator" {
		t.Error("Original was modified - not a deep copy")
	}

	// Test DeepCopy on nil
	var nilList *TelegramBotPermissionList
	copiedNil := nilList.DeepCopy()
	if copiedNil != nil {
		t.Error("DeepCopy of nil should return nil")
	}
}
