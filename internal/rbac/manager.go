package rbac

import (
	"context"
	"fmt"
	"strconv"

	"kubectl-bot/internal/config"
	"kubectl-bot/internal/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type Manager struct {
	k8sClient *k8s.Client
	config    *config.Config
}

func NewManager(k8sClient *k8s.Client, cfg *config.Config) *Manager {
	return &Manager{
		k8sClient: k8sClient,
		config:    cfg,
	}
}

// GetUserPermission retrieves the TelegramBotPermission for a user
func (m *Manager) GetUserPermission(ctx context.Context, userID int64) (*TelegramBotPermission, error) {
	resourceName := fmt.Sprintf("user-%d", userID)

	unstructuredObj, err := m.k8sClient.GetDynamicClient().
		Resource(k8s.TelegramBotPermissionGVR()).
		Get(ctx, resourceName, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	var permission TelegramBotPermission
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &permission)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to TelegramBotPermission: %w", err)
	}

	return &permission, nil
}

// CreateUserPermission creates a new TelegramBotPermission
func (m *Manager) CreateUserPermission(ctx context.Context, permission *TelegramBotPermission) error {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(permission)
	if err != nil {
		return fmt.Errorf("failed to convert to unstructured: %w", err)
	}

	unstructuredPermission := &unstructured.Unstructured{Object: unstructuredObj}
	unstructuredPermission.SetGroupVersionKind(k8s.TelegramBotPermissionGVR().GroupVersion().WithKind("TelegramBotPermission"))

	_, err = m.k8sClient.GetDynamicClient().
		Resource(k8s.TelegramBotPermissionGVR()).
		Create(ctx, unstructuredPermission, metav1.CreateOptions{})

	return err
}

// UpdateUserPermission updates an existing TelegramBotPermission
func (m *Manager) UpdateUserPermission(ctx context.Context, permission *TelegramBotPermission) error {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(permission)
	if err != nil {
		return fmt.Errorf("failed to convert to unstructured: %w", err)
	}

	unstructuredPermission := &unstructured.Unstructured{Object: unstructuredObj}
	unstructuredPermission.SetGroupVersionKind(k8s.TelegramBotPermissionGVR().GroupVersion().WithKind("TelegramBotPermission"))

	_, err = m.k8sClient.GetDynamicClient().
		Resource(k8s.TelegramBotPermissionGVR()).
		Update(ctx, unstructuredPermission, metav1.UpdateOptions{})

	return err
}

// GrantPermission grants a specific permission to a user
func (m *Manager) GrantPermission(ctx context.Context, userID int64, namespace, resource, verb, selector string) error {
	// Get existing permission or create new one
	permission, err := m.GetUserPermission(ctx, userID)
	if err != nil {
		// Create new permission with viewer role
		permission = &TelegramBotPermission{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "kbot.go.mamad.dev/v1",
				Kind:       "TelegramBotPermission",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("user-%d", userID),
			},
			Spec: TelegramBotPermissionSpec{
				TelegramUserID: userID,
				Role:           "viewer",
				Permissions:    []Permission{},
			},
		}
	}

	// Add new permission
	newPerm := Permission{
		Namespace: namespace,
		Resources: []string{resource},
		Verbs:     []string{verb},
		Selector:  selector,
	}

	// Check if similar permission exists and merge
	merged := false
	for i, p := range permission.Spec.Permissions {
		if p.Namespace == namespace && p.Selector == selector {
			// Merge resources and verbs
			permission.Spec.Permissions[i].Resources = mergeUnique(p.Resources, newPerm.Resources)
			permission.Spec.Permissions[i].Verbs = mergeUnique(p.Verbs, newPerm.Verbs)
			merged = true
			break
		}
	}

	if !merged {
		permission.Spec.Permissions = append(permission.Spec.Permissions, newPerm)
	}

	// Update or create
	if permission.ObjectMeta.ResourceVersion == "" {
		return m.CreateUserPermission(ctx, permission)
	}
	return m.UpdateUserPermission(ctx, permission)
}

// RevokePermission revokes a specific permission from a user
func (m *Manager) RevokePermission(ctx context.Context, userID int64, namespace, resource, verb string) error {
	permission, err := m.GetUserPermission(ctx, userID)
	if err != nil {
		return err
	}

	// Remove matching permissions
	newPermissions := []Permission{}
	for _, p := range permission.Spec.Permissions {
		if p.Namespace != namespace {
			newPermissions = append(newPermissions, p)
			continue
		}

		// Filter out the resource and verb
		newResources := []string{}
		for _, r := range p.Resources {
			if r != resource {
				newResources = append(newResources, r)
			}
		}

		newVerbs := []string{}
		for _, v := range p.Verbs {
			if v != verb {
				newVerbs = append(newVerbs, v)
			}
		}

		// Keep permission if it still has resources or verbs
		if len(newResources) > 0 || len(newVerbs) > 0 {
			p.Resources = newResources
			p.Verbs = newVerbs
			newPermissions = append(newPermissions, p)
		}
	}

	permission.Spec.Permissions = newPermissions
	return m.UpdateUserPermission(ctx, permission)
}

// GetPermissionSummary returns a formatted summary of user permissions
func (m *Manager) GetPermissionSummary(ctx context.Context, userID int64) (string, error) {
	permission, err := m.GetUserPermission(ctx, userID)
	if err != nil {
		return "", err
	}

	summary := fmt.Sprintf("User ID: %d\nRole: %s\n\n", permission.Spec.TelegramUserID, permission.Spec.Role)

	if len(permission.Spec.Permissions) == 0 {
		summary += "No permissions granted"
		return summary, nil
	}

	summary += "Permissions:\n"
	for i, p := range permission.Spec.Permissions {
		summary += fmt.Sprintf("%d. Namespace: %s\n", i+1, p.Namespace)
		summary += fmt.Sprintf("   Resources: %v\n", p.Resources)
		summary += fmt.Sprintf("   Verbs: %v\n", p.Verbs)
		if p.Selector != "" {
			summary += fmt.Sprintf("   Selector: %s\n", p.Selector)
		}
		summary += "\n"
	}

	return summary, nil
}

// IsBootstrapAdmin checks if user is a bootstrap admin
func (m *Manager) IsBootstrapAdmin(userID int64) bool {
	return m.config.IsBootstrapAdmin(userID)
}

// Helper function to merge unique strings
func mergeUnique(slice1, slice2 []string) []string {
	set := make(map[string]bool)
	result := []string{}

	for _, s := range slice1 {
		if !set[s] {
			set[s] = true
			result = append(result, s)
		}
	}

	for _, s := range slice2 {
		if !set[s] {
			set[s] = true
			result = append(result, s)
		}
	}

	return result
}

// parseUserIDFromName extracts user ID from resource name (e.g., "user-123456" -> 123456)
func parseUserIDFromName(name string) (int64, error) {
	var userID int64
	_, err := fmt.Sscanf(name, "user-%d", &userID)
	if err != nil {
		return 0, fmt.Errorf("invalid resource name format: %s", name)
	}
	return userID, nil
}

// formatUserResourceName creates resource name from user ID
func formatUserResourceName(userID int64) string {
	return "user-" + strconv.FormatInt(userID, 10)
}
