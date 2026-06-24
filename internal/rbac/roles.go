package rbac

import "fmt"

// roleCatalog maps a role preset to the verbs it grants. Resources are always
// the full set. Verbs MUST stay within the CRD verb enum.
// admin is handled separately (cluster-wide, unrestricted) and is not listed.
var roleCatalog = map[string][]string{
	"viewer":   {"get", "list", "logs"},
	"operator": {"get", "list", "logs", "restart", "rollback", "scale"},
}

// roleResources is the resource set every preset spans.
var roleResources = []string{"pods", "deployments", "services"}

// roleBindingRoles is the set of roles accepted by /role.
var roleBindingRoles = map[string]bool{"admin": true, "operator": true, "viewer": true}

// roleVerbs returns the verbs a non-admin preset grants.
func roleVerbs(role string) ([]string, bool) {
	v, ok := roleCatalog[role]
	return v, ok
}

// expandRoleBinding turns a viewer/operator binding into one Permission bundle.
// admin and unknown roles return nil (admin is short-circuited in decide).
func expandRoleBinding(rb RoleBinding) []Permission {
	verbs, ok := roleVerbs(rb.Role)
	if !ok {
		return nil
	}
	return []Permission{{
		Namespace: rb.Namespace,
		Resources: roleResources,
		Verbs:     verbs,
		Selector:  rb.Selector,
	}}
}

// effectiveRoleBindings returns the spec's bindings plus a synthesized binding
// for any legacy flat role, so old CRs keep working unchanged.
func effectiveRoleBindings(spec TelegramBotPermissionSpec) []RoleBinding {
	out := make([]RoleBinding, 0, len(spec.RoleBindings)+1)
	out = append(out, spec.RoleBindings...)
	if spec.Role != "" {
		out = append(out, RoleBinding{Role: spec.Role, Namespace: "*"})
	}
	return out
}

// effectiveRules flattens bindings (expanded) and raw permissions into one list.
func effectiveRules(spec TelegramBotPermissionSpec) []Permission {
	bindings := effectiveRoleBindings(spec)
	rules := make([]Permission, 0, len(bindings)+len(spec.Permissions))
	for _, b := range bindings {
		rules = append(rules, expandRoleBinding(b)...)
	}
	rules = append(rules, spec.Permissions...)
	return rules
}

// hasAdminBinding reports whether any effective binding grants cluster admin.
func hasAdminBinding(spec TelegramBotPermissionSpec) bool {
	for _, b := range effectiveRoleBindings(spec) {
		if b.Role == "admin" {
			return true
		}
	}
	return false
}

// decide is the pure permission core: no Kubernetes calls. It returns whether
// the check is allowed and, for selector-restricted grants, the selector that
// list/get queries must apply. A matching rule with an empty selector makes the
// access unrestricted (broad grants are not shadowed by narrow ones).
func decide(spec TelegramBotPermissionSpec, check PermissionCheck) Decision {
	if hasAdminBinding(spec) {
		return Decision{Allowed: true}
	}

	matched := false
	selector := ""
	unrestricted := false
	for _, r := range effectiveRules(spec) {
		if !matchesNamespace(r.Namespace, check.Namespace) {
			continue
		}
		if !contains(r.Resources, check.Resource) || !contains(r.Verbs, check.Verb) {
			continue
		}
		matched = true
		if r.Selector == "" {
			unrestricted = true
		} else if selector == "" {
			selector = r.Selector
		}
	}

	if !matched {
		return Decision{Reason: fmt.Sprintf("missing '%s' access to %s in namespace '%s'",
			check.Verb, check.Resource, check.Namespace)}
	}
	if unrestricted {
		return Decision{Allowed: true}
	}
	return Decision{Allowed: true, EffectiveSelector: selector}
}

// collectNamespaces returns the union of namespaces the spec references and
// whether any rule uses the * wildcard (meaning all namespaces).
func collectNamespaces(spec TelegramBotPermissionSpec) (namespaces []string, wildcard bool) {
	set := map[string]bool{}
	add := func(ns string) {
		if ns == "*" {
			wildcard = true
			return
		}
		set[ns] = true
	}
	for _, b := range effectiveRoleBindings(spec) {
		add(b.Namespace)
	}
	for _, p := range spec.Permissions {
		add(p.Namespace)
	}
	for ns := range set {
		namespaces = append(namespaces, ns)
	}
	return namespaces, wildcard
}

// upsertRoleBinding replaces the binding for rb.Namespace, or appends it.
func upsertRoleBinding(bindings []RoleBinding, rb RoleBinding) []RoleBinding {
	for i := range bindings {
		if bindings[i].Namespace == rb.Namespace {
			bindings[i] = rb
			return bindings
		}
	}
	return append(bindings, rb)
}

// removeRoleBinding drops every binding for the given namespace.
func removeRoleBinding(bindings []RoleBinding, namespace string) []RoleBinding {
	out := make([]RoleBinding, 0, len(bindings))
	for _, b := range bindings {
		if b.Namespace != namespace {
			out = append(out, b)
		}
	}
	return out
}
