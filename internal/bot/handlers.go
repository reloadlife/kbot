package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kubectl-bot/internal/rbac"
)

// handleStart handles the /start command
func (b *Bot) handleStart(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	role := b.getUserRole(ctx, userID)

	response := fmt.Sprintf("👋 Welcome to Kubernetes Bot!\n\n"+
		"Your role: *%s*\n"+
		"User ID: `%d`\n\n"+
		"Type /help to see available commands.", role, userID)

	b.sendMessage(message.Chat.ID, response)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(ctx context.Context, message *tgbotapi.Message) {
	help := `*Available Commands:*

*Resource Queries:*
/namespaces - List accessible namespaces
/pods [namespace] - List pods
/deployments [namespace] - List deployments
/services [namespace] - List services

*Operations:*
/logs <pod> [-n <namespace>] - Get pod logs
/restart <deployment> [-n <namespace>] - Restart deployment
/rollback <deployment> [-n <namespace>] - Rollback deployment
/scale <deployment> <replicas> [-n <namespace>] - Scale deployment

*Admin Commands:*
/grant <user_id> <verb> <resource> [-n <namespace>] [-l <selector>] - Grant permission
/revoke <user_id> <verb> <resource> [-n <namespace>] - Revoke permission
/permissions [user_id] - Show user permissions

*Examples:*
/pods production
/logs frontend-pod-abc -n production
/restart api-deployment -n staging
/grant 123456789 logs pods -n production -l app=frontend
`

	b.sendMessage(message.Chat.ID, help)
}

// handleNamespaces handles the /namespaces command
func (b *Bot) handleNamespaces(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	namespaces, err := b.validator.ValidateAndGetNamespaces(ctx, userID)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	if len(namespaces) == 0 {
		b.sendMessage(message.Chat.ID, "No accessible namespaces")
		return
	}

	response := "*Accessible Namespaces:*\n"
	for _, ns := range namespaces {
		response += fmt.Sprintf("- %s\n", ns)
	}

	b.sendMessage(message.Chat.ID, response)
}

// handlePods handles the /pods command
func (b *Bot) handlePods(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	namespace := "default"
	if len(args) > 0 {
		namespace = args[0]
	}

	namespace = rbac.NormalizeNamespace(namespace)

	// Check permission
	allowed, reason, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: userID,
		Namespace:      namespace,
		Resource:       "pods",
		Verb:           "list",
	})

	if err != nil || !allowed {
		b.sendMessage(message.Chat.ID, rbac.FormatPermissionDenied(reason))
		return
	}

	// List pods
	pods, err := b.k8sClient.ListPods(ctx, namespace, "")
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	if len(pods.Items) == 0 {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("No pods found in namespace *%s*", namespace))
		return
	}

	response := fmt.Sprintf("*Pods in namespace %s:*\n\n", namespace)
	for _, pod := range pods.Items {
		status := string(pod.Status.Phase)
		response += fmt.Sprintf("📦 `%s`\n   Status: %s\n\n", pod.Name, status)
	}

	b.sendMessage(message.Chat.ID, response)
}

// handleDeployments handles the /deployments command
func (b *Bot) handleDeployments(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	namespace := "default"
	if len(args) > 0 {
		namespace = args[0]
	}

	namespace = rbac.NormalizeNamespace(namespace)

	// Check permission
	allowed, reason, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: userID,
		Namespace:      namespace,
		Resource:       "deployments",
		Verb:           "list",
	})

	if err != nil || !allowed {
		b.sendMessage(message.Chat.ID, rbac.FormatPermissionDenied(reason))
		return
	}

	// List deployments
	deployments, err := b.k8sClient.ListDeployments(ctx, namespace, "")
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	if len(deployments.Items) == 0 {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("No deployments found in namespace *%s*", namespace))
		return
	}

	response := fmt.Sprintf("*Deployments in namespace %s:*\n\n", namespace)
	for _, dep := range deployments.Items {
		replicas := int32(0)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		response += fmt.Sprintf("🚀 `%s`\n   Replicas: %d/%d\n\n",
			dep.Name, dep.Status.ReadyReplicas, replicas)
	}

	b.sendMessage(message.Chat.ID, response)
}

