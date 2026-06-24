package rbac

import (
	"context"
	"testing"

	"kubectl-bot/internal/config"
	"kubectl-bot/internal/k8s"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func tbpObject(userID int64, role, selector string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kbot.go.mamad.dev/v1",
		"kind":       "TelegramBotPermission",
		"metadata":   map[string]interface{}{"name": "user-123"},
		"spec": map[string]interface{}{
			"telegramUserId": userID,
			"role":           role,
			"permissions": []interface{}{
				map[string]interface{}{
					"namespace": "prod",
					"resources": []interface{}{"pods"},
					"verbs":     []interface{}{"list", "logs"},
					"selector":  selector,
				},
			},
		},
	}}
}

func newTestValidator(t *testing.T, obj *unstructured.Unstructured) *Validator {
	t.Helper()
	scheme := runtime.NewScheme()
	gvr := k8s.TelegramBotPermissionGVR()
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{gvr: "TelegramBotPermissionList"},
		obj,
	)
	client := k8s.NewClientWithInterfaces(nil, dc)
	cfg := &config.Config{AdminTelegramIDs: []int64{}}
	return NewValidator(NewManager(client, cfg), client)
}

// TestSelectorReturnedForList is the regression test for S1: a viewer
// restricted by selector "app=frontend" must have that selector handed back so
// list queries are scoped, instead of being dropped (which leaked all pods).
func TestSelectorReturnedForList(t *testing.T) {
	v := newTestValidator(t, tbpObject(123, "viewer", "app=frontend"))

	dec, err := v.CheckPermission(context.Background(), PermissionCheck{
		TelegramUserID: 123,
		Namespace:      "prod",
		Resource:       "pods",
		Verb:           "list",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("expected allowed")
	}
	if dec.EffectiveSelector != "app=frontend" {
		t.Fatalf("expected effective selector app=frontend, got %q", dec.EffectiveSelector)
	}
}

func TestDeniedWhenNoMatchingVerb(t *testing.T) {
	v := newTestValidator(t, tbpObject(123, "viewer", ""))

	dec, _ := v.CheckPermission(context.Background(), PermissionCheck{
		TelegramUserID: 123,
		Namespace:      "prod",
		Resource:       "pods",
		Verb:           "scale", // not granted
	})
	if dec.Allowed {
		t.Fatalf("expected denial for ungranted verb")
	}
}
