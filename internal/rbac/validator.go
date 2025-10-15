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

// CheckPermission validates if a user has permission to perform an action
func (v *Validator) CheckPermission(ctx context.Context, check PermissionCheck) (bool, string, error) {
	// Bootstrap admins have all permissions
	if v.manager.IsBootstrapAdmin(check.TelegramUserID) {
		return true, "", nil
	}

	// Get user's permissions from CRD
	permission, err := v.manager.GetUserPermission(ctx, check.TelegramUserID)
	if err != nil {
		return false, fmt.Sprintf("No permissions found for user %d", check.TelegramUserID), err
	}

	// Admin role has all permissions
	if permission.Spec.Role == "admin" {
		return true, "", nil
	}

	// Check each permission entry
	for _, perm := range permission.Spec.Permissions {
		// Check namespace (exact match or wildcard)
		if !matchesNamespace(perm.Namespace, check.Namespace) {
			continue
		}

		// Check resource
		if !contains(perm.Resources, check.Resource) {
			continue
		}

		// Check verb
		if !contains(perm.Verbs, check.Verb) {
			continue
		}

		// If permission has a selector, validate the resource matches it
		if perm.Selector != "" && check.ResourceName != "" {
			matches, err := v.validateSelector(ctx, check.Namespace, check.Resource, check.ResourceName, perm.Selector)
			if err != nil {
				return false, fmt.Sprintf("Failed to validate selector: %v", err), err
			}
			if !matches {
				return false, fmt.Sprintf("Resource '%s' does not match required selector: %s", check.ResourceName, perm.Selector), nil
			}
		}

		// Permission granted
		return true, "", nil
	}

	// No matching permission found
	return false, fmt.Sprintf("Permission denied: missing '%s' access to %s in namespace '%s'",
		check.Verb, check.Resource, check.Namespace), nil
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

	// Admin role can access all namespaces
	if permission.Spec.Role == "admin" {
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

	// Collect unique namespaces from permissions
	nsSet := make(map[string]bool)
	for _, perm := range permission.Spec.Permissions {
		if perm.Namespace == "*" {
			// Wildcard, return all namespaces
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
		nsSet[perm.Namespace] = true
	}

	namespaces := []string{}
	for ns := range nsSet {
		namespaces = append(namespaces, ns)
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