// handleServices handles the /services command
func (b *Bot) handleServices(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	namespace := "default"
	if len(args) > 0 {
		namespace = args[0]
	}

	namespace = rbac.NormalizeNamespace(namespace)

	// Check permission
	allowed, reason, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: userID,
		Namespace:      namespace,
		Resource:       "services",
		Verb:           "list",
	})

	if err != nil || !allowed {
		b.sendMessage(message.Chat.ID, rbac.FormatPermissionDenied(reason))
		return
	}

	// List services
	services, err := b.k8sClient.ListServices(ctx, namespace, "")
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	if len(services.Items) == 0 {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("No services found in namespace *%s*", namespace))
		return
	}

	response := fmt.Sprintf("*Services in namespace %s:*\n\n", namespace)
	for _, svc := range services.Items {
		response += fmt.Sprintf("🌐 `%s`\n   Type: %s\n\n", svc.Name, svc.Spec.Type)
	}

	b.sendMessage(message.Chat.ID, response)
}

// handleLogs handles the /logs command
func (b *Bot) handleLogs(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	if len(args) == 0 {
		b.sendMessage(message.Chat.ID, "Usage: /logs <pod> [-n <namespace>]")
		return
	}

	podName := args[0]
	namespace := "default"

	// Parse flags
	for i := 1; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			namespace = args[i+1]
			i++
		}
	}

	namespace = rbac.NormalizeNamespace(namespace)

	// Check permission
	allowed, reason, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: userID,
		Namespace:      namespace,
		Resource:       "pods",
		Verb:           "logs",
		ResourceName:   podName,
	})

	if err != nil || !allowed {
		b.sendMessage(message.Chat.ID, rbac.FormatPermissionDenied(reason))
		return
	}

	// Get logs (last 100 lines)
	logs, err := b.k8sClient.GetPodLogs(ctx, namespace, podName, 100)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	response := fmt.Sprintf("*Logs for pod %s (namespace: %s):*\n```\n%s\n```", podName, namespace, logs)

	// Telegram message limit is 4096 chars
	if len(response) > 4000 {
		response = response[:4000] + "\n...(truncated)\n```"
	}

	b.sendMessage(message.Chat.ID, response)
}

// handleRestart handles the /restart command
func (b *Bot) handleRestart(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	if len(args) == 0 {
		b.sendMessage(message.Chat.ID, "Usage: /restart <deployment> [-n <namespace>]")
		return
	}

	deploymentName := args[0]
	namespace := "default"

	// Parse flags
	for i := 1; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			namespace = args[i+1]
			i++
		}
	}

	namespace = rbac.NormalizeNamespace(namespace)

	// Check permission
	allowed, reason, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: userID,
		Namespace:      namespace,
		Resource:       "deployments",
		Verb:           "restart",
		ResourceName:   deploymentName,
	})

	if err != nil || !allowed {
		b.sendMessage(message.Chat.ID, rbac.FormatPermissionDenied(reason))
		return
	}

	// Restart deployment
	err = b.k8sClient.RestartDeployment(ctx, namespace, deploymentName)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Deployment `%s` restarted in namespace *%s*", deploymentName, namespace))
}

// handleRollback handles the /rollback command
func (b *Bot) handleRollback(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	if len(args) == 0 {
		b.sendMessage(message.Chat.ID, "Usage: /rollback <deployment> [-n <namespace>]")
		return
	}

	deploymentName := args[0]
	namespace := "default"

	// Parse flags
	for i := 1; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			namespace = args[i+1]
			i++
		}
	}

	namespace = rbac.NormalizeNamespace(namespace)

	// Check permission
	allowed, reason, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: userID,
		Namespace:      namespace,
		Resource:       "deployments",
		Verb:           "rollback",
		ResourceName:   deploymentName,
	})

	if err != nil || !allowed {
		b.sendMessage(message.Chat.ID, rbac.FormatPermissionDenied(reason))
		return
	}

	// Rollback deployment
	err = b.k8sClient.RollbackDeployment(ctx, namespace, deploymentName)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Deployment `%s` rolled back in namespace *%s*", deploymentName, namespace))
}

