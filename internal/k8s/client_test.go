package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestTelegramBotPermissionGVR(t *testing.T) {
	gvr := TelegramBotPermissionGVR()

	if gvr.Group != "telegram.k8s.io" {
		t.Errorf("Expected group 'telegram.k8s.io', got '%s'", gvr.Group)
	}

	if gvr.Version != "v1" {
		t.Errorf("Expected version 'v1', got '%s'", gvr.Version)
	}

	if gvr.Resource != "telegrambotpermissions" {
		t.Errorf("Expected resource 'telegrambotpermissions', got '%s'", gvr.Resource)
	}
}

func TestTelegramBotPermissionGVR_GroupVersion(t *testing.T) {
	gvr := TelegramBotPermissionGVR()
	gv := gvr.GroupVersion()

	expectedGV := schema.GroupVersion{
		Group:   "telegram.k8s.io",
		Version: "v1",
	}

	if gv != expectedGV {
		t.Errorf("GroupVersion() = %v, expected %v", gv, expectedGV)
	}
}

func TestTelegramBotPermissionGVR_Consistency(t *testing.T) {
	// Ensure multiple calls return the same value
	gvr1 := TelegramBotPermissionGVR()
	gvr2 := TelegramBotPermissionGVR()

	if gvr1 != gvr2 {
		t.Error("TelegramBotPermissionGVR should return consistent values")
	}
}
