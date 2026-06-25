package rbac

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestSpecRoundTripsRoleBindings(t *testing.T) {
	in := TelegramBotPermission{
		Spec: TelegramBotPermissionSpec{
			TelegramUserID: 42,
			RoleBindings: []RoleBinding{
				{Role: "viewer", Namespace: "prod", Selector: "app=web"},
				{Role: "operator", Namespace: "*"},
			},
		},
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&in)
	if err != nil {
		t.Fatalf("to unstructured: %v", err)
	}
	// Empty legacy role must be omitted entirely (CRD enum rejects "").
	if _, ok := obj["spec"].(map[string]interface{})["role"]; ok {
		t.Fatalf("expected empty role to be omitted, got it present")
	}

	var out TelegramBotPermission
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &out); err != nil {
		t.Fatalf("from unstructured: %v", err)
	}
	if len(out.Spec.RoleBindings) != 2 || out.Spec.RoleBindings[0].Selector != "app=web" {
		t.Fatalf("roleBindings did not round-trip: %+v", out.Spec.RoleBindings)
	}
}
