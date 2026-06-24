# RBAC + UX Overhaul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `viewer`/`operator`/`admin` into namespace-scopable role presets stored as `roleBindings`, make the bot self-describing per user, and audit all permission changes.

**Architecture:** Add a `roleBindings[]` field to the `TelegramBotPermission` CR. A pure role catalog expands a binding into the same `Permission` (namespace × resources × verbs × selector) bundles the validator already understands, so the decision core stays one code path. Legacy flat `role` is normalized into an equivalent binding on read. Bot handlers gain per-user help, an enriched `/start`, a selector-aware footer, and K8s audit events for grant/revoke/role.

**Tech Stack:** Go, `k8s.io/client-go` (dynamic + unstructured), `go-telegram-bot-api/v5`. Module path `kubectl-bot`. Tests use the standard `testing` package.

## Global Constraints

- Module import path is `kubectl-bot` (e.g. `kubectl-bot/internal/rbac`).
- Telegram HTML output: every dynamic string goes through `htmlEscape`/`code`/`bold`/`pre` (in `internal/bot/format.go`). Never interpolate raw user/cluster strings into HTML.
- Role catalog uses ONLY verbs in the CRD enum: `get, list, logs, restart, rollback, scale`. There is no `describe`/`events`/`top` verb — `/describe` checks `get`, `/events` and `/top` check `list`.
- Resources are exactly `pods, deployments, services`.
- The wildcard `*` matches any namespace/resource/verb via the existing `matchesNamespace`/`contains` helpers in `internal/rbac/validator.go`.
- Audit events must never fail the operation: log and continue on event-record error (mirror `internal/bot/confirm.go:188-192`).
- Run `go build ./...`, `go vet ./...`, and `go test ./...` green before every commit.

---

## Task 1: CR type + CRD schema for roleBindings

**Files:**
- Modify: `internal/rbac/types.go` (Spec struct ~17-21, Permission ~24-29, DeepCopy ~62-86)
- Modify: `manifests/crd.yaml` (required ~24-26, properties ~27-60)
- Test: `internal/rbac/types_test.go` (create)

**Interfaces:**
- Produces: `rbac.RoleBinding{Role string; Namespace string; Selector string}` and field `TelegramBotPermissionSpec.RoleBindings []RoleBinding`.
- Produces: `Spec.Role` becomes JSON `role,omitempty` (so an empty role is dropped and passes the CRD enum).

- [ ] **Step 1: Write the failing test**

Create `internal/rbac/types_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/rbac/ -run TestSpecRoundTripsRoleBindings -v`
Expected: FAIL — `RoleBinding` undefined / `RoleBindings` undefined.

- [ ] **Step 3: Implement the type changes**

In `internal/rbac/types.go`, replace the Spec struct (lines 16-29) with:

