package rbac

import (
	"sort"
	"testing"
)

func dec(spec TelegramBotPermissionSpec, ns, res, verb string) Decision {
	return decide(spec, PermissionCheck{Namespace: ns, Resource: res, Verb: verb})
}

func TestViewerBindingAllowsReadsDeniesWrites(t *testing.T) {
	spec := TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "viewer", Namespace: "prod"}},
	}
	if !dec(spec, "prod", "pods", "list").Allowed {
		t.Error("viewer should list pods in prod")
	}
	if !dec(spec, "prod", "deployments", "get").Allowed {
		t.Error("viewer should get deployments in prod")
	}
	if dec(spec, "prod", "deployments", "restart").Allowed {
		t.Error("viewer must NOT restart")
	}
	if dec(spec, "staging", "pods", "list").Allowed {
		t.Error("viewer in prod must NOT see staging")
	}
}

func TestOperatorBindingAllowsWrites(t *testing.T) {
	spec := TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "operator", Namespace: "*"}},
	}
	for _, v := range []string{"list", "logs", "restart", "rollback", "scale"} {
		if !dec(spec, "anything", "deployments", v).Allowed {
			t.Errorf("operator should be allowed verb %q", v)
		}
	}
}

func TestAdminBindingAllowsEverythingUnrestricted(t *testing.T) {
	spec := TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "admin", Namespace: "*"}},
	}
	d := dec(spec, "kube-system", "services", "scale")
	if !d.Allowed || d.EffectiveSelector != "" {
		t.Errorf("admin should be allowed and unrestricted, got %+v", d)
	}
}

func TestLegacyRoleNormalizesToBinding(t *testing.T) {
	spec := TelegramBotPermissionSpec{Role: "viewer"}
	if !dec(spec, "prod", "pods", "list").Allowed {
		t.Error("legacy viewer role should behave like viewer binding on *")
	}
	if dec(spec, "prod", "pods", "scale").Allowed {
		t.Error("legacy viewer must not write")
	}
}

func TestBindingSelectorFlowsToDecision(t *testing.T) {
	spec := TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "viewer", Namespace: "prod", Selector: "app=web"}},
	}
	d := dec(spec, "prod", "pods", "list")
	if !d.Allowed || d.EffectiveSelector != "app=web" {
		t.Errorf("expected selector app=web, got %+v", d)
	}
}

func TestEmptySelectorMatchWinsOverSelectorMatch(t *testing.T) {
	// A broad grant (no selector) plus a narrow one must NOT restrict the user.
	spec := TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "viewer", Namespace: "prod"}},
		Permissions: []Permission{
			{Namespace: "prod", Resources: []string{"pods"}, Verbs: []string{"list"}, Selector: "app=web"},
		},
	}
	d := dec(spec, "prod", "pods", "list")
	if !d.Allowed || d.EffectiveSelector != "" {
		t.Errorf("broad grant should make access unrestricted, got %+v", d)
	}
}

func TestDecideDeniesWithReason(t *testing.T) {
	d := dec(TelegramBotPermissionSpec{}, "prod", "pods", "list")
	if d.Allowed || d.Reason == "" {
		t.Errorf("empty spec should deny with a reason, got %+v", d)
	}
}

func TestPermissionsStillHonored(t *testing.T) {
	spec := TelegramBotPermissionSpec{
		Permissions: []Permission{
			{Namespace: "prod", Resources: []string{"pods"}, Verbs: []string{"logs"}},
		},
	}
	if !dec(spec, "prod", "pods", "logs").Allowed {
		t.Error("fine-grained permission should still grant access")
	}
}

func TestCollectNamespaces(t *testing.T) {
	spec := TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "viewer", Namespace: "prod"}},
		Permissions:  []Permission{{Namespace: "staging", Resources: []string{"pods"}, Verbs: []string{"list"}}},
	}
	ns, wild := collectNamespaces(spec)
	if wild {
		t.Fatal("no wildcard expected")
	}
	sort.Strings(ns)
	if len(ns) != 2 || ns[0] != "prod" || ns[1] != "staging" {
		t.Fatalf("expected [prod staging], got %v", ns)
	}

	if _, wild := collectNamespaces(TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "operator", Namespace: "*"}},
	}); !wild {
		t.Fatal("expected wildcard for * binding")
	}
}

func TestUpsertAndRemoveRoleBinding(t *testing.T) {
	bs := []RoleBinding{{Role: "viewer", Namespace: "prod"}}
	bs = upsertRoleBinding(bs, RoleBinding{Role: "operator", Namespace: "prod"})
	if len(bs) != 1 || bs[0].Role != "operator" {
		t.Fatalf("upsert should replace same-ns binding, got %+v", bs)
	}
	bs = upsertRoleBinding(bs, RoleBinding{Role: "viewer", Namespace: "staging"})
	if len(bs) != 2 {
		t.Fatalf("upsert should append new-ns binding, got %+v", bs)
	}
	bs = removeRoleBinding(bs, "prod")
	if len(bs) != 1 || bs[0].Namespace != "staging" {
		t.Fatalf("remove should drop only prod, got %+v", bs)
	}
}
