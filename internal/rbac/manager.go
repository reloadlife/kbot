package rbac

import (
	"context"
	"fmt"
	"strconv"

	"kubectl-bot/internal/config"
	"kubectl-bot/internal/k8s"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
)

// validRoles is the set of roles a user may be assigned.
var validRoles = map[string]bool{"admin": true, "operator": true, "viewer": true}

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

// newUserPermission builds an empty viewer permission object for a user.
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
			Role:           "viewer",
			Permissions:    []Permission{},
		},
	}
}

// mutateUserPermission performs a read-modify-write of a user's permission CR,
// retrying on optimistic-concurrency conflicts. The mutate callback receives
// the current object (a fresh empty one if none exists) and returns nothing;
// the object is then created or updated as appropriate.
func (m *Manager) mutateUserPermission(ctx context.Context, userID int64, mutate func(p *TelegramBotPermission)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		permission, err := m.GetUserPermission(ctx, userID)
		create := false
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			permission = newUserPermission(userID)
			create = true
		}

		mutate(permission)

		if create {
			return m.CreateUserPermission(ctx, permission)
		}
		return m.UpdateUserPermission(ctx, permission)
	})
}

// GrantPermission grants a specific (verb, resource) capability to a user,
// merging into an existing bundle keyed by (namespace, selector).
func (m *Manager) GrantPermission(ctx context.Context, userID int64, namespace, resource, verb, selector string) error {
	return m.mutateUserPermission(ctx, userID, func(permission *TelegramBotPermission) {
		for i, p := range permission.Spec.Permissions {
			if p.Namespace == namespace && p.Selector == selector {
				permission.Spec.Permissions[i].Resources = mergeUnique(p.Resources, []string{resource})
				permission.Spec.Permissions[i].Verbs = mergeUnique(p.Verbs, []string{verb})
				return
			}
		}
		permission.Spec.Permissions = append(permission.Spec.Permissions, Permission{
			Namespace: namespace,
			Resources: []string{resource},
			Verbs:     []string{verb},
			Selector:  selector,
		})
	})
}

// RevokePermission removes a single (verb, resource) capability in a namespace,
// correctly splitting any cross-product bundle it appears in.
func (m *Manager) RevokePermission(ctx context.Context, userID int64, namespace, resource, verb string) error {
	return m.mutateUserPermission(ctx, userID, func(permission *TelegramBotPermission) {
		permission.Spec.Permissions = removeCapability(permission.Spec.Permissions, namespace, resource, verb)
	})
}

// SetRole sets a user's role, creating the permission object if needed.
func (m *Manager) SetRole(ctx context.Context, userID int64, role string) error {
	if !validRoles[role] {
		return fmt.Errorf("invalid role %q (must be admin, operator, or viewer)", role)
	}
	return m.mutateUserPermission(ctx, userID, func(permission *TelegramBotPermission) {
		permission.Spec.Role = role
		permission.Spec.TelegramUserID = userID
	})
}

// ListUserPermissions returns all TelegramBotPermission objects in the cluster.
func (m *Manager) ListUserPermissions(ctx context.Context) ([]TelegramBotPermission, error) {
	list, err := m.k8sClient.GetDynamicClient().
		Resource(k8s.TelegramBotPermissionGVR()).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	out := make([]TelegramBotPermission, 0, len(list.Items))
	for i := range list.Items {
		var perm TelegramBotPermission
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &perm); err != nil {
			continue
		}
		out = append(out, perm)
	}
	return out, nil
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