```go
// TelegramBotPermissionSpec defines the desired state of TelegramBotPermission
type TelegramBotPermissionSpec struct {
	TelegramUserID int64         `json:"telegramUserId"`
	Role           string        `json:"role,omitempty"` // legacy; normalized into a RoleBinding on read
	RoleBindings   []RoleBinding `json:"roleBindings,omitempty"`
	Permissions    []Permission  `json:"permissions,omitempty"`
}

// RoleBinding grants a named role (viewer/operator/admin) within a namespace.
// A binding expands into concrete Permission bundles via the role catalog.
type RoleBinding struct {
	Role      string `json:"role"`
	Namespace string `json:"namespace"`
	Selector  string `json:"selector,omitempty"`
}

// Permission defines granular access control
type Permission struct {
	Namespace string   `json:"namespace"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
	Selector  string   `json:"selector,omitempty"`
}
```

In the same file, update `TelegramBotPermissionSpec.DeepCopyInto` (currently lines 62-71) to also copy `RoleBindings`:

```go
// DeepCopyInto copies spec
func (in *TelegramBotPermissionSpec) DeepCopyInto(out *TelegramBotPermissionSpec) {
	*out = *in
	if in.RoleBindings != nil {
		out.RoleBindings = make([]RoleBinding, len(in.RoleBindings))
		copy(out.RoleBindings, in.RoleBindings)
	}
	if in.Permissions != nil {
		in, out := &in.Permissions, &out.Permissions
		*out = make([]Permission, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}
```

(`RoleBinding` holds only value-type strings, so `copy` is a full deep copy.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/rbac/ -run TestSpecRoundTripsRoleBindings -v`
Expected: PASS

- [ ] **Step 5: Update the CRD schema**

In `manifests/crd.yaml`, change `required` (lines 24-26) to drop `role`:

```yaml
              required:
                - telegramUserId
```

Then add a `roleBindings` property alongside `role`/`permissions` (insert after the `role` property block, before `permissions` at line 36):

```yaml
                roleBindings:
                  type: array
                  description: "Named role grants, optionally scoped to a namespace"
                  items:
                    type: object
                    required:
                      - role
                      - namespace
                    properties:
                      role:
                        type: string
                        enum: ["admin", "operator", "viewer"]
                      namespace:
                        type: string
                        description: "Kubernetes namespace (* for all)"
                      selector:
                        type: string
                        description: "Label selector (e.g., app=frontend)"
```

- [ ] **Step 6: Validate the CRD YAML parses**

Run: `go run sigs.k8s.io/yaml 2>/dev/null; python3 -c "import yaml,sys; yaml.safe_load(open('manifests/crd.yaml'))" && echo OK`
Expected: `OK` (YAML is well-formed). If `kubectl` is available against a cluster, optionally: `kubectl apply --dry-run=server -f manifests/crd.yaml`.

- [ ] **Step 7: Commit**

```bash
git add internal/rbac/types.go internal/rbac/types_test.go manifests/crd.yaml
git commit -m "feat(rbac): add roleBindings to CR type and CRD schema"
```

---

## Task 2: Role catalog and pure decision core

**Files:**
- Create: `internal/rbac/roles.go`
- Test: `internal/rbac/roles_test.go`

**Interfaces:**
- Consumes: `TelegramBotPermissionSpec`, `RoleBinding`, `Permission`, `PermissionCheck`, `Decision` (Task 1 + existing `validator.go`), and existing helpers `matchesNamespace`, `contains` (in `validator.go`).
- Produces:
  - `func roleVerbs(role string) ([]string, bool)` — catalog lookup.
  - `func expandRoleBinding(rb RoleBinding) []Permission` — viewer/operator → bundles; admin/unknown → nil.
  - `func effectiveRoleBindings(spec TelegramBotPermissionSpec) []RoleBinding` — spec bindings plus a synthesized binding for a legacy `spec.Role`.
  - `func decide(spec TelegramBotPermissionSpec, check PermissionCheck) Decision` — pure allow/deny + `EffectiveSelector`, NO Kubernetes calls.
  - `func collectNamespaces(spec TelegramBotPermissionSpec) (namespaces []string, wildcard bool)` — union of namespaces across bindings+permissions; `wildcard` true if any `*`.
  - `func upsertRoleBinding(bindings []RoleBinding, rb RoleBinding) []RoleBinding` — replace same-namespace binding or append.
  - `func removeRoleBinding(bindings []RoleBinding, namespace string) []RoleBinding` — drop binding(s) for a namespace.
  - `var roleBindingRoles = map[string]bool{"admin":true,"operator":true,"viewer":true}`

- [ ] **Step 1: Write the failing tests**

Create `internal/rbac/roles_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/rbac/ -run 'Viewer|Operator|Admin|Legacy|Selector|Decide|Permissions|Collect|Upsert' -v`
Expected: FAIL — `decide`/`collectNamespaces`/etc. undefined.

- [ ] **Step 3: Implement `internal/rbac/roles.go`**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/rbac/ -v`
Expected: PASS (all roles_test + types_test).

- [ ] **Step 5: Commit**

```bash
git add internal/rbac/roles.go internal/rbac/roles_test.go
git commit -m "feat(rbac): role catalog, binding expansion, pure decision core"
```

---

## Task 3: Wire decision core into the validator

**Files:**
- Modify: `internal/rbac/validator.go` (`CheckPermission` 46-93; `ValidateAndGetNamespaces` 129-189)
- Test: `internal/rbac/roles_test.go` (add namespace-helper test if not covered) — core already covered by Task 2.

**Interfaces:**
- Consumes: `decide`, `collectNamespaces`, `hasAdminBinding` (Task 2).
- Produces: `CheckPermission` and `ValidateAndGetNamespaces` honoring roleBindings; behavior of `Decision`/selector enforcement unchanged for callers.

- [ ] **Step 1: Replace the body of `CheckPermission`**

In `internal/rbac/validator.go`, replace lines 58-92 (everything after the `GetUserPermission` block, i.e. from the `// Admin role` comment through the final deny `return`) with:

```go
	dec := decide(permission.Spec, check)
	if !dec.Allowed {
		return Decision{Reason: fmt.Sprintf("Permission denied: %s", dec.Reason)}, nil
	}

	// For an operation on a specific named resource with a selector restriction,
	// verify the resource actually matches (prevents privilege escalation).
	if dec.EffectiveSelector != "" && check.ResourceName != "" {
		matches, err := v.validateSelector(ctx, check.Namespace, check.Resource, check.ResourceName, dec.EffectiveSelector)
		if err != nil {
			return Decision{Reason: "Failed to validate label selector"}, err
		}
		if !matches {
			return Decision{Reason: fmt.Sprintf("Resource %q does not match required selector %q", check.ResourceName, dec.EffectiveSelector)}, nil
		}
	}

	return dec, nil
```

The bootstrap-admin early return (lines 47-50) and the `GetUserPermission` block (lines 52-56) stay as-is. The standalone `permission.Spec.Role == "admin"` check is now handled inside `decide` (via `hasAdminBinding`), so it is removed by this replacement.

- [ ] **Step 2: Replace the per-permission loop in `ValidateAndGetNamespaces`**

In the same file, replace lines 150-188 (from `// Admin role can access all namespaces` through the final `return namespaces, nil`) with:

```go
	listAll := func() ([]string, error) {
		nsList, err := v.k8sClient.ListNamespaces(ctx)
		if err != nil {
			return nil, err
		}
		namespaces := make([]string, 0, len(nsList.Items))
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
		return namespaces, nil
	}

	if hasAdminBinding(permission.Spec) {
		return listAll()
	}

	namespaces, wildcard := collectNamespaces(permission.Spec)
	if wildcard {
		return listAll()
	}
	return namespaces, nil
```

This removes the now-duplicated `nsList` loops; keep the bootstrap-admin block at lines 130-142 (it still calls `ListNamespaces` directly — leave it, or refactor to a local `listAll` later; not required).

- [ ] **Step 3: Run build, vet, and the rbac tests**

Run: `go build ./... && go vet ./... && go test ./internal/rbac/ -v`
Expected: build OK, vet clean, tests PASS. If `fmt` becomes unused anywhere, fix imports.

- [ ] **Step 4: Commit**

```bash
git add internal/rbac/validator.go
git commit -m "feat(rbac): validator honors roleBindings via decision core"
```

---

## Task 4: Manager role-binding mutations and summary

**Files:**
- Modify: `internal/rbac/manager.go` (`newUserPermission` 88-104; add methods; `GetPermissionSummary` 190-216; keep `SetRole` 159-168)
- Test: `internal/rbac/manager_test.go` (create — tests the pure summary helper)

**Interfaces:**
- Consumes: `upsertRoleBinding`, `removeRoleBinding`, `roleBindingRoles`, `effectiveRoleBindings` (Task 2).
- Produces:
  - `func (m *Manager) SetRoleBinding(ctx, userID int64, role, namespace, selector string) error`
  - `func (m *Manager) RemoveRoleBinding(ctx, userID int64, namespace string) error`
  - `func (m *Manager) EffectiveRoleBindings(ctx, userID int64) ([]RoleBinding, error)` (bootstrap admin → `[{admin,*}]`)
  - `func formatPermissionSummary(spec TelegramBotPermissionSpec) string` (pure; used by `GetPermissionSummary`)

- [ ] **Step 1: Write the failing test**

Create `internal/rbac/manager_test.go`:

```go
package rbac

import (
	"strings"
	"testing"
)

func TestFormatPermissionSummaryShowsBindings(t *testing.T) {
	spec := TelegramBotPermissionSpec{
		TelegramUserID: 7,
		RoleBindings: []RoleBinding{
			{Role: "viewer", Namespace: "prod", Selector: "app=web"},
			{Role: "operator", Namespace: "*"},
		},
		Permissions: []Permission{
			{Namespace: "staging", Resources: []string{"pods"}, Verbs: []string{"logs"}},
		},
	}
	out := formatPermissionSummary(spec)
	for _, want := range []string{"viewer", "prod", "app=web", "operator", "*", "staging", "logs"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestFormatPermissionSummaryEmpty(t *testing.T) {
	out := formatPermissionSummary(TelegramBotPermissionSpec{TelegramUserID: 7})
	if !strings.Contains(out, "No permissions") {
		t.Errorf("expected empty notice, got %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/rbac/ -run TestFormatPermissionSummary -v`
Expected: FAIL — `formatPermissionSummary` undefined.

- [ ] **Step 3: Implement the manager changes**

In `internal/rbac/manager.go`:

(a) Change `newUserPermission` (lines 88-104) so a fresh user has NO implicit role (an empty role would otherwise normalize to "viewer everywhere"):

```go
// newUserPermission builds an empty permission object for a user. No role or
// bindings are set, so a freshly-created user has zero access until granted.
func newUserPermission(userID int64) *TelegramBotPermission {
	return &TelegramBotPermission{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kbot.go.mamad.dev/v1",
			Kind:       "TelegramBotPermission",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: formatUserResourceName(userID),
		},
		Spec: TelegramBotPermissionSpec{
			TelegramUserID: userID,
			RoleBindings:   []RoleBinding{},
			Permissions:    []Permission{},
		},
	}
}
```

(b) Add the new methods (place after `SetRole`, ~line 168):

```go
// SetRoleBinding upserts a role binding for a namespace ("*" for all).
func (m *Manager) SetRoleBinding(ctx context.Context, userID int64, role, namespace, selector string) error {
	if !roleBindingRoles[role] {
		return fmt.Errorf("invalid role %q (must be admin, operator, or viewer)", role)
	}
	if namespace == "" {
		namespace = "*"
	}
	return m.mutateUserPermission(ctx, userID, func(p *TelegramBotPermission) {
		p.Spec.TelegramUserID = userID
		p.Spec.RoleBindings = upsertRoleBinding(p.Spec.RoleBindings, RoleBinding{
			Role: role, Namespace: namespace, Selector: selector,
		})
	})
}

// RemoveRoleBinding removes any role binding for a namespace ("*" for the
// cluster-wide binding).
func (m *Manager) RemoveRoleBinding(ctx context.Context, userID int64, namespace string) error {
	if namespace == "" {
		namespace = "*"
	}
	return m.mutateUserPermission(ctx, userID, func(p *TelegramBotPermission) {
		p.Spec.RoleBindings = removeRoleBinding(p.Spec.RoleBindings, namespace)
	})
}

// EffectiveRoleBindings returns the user's role bindings including any legacy
// role. Bootstrap admins report a synthetic cluster-wide admin binding.
func (m *Manager) EffectiveRoleBindings(ctx context.Context, userID int64) ([]RoleBinding, error) {
	if m.IsBootstrapAdmin(userID) {
		return []RoleBinding{{Role: "admin", Namespace: "*"}}, nil
	}
	permission, err := m.GetUserPermission(ctx, userID)
	if err != nil {
		return nil, err
	}
	return effectiveRoleBindings(permission.Spec), nil
}
```

(c) Replace `GetPermissionSummary` (lines 190-216) to delegate to a pure formatter and include bindings:

```go
// GetPermissionSummary returns a formatted summary of user permissions.
func (m *Manager) GetPermissionSummary(ctx context.Context, userID int64) (string, error) {
	permission, err := m.GetUserPermission(ctx, userID)
	if err != nil {
		return "", err
	}
	return formatPermissionSummary(permission.Spec), nil
}

// formatPermissionSummary renders bindings and fine-grained permissions as text.
func formatPermissionSummary(spec TelegramBotPermissionSpec) string {
	summary := fmt.Sprintf("User ID: %d\n", spec.TelegramUserID)

	bindings := effectiveRoleBindings(spec)
	if len(bindings) == 0 && len(spec.Permissions) == 0 {
		return summary + "\nNo permissions granted"
	}

	if len(bindings) > 0 {
		summary += "\nRoles:\n"
		for _, b := range bindings {
			line := fmt.Sprintf("  • %s in %s", b.Role, b.Namespace)
			if b.Selector != "" {
				line += " (" + b.Selector + ")"
			}
			summary += line + "\n"
		}
	}

	if len(spec.Permissions) > 0 {
		summary += "\nFine-grained permissions:\n"
		for i, p := range spec.Permissions {
			summary += fmt.Sprintf("%d. Namespace: %s\n", i+1, p.Namespace)
			summary += fmt.Sprintf("   Resources: %v\n", p.Resources)
			summary += fmt.Sprintf("   Verbs: %v\n", p.Verbs)
			if p.Selector != "" {
				summary += fmt.Sprintf("   Selector: %s\n", p.Selector)
			}
		}
	}
	return summary
}
```

- [ ] **Step 4: Run tests, build, vet**

Run: `go test ./internal/rbac/ -v && go build ./... && go vet ./...`
Expected: PASS, build OK, vet clean.

- [ ] **Step 5: Commit**

```bash
git add internal/rbac/manager.go internal/rbac/manager_test.go
git commit -m "feat(rbac): SetRoleBinding/RemoveRoleBinding + binding-aware summary"
```

---

## Task 5: `/role` rewrite, `/whoami` registration, command list

**Files:**
- Modify: `internal/bot/handlers.go` (`handleRole` 620-643)
- Modify: `internal/bot/bot.go` (dispatch 129-170; `setupCommands` 222-253)

**Interfaces:**
- Consumes: `Manager.SetRoleBinding`, `Manager.RemoveRoleBinding` (Task 4).
- Produces: `/role <user> <role|none> [-n ns] [-l sel]`; `/whoami` routed to `handleStart`.

- [ ] **Step 1: Rewrite `handleRole`**

Replace `handleRole` (lines 620-643 in `internal/bot/handlers.go`) with:

```go
// handleRole sets or removes a user's role binding (admin only).
//   /role <user_id> <admin|operator|viewer> [-n ns] [-l selector]
//   /role <user_id> none [-n ns]
func (b *Bot) handleRole(ctx context.Context, message *tgbotapi.Message) {
	if !b.isAdmin(ctx, message.From.ID) {
		b.send(message.Chat.ID, "❌ Admin access required.")
		return
	}
	p := parseArgs(message.CommandArguments())
	if len(p.positional) < 2 {
		b.send(message.Chat.ID, "Usage: /role &lt;user_id&gt; &lt;admin|operator|viewer|none&gt; [-n ns] [-l selector]")
		return
	}
	targetUserID, err := strconv.ParseInt(p.positional[0], 10, 64)
	if err != nil {
		b.send(message.Chat.ID, "❌ Invalid user ID.")
		return
	}
	role := strings.ToLower(p.positional[1])
	nsLabel := p.namespace
	if nsLabel == "" {
		nsLabel = "*"
	}

	if role == "none" {
		if err := b.rbac.RemoveRoleBinding(ctx, targetUserID, p.namespace); err != nil {
			b.fail(message.Chat.ID, "Remove role", err)
			return
		}
		b.auditPermissionChange(ctx, message.From.ID, targetUserID, "PermissionRevoked",
			fmt.Sprintf("removed role in %s", nsLabel))
		b.send(message.Chat.ID, "✅ Removed role of user "+code(p.positional[0])+" in "+code(nsLabel))
		return
	}

	if err := b.rbac.SetRoleBinding(ctx, targetUserID, role, p.namespace, p.selector); err != nil {
		b.fail(message.Chat.ID, "Set role", err)
		return
	}
	b.auditPermissionChange(ctx, message.From.ID, targetUserID, "PermissionGranted",
		fmt.Sprintf("set %s in %s", role, nsLabel))
	resp := "✅ Set user " + code(p.positional[0]) + " to " + code(role) + " in " + code(nsLabel)
	if p.selector != "" {
		resp += " (" + code(p.selector) + ")"
	}
	b.send(message.Chat.ID, resp)
}
```

(`auditPermissionChange` is added in Task 6. If implementing strictly task-by-task, this task will not compile until Task 6 lands; combine Tasks 5 and 6 into one commit, OR add a temporary no-op `auditPermissionChange` here and fill it in Task 6. Recommended: implement Task 6's helper first, then this. The plan orders them 5→6 for narrative; the implementer may swap.)

- [ ] **Step 2: Register `/whoami` in dispatch**

In `internal/bot/bot.go`, add a case to the `switch message.Command()` block (after the `case "start":` at line 130-131):

```go
		case "start", "whoami":
			b.handleStart(cmdCtx, message)
```

Remove the now-duplicate standalone `case "start":` (old lines 130-131).

- [ ] **Step 3: Update `setupCommands`**

In `internal/bot/bot.go` `setupCommands` (lines 223-243), update the `/role` description and add `/whoami`:

```go
		{Command: "whoami", Description: "Show your identity, roles, and what you can do"},
```
(insert after the `start` entry) and change the `role` entry to:
```go
		{Command: "role", Description: "Set a user's role: /role <id> <viewer|operator|admin|none> [-n ns] (admin only)"},
```

- [ ] **Step 4: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: OK (assuming `auditPermissionChange` exists — see Task 6 note).

- [ ] **Step 5: Commit** (combined with Task 6 if needed)

```bash
git add internal/bot/handlers.go internal/bot/bot.go
git commit -m "feat(bot): /role manages role bindings; add /whoami"
```

---

## Task 6: Audit events for permission changes

**Files:**
- Modify: `internal/bot/confirm.go` (add `auditPermissionChange` near `audit` 187-193) or a new `internal/bot/audit.go`
- Modify: `internal/bot/handlers.go` (`handleGrant` 578-588; `handleRevoke` 613-617)

**Interfaces:**
- Consumes: `k8sClient.RecordEvent(ctx, namespace, involvedKind, involvedName, reason, message string)` (`internal/k8s/observe.go:31`); `config.BotNamespace`.
- Produces: `func (b *Bot) auditPermissionChange(ctx context.Context, adminID, targetUserID int64, reason, detail string)`.

- [ ] **Step 1: Add the audit helper**

Create `internal/bot/audit.go`:

```go
package bot

import (
	"context"
	"fmt"
	"log"
)

// auditPermissionChange records a Kubernetes Event attributing a permission
// mutation to the acting admin. Failure to record never fails the operation.
func (b *Bot) auditPermissionChange(ctx context.Context, adminID, targetUserID int64, reason, detail string) {
	name := fmt.Sprintf("user-%d", targetUserID)
	msg := fmt.Sprintf("admin %d %s for user %d", adminID, detail, targetUserID)
	if err := b.k8sClient.RecordEvent(ctx, b.config.BotNamespace, "TelegramBotPermission", name, reason, msg); err != nil {
		log.Printf("Failed to record permission audit event: %v", err)
	}
}
```

- [ ] **Step 2: Wire audit into `handleGrant`**

In `internal/bot/handlers.go` `handleGrant`, immediately after the successful `GrantPermission` call (after line 581, before building `resp`), add:

```go
	b.auditPermissionChange(ctx, message.From.ID, targetUserID, "PermissionGranted",
		fmt.Sprintf("granted %s %s in %s", verb, resource, namespace))
```

- [ ] **Step 3: Wire audit into `handleRevoke`**

In `handleRevoke`, after the successful `RevokePermission` call (after line 616, before the success `send`), add:

```go
	b.auditPermissionChange(ctx, message.From.ID, targetUserID, "PermissionRevoked",
		fmt.Sprintf("revoked %s %s in %s", verb, resource, p.namespace))
```

- [ ] **Step 4: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: OK / PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bot/audit.go internal/bot/handlers.go
git commit -m "feat(bot): audit grant/revoke/role as Kubernetes Events"
```

---

## Task 7: Per-user `/help` and enriched `/start`

**Files:**
- Modify: `internal/bot/handlers.go` (`handleStart` 105-112; `handleHelp` 114-148)
- Modify: `internal/bot/bot.go` (`hasAnyPermission` 204-219)
- Test: `internal/rbac/roles_test.go` (add a `summarizeCapabilities` test) — capability logic lives in rbac as a pure function.

**Interfaces:**
- Consumes: `Manager.EffectiveRoleBindings` (Task 4), `effectiveRules` (Task 2).
- Produces: `rbac.Capabilities{Read, Write, Admin bool}` and `func SummarizeCapabilities(spec TelegramBotPermissionSpec) Capabilities` (exported, pure, for the bot to render help).

- [ ] **Step 1: Write the failing capability test**

Add to `internal/rbac/roles_test.go`:

```go
func TestSummarizeCapabilities(t *testing.T) {
	viewer := SummarizeCapabilities(TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "viewer", Namespace: "prod"}},
	})
	if !viewer.Read || viewer.Write || viewer.Admin {
		t.Errorf("viewer caps wrong: %+v", viewer)
	}
	op := SummarizeCapabilities(TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "operator", Namespace: "*"}},
	})
	if !op.Read || !op.Write || op.Admin {
		t.Errorf("operator caps wrong: %+v", op)
	}
	adm := SummarizeCapabilities(TelegramBotPermissionSpec{
		RoleBindings: []RoleBinding{{Role: "admin", Namespace: "*"}},
	})
	if !adm.Admin {
		t.Errorf("admin caps wrong: %+v", adm)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/rbac/ -run TestSummarizeCapabilities -v`
Expected: FAIL — `SummarizeCapabilities` undefined.

- [ ] **Step 3: Implement `SummarizeCapabilities` in `internal/rbac/roles.go`**

```go
// Capabilities is a coarse summary of what a user can do, used to tailor help.
type Capabilities struct {
	Read  bool
	Write bool
	Admin bool
}

// writeVerbs are the mutating verbs.
var writeVerbs = map[string]bool{"restart": true, "rollback": true, "scale": true}

// SummarizeCapabilities derives coarse read/write/admin flags from a spec.
func SummarizeCapabilities(spec TelegramBotPermissionSpec) Capabilities {
	if hasAdminBinding(spec) {
		return Capabilities{Read: true, Write: true, Admin: true}
	}
	caps := Capabilities{}
	for _, r := range effectiveRules(spec) {
		for _, v := range r.Verbs {
			if writeVerbs[v] {
				caps.Write = true
			} else {
				caps.Read = true
			}
		}
	}
	return caps
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/rbac/ -run TestSummarizeCapabilities -v`
Expected: PASS

- [ ] **Step 5: Add a Bot capability helper and rewrite `handleStart`**

In `internal/bot/handlers.go`, replace `handleStart` (lines 105-112) with:

```go
// handleStart / handleWhoami greets the user and shows their roles + abilities.
func (b *Bot) handleStart(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	var sb strings.Builder
	sb.WriteString("👋 <b>Kubernetes Bot</b>\n\n")
	sb.WriteString("User ID: " + code(strconv.FormatInt(userID, 10)) + "\n")

	bindings, err := b.rbac.EffectiveRoleBindings(ctx, userID)
	if err != nil || len(bindings) == 0 {
		sb.WriteString("\nYou have no access yet. Ask an admin to grant you a role.\n")
		b.send(message.Chat.ID, sb.String())
		return
	}

	sb.WriteString("\n<b>Roles:</b>\n")
	for _, rb := range bindings {
		line := "• " + htmlEscape(rb.Role) + " in " + code(rb.Namespace)
		if rb.Selector != "" {
			line += " (" + code(rb.Selector) + ")"
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\nType /help for the commands you can run.")
	b.send(message.Chat.ID, sb.String())
}
```

- [ ] **Step 6: Rewrite `handleHelp` to be per-user**

Replace `handleHelp` (lines 114-148) with:

```go
// handleHelp shows only the command groups the caller can use.
func (b *Bot) handleHelp(ctx context.Context, message *tgbotapi.Message) {
	caps := b.userCapabilities(ctx, message.From.ID)

	var sb strings.Builder
	sb.WriteString("<b>Available Commands</b>\n\n")
	sb.WriteString("<b>General:</b>\n/start, /whoami — your identity and roles\n/help — this message\n/namespaces — accessible namespaces\n/permissions — your permissions\n")

	if caps.Read {
		sb.WriteString("\n<b>Resource Queries:</b>\n")
		sb.WriteString("/pods [ns] [-l selector]\n/deployments [ns] [-l selector]\n/services [ns]\n")
		sb.WriteString("/describe &lt;pod|deployment&gt; &lt;name&gt; [-n ns]\n/events [ns]\n/top [ns]\n")
		sb.WriteString("/logs &lt;pod&gt; [-n ns] [-c container] [--tail N] [--previous] [--since secs]\n")
	}
	if caps.Write {
		sb.WriteString("\n<b>Operations (confirmed):</b>\n")
		sb.WriteString("/restart &lt;deployment&gt; [-n ns]\n/rollback &lt;deployment&gt; [-n ns]\n/scale &lt;deployment&gt; &lt;replicas&gt; [-n ns]\n")
	}
	if caps.Admin {
		sb.WriteString("\n<b>Admin Commands:</b>\n")
		sb.WriteString("/role &lt;user_id&gt; &lt;viewer|operator|admin|none&gt; [-n ns] [-l selector]\n")
		sb.WriteString("/grant &lt;user_id&gt; &lt;verb&gt; &lt;resource&gt; [-n ns] [-l selector]\n")
		sb.WriteString("/revoke &lt;user_id&gt; &lt;verb&gt; &lt;resource&gt; -n ns\n")
		sb.WriteString("/permissions [user_id]\n/users\n/selfupdate\n")
	}
	if !caps.Read && !caps.Write && !caps.Admin {
		sb.WriteString("\nYou have no access yet. Ask an admin to grant you a role.")
	}
	b.send(message.Chat.ID, sb.String())
}

// userCapabilities returns coarse read/write/admin flags for a user.
func (b *Bot) userCapabilities(ctx context.Context, userID int64) rbac.Capabilities {
	if b.rbac.IsBootstrapAdmin(userID) {
		return rbac.Capabilities{Read: true, Write: true, Admin: true}
	}
	permission, err := b.rbac.GetUserPermission(ctx, userID)
	if err != nil {
		return rbac.Capabilities{}
	}
	return rbac.SummarizeCapabilities(permission.Spec)
}
```

- [ ] **Step 7: Update `hasAnyPermission` to count role bindings**

In `internal/bot/bot.go`, replace the final return of `hasAnyPermission` (line 218) with:

```go
	return permission.Spec.Role != "" ||
		len(permission.Spec.RoleBindings) > 0 ||
		len(permission.Spec.Permissions) > 0
```

- [ ] **Step 8: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: OK / PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/rbac/roles.go internal/rbac/roles_test.go internal/bot/handlers.go internal/bot/bot.go
git commit -m "feat(bot): per-user /help and role-aware /start + /whoami"
```

---

## Task 8: Selector-restriction footer on list output

**Files:**
- Modify: `internal/bot/handlers.go` (`resolveListAccess` 171-185; `handlePods` 188-216; `handleDeployments` 230-262; `handleServices` 264-292; `handleEvents` 420; `handleTop` 458)

**Interfaces:**
- Consumes: `Decision.EffectiveSelector` (existing).
- Produces: `resolveListAccess` returns `(combined string, restriction string, ok bool)` where `restriction` is the permission-imposed selector (empty if unrestricted), for rendering a footer.

- [ ] **Step 1: Change `resolveListAccess` signature**

Replace `resolveListAccess` (lines 171-185) with:

```go
// resolveListAccess checks list permission for a resource. It returns the
// selector to apply to the query, the permission-imposed restriction (for
// display, empty if unrestricted), and ok=false if denied.
func (b *Bot) resolveListAccess(ctx context.Context, chatID, userID int64, namespace, resource, userSelector string) (combined string, restriction string, ok bool) {
	dec, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: userID,
		Namespace:      namespace,
		Resource:       resource,
		Verb:           "list",
	})
	if err != nil || !dec.Allowed {
		b.send(chatID, rbac.FormatPermissionDenied(dec.Reason))
		return "", "", false
	}
	return combineSelectors(dec.EffectiveSelector, userSelector), dec.EffectiveSelector, true
}

// selectorFooter renders a restriction note, or "" when unrestricted.
func selectorFooter(restriction string) string {
	if restriction == "" {
		return ""
	}
	return "\n🔒 filtered to " + code(restriction)
}
```

- [ ] **Step 2: Update the three list handlers**

In `handlePods` (line 192) change:
```go
	selector, ok := b.resolveListAccess(ctx, message.Chat.ID, message.From.ID, namespace, "pods", p.selector)
	if !ok {
		return
	}
```
to:
```go
	selector, restriction, ok := b.resolveListAccess(ctx, message.Chat.ID, message.From.ID, namespace, "pods", p.selector)
	if !ok {
		return
	}
```
and change the final `b.send(message.Chat.ID, sb.String())` (line 215) to:
```go
	b.send(message.Chat.ID, sb.String()+selectorFooter(restriction))
```

Apply the identical change to `handleDeployments` (lines 235, 261) with `"deployments"`, and `handleServices` (lines 269, 291) with `"services"`.

- [ ] **Step 3: Update `handleEvents` and `handleTop` call sites**

These ignore the selector. Change `handleEvents` line 420:
```go
	if _, _, ok := b.resolveListAccess(ctx, message.Chat.ID, message.From.ID, namespace, "pods", ""); !ok {
		return
	}
```
and `handleTop` line 458 identically (three return values, two discarded).

- [ ] **Step 4: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: OK / PASS. Confirm no other callers of `resolveListAccess` remain on the 2-value signature: `grep -n "resolveListAccess" internal/bot/handlers.go` — every call must use three return values.

- [ ] **Step 5: Commit**

```bash
git add internal/bot/handlers.go
git commit -m "feat(bot): show selector restriction footer on list output"
```

---

## Task 9: Docs and final verification

**Files:**
- Modify: `CLAUDE.md` (Bot Commands Reference, CRD Spec Structure sections)

- [ ] **Step 1: Update CLAUDE.md command reference**

In `CLAUDE.md`, update the `/role` line under Admin Commands to:
```
- `/role <user_id> <viewer|operator|admin|none> [-n <ns>] [-l <selector>]` - Set/remove a namespace-scoped role
```
Add under User Commands:
```
- `/whoami` - Show your identity, roles, and what you can do
```
In the CRD Spec Structure block, add `roleBindings` above `permissions`:
```yaml
  roleBindings:
    - role: viewer|operator|admin
      namespace: string        # "*" for all
      selector: string         # optional
```
Add a sentence to the RBAC Model / Permission Storage section:
```
Roles are presets expanded by a catalog (internal/rbac/roles.go): viewer = get/list/logs, operator = viewer + restart/rollback/scale, admin = cluster-wide. A legacy flat `role` field is normalized to a `*`-scoped binding on read.
```

- [ ] **Step 2: Full verification**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build OK, vet clean, all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document roleBindings, /role, and /whoami"
```

---

## Self-Review Notes

- **Spec coverage:** roleBindings model (Tasks 1-4) ✓; validator expansion (Task 3) ✓; legacy auto-migration (`effectiveRoleBindings`, Task 2, tested) ✓; `/role` one-command grant (Task 5) ✓; per-user `/help` + enriched `/start` + `/whoami` (Tasks 5, 7) ✓; selector visibility (Task 8) ✓; audit grant/revoke/role (Task 6) ✓. Non-goals (OR selectors, button wizard, confirm persistence) intentionally absent.
- **Resolved spec open-checks:** (1) verbs for describe/events/top are `get`/`list`/`list` — catalog uses real CRD verbs, no new verb strings. (2) services keep their existing selector bypass — unchanged. (3) audit namespace = `config.BotNamespace`.
- **Migration risk flagged:** a user previously set to legacy `role: viewer` now gains read access in ALL namespaces (the chosen "keep + auto-migrate" semantics). `newUserPermission` no longer defaults to `viewer`, so freshly-granted users are NOT broadened. This is intentional per the design decision.
- **Actionable-denial wording:** the design's richer denial text ("you have viewer there, ask for operator") requires knowing the user's standing at deny time. The current plan keeps the existing `decide`-reason format (`missing 'restart' access to deployments in 'prod'`) to avoid an extra fetch in the hot path; the enriched suggestion is deferred as a follow-up rather than silently dropped. Note this to the user.
- **Type consistency:** `resolveListAccess` returns 3 values everywhere (Task 8 grep gate); `auditPermissionChange` signature is identical across Tasks 5/6 (ordering note added).