// handleScale handles the /scale command
func (b *Bot) handleScale(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	if len(args) < 2 {
		b.sendMessage(message.Chat.ID, "Usage: /scale <deployment> <replicas> [-n <namespace>]")
		return
	}

	deploymentName := args[0]
	replicas, err := strconv.ParseInt(args[1], 10, 32)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ Invalid replica count")
		return
	}

	namespace := "default"

	// Parse flags
	for i := 2; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			namespace = args[i+1]
			i++
		}
	}

	namespace = rbac.NormalizeNamespace(namespace)

	// Check permission
	allowed, reason, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: userID,
		Namespace:      namespace,
		Resource:       "deployments",
		Verb:           "scale",
		ResourceName:   deploymentName,
	})

	if err != nil || !allowed {
		b.sendMessage(message.Chat.ID, rbac.FormatPermissionDenied(reason))
		return
	}

	// Scale deployment
	err = b.k8sClient.ScaleDeployment(ctx, namespace, deploymentName, int32(replicas))
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Deployment `%s` scaled to %d replicas in namespace *%s*",
		deploymentName, replicas, namespace))
}

// handleGrant handles the /grant command (admin only)
func (b *Bot) handleGrant(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	// Check if user is admin
	if !b.rbac.IsBootstrapAdmin(userID) {
		permission, err := b.rbac.GetUserPermission(ctx, userID)
		if err != nil || permission.Spec.Role != "admin" {
			b.sendMessage(message.Chat.ID, "❌ Admin access required")
			return
		}
	}

	if len(args) < 3 {
		b.sendMessage(message.Chat.ID, "Usage: /grant <user_id> <verb> <resource> [-n <namespace>] [-l <selector>]")
		return
	}

	targetUserID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ Invalid user ID")
		return
	}

	verb := args[1]
	resource := args[2]
	namespace := "*"
	selector := ""

	// Parse flags
	for i := 3; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			namespace = args[i+1]
			i++
		} else if args[i] == "-l" && i+1 < len(args) {
			selector = args[i+1]
			i++
		}
	}

	// Grant permission
	err = b.rbac.GrantPermission(ctx, targetUserID, namespace, resource, verb, selector)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	response := fmt.Sprintf("✅ Permission granted to user `%d`\n\n"+
		"Namespace: %s\n"+
		"Resource: %s\n"+
		"Verb: %s\n",
		targetUserID, namespace, resource, verb)

	if selector != "" {
		response += fmt.Sprintf("Selector: %s\n", selector)
	}

	b.sendMessage(message.Chat.ID, response)
}

// handleRevoke handles the /revoke command (admin only)
func (b *Bot) handleRevoke(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	// Check if user is admin
	if !b.rbac.IsBootstrapAdmin(userID) {
		permission, err := b.rbac.GetUserPermission(ctx, userID)
		if err != nil || permission.Spec.Role != "admin" {
			b.sendMessage(message.Chat.ID, "❌ Admin access required")
			return
		}
	}

	if len(args) < 4 {
		b.sendMessage(message.Chat.ID, "Usage: /revoke <user_id> <verb> <resource> -n <namespace>")
		return
	}

	targetUserID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ Invalid user ID")
		return
	}

	verb := args[1]
	resource := args[2]
	namespace := ""

	// Parse namespace flag
	for i := 3; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			namespace = args[i+1]
			break
		}
	}

	if namespace == "" {
		b.sendMessage(message.Chat.ID, "❌ Namespace is required (-n <namespace>)")
		return
	}

	// Revoke permission
	err = b.rbac.RevokePermission(ctx, targetUserID, namespace, resource, verb)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Permission revoked from user `%d`", targetUserID))
}

// handlePermissions handles the /permissions command
func (b *Bot) handlePermissions(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	args := strings.Fields(message.CommandArguments())

	targetUserID := userID
	if len(args) > 0 {
		// Admin can check other users' permissions
		if !b.rbac.IsBootstrapAdmin(userID) {
			permission, err := b.rbac.GetUserPermission(ctx, userID)
			if err != nil || permission.Spec.Role != "admin" {
				b.sendMessage(message.Chat.ID, "❌ Admin access required to view other users' permissions")
				return
			}
		}

		var err error
		targetUserID, err = strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			b.sendMessage(message.Chat.ID, "❌ Invalid user ID")
			return
		}
	}

	summary, err := b.rbac.GetPermissionSummary(ctx, targetUserID)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf("*Permissions Summary:*\n\n```\n%s\n```", summary))
}
