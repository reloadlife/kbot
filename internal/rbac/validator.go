package rbac

import (
	"context"
	"fmt"
	"strings"

	"kubectl-bot/internal/k8s"
)

// PermissionCheck defines a permission check request
type PermissionCheck struct {
	TelegramUserID int64
	Namespace      string
	Resource       string // "pods", "deployments", "services"
	Verb           string // "get", "list", "logs", "restart", etc.
	ResourceName   string // Specific resource name (e.g., pod name)
	Selector       string // Optional label selector
}

// Validator validates permissions
type Validator struct {
	manager   *Manager
	k8sClient *k8s.Client
}

// NewValidator creates a new permission validator
func NewValidator(manager *Manager, k8sClient *k8s.Client) *Validator {
	return &Validator{
		manager:   manager,
		k8sClient: k8sClient,
	}
}

// Decision is the result of a permission check. EffectiveSelector, when
// non-empty, MUST be applied to list/get queries so that selector-restricted
// users only ever see resources they are entitled to.
type Decision struct {
	Allowed           bool
	EffectiveSelector string
	Reason            string
}

// CheckPermission validates if a user has permission to perform an action and,
// for list operations, returns the label selector that must be applied.
func (v *Validator) CheckPermission(ctx context.Context, check PermissionCheck) (Decision, error) {
	// Bootstrap admins have all permissions, unrestricted.
	if v.manager.IsBootstrapAdmin(check.TelegramUserID) {
		return Decision{Allowed: true}, nil
	}

	// Get user's permissions from CRD
	permission, err := v.manager.GetUserPermission(ctx, check.TelegramUserID)
	if err != nil {
		return Decision{Reason: "No permissions found. Ask an admin to grant you access."}, err
	}

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
}

// validateSelector checks if a resource matches the permission's label selector
func (v *Validator) validateSelector(ctx context.Context, namespace, resource, resourceName, selector string) (bool, error) {
	switch resource {
	case "pods":
		return v.k8sClient.PodMatchesSelector(ctx, namespace, resourceName, selector)
	case "deployments":
		return v.k8sClient.DeploymentMatchesSelector(ctx, namespace, resourceName, selector)
	case "services":
		// Services don't have selector validation in current implementation
		return true, nil
	default:
		return false, fmt.Errorf("unsupported resource type: %s", resource)
	}
}

// matchesNamespace checks if a namespace matches the permission namespace
func matchesNamespace(permNamespace, requestNamespace string) bool {
	if permNamespace == "*" {
		return true
	}
	return permNamespace == requestNamespace
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item || s == "*" {
			return true
		}
	}
	return false
}

// ValidateAndGetNamespaces returns list of namespaces user has access to
func (v *Validator) ValidateAndGetNamespaces(ctx context.Context, userID int64) ([]string, error) {
	// Bootstrap admins can access all namespaces
	if v.manager.IsBootstrapAdmin(userID) {
		nsList, err := v.k8sClient.ListNamespaces(ctx)
		if err != nil {
			return nil, err
		}

		namespaces := []string{}
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
		return namespaces, nil
	}

	// Get user's permissions
	permission, err := v.manager.GetUserPermission(ctx, userID)
	if err != nil {
		return nil, err
	}

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
}

// FormatPermissionDenied formats a permission denied message
func FormatPermissionDenied(reason string) string {
	if reason == "" {
		return "Permission denied"
	}
	return fmt.Sprintf("❌ %s", reason)
}

// NormalizeNamespace returns default namespace if empty
func NormalizeNamespace(namespace string) string {
	if namespace == "" || strings.TrimSpace(namespace) == "" {
		return "default"
	}
	return strings.TrimSpace(namespace)
}
