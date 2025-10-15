# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Kubernetes Telegram Bot written in Go that provides secure, RBAC-controlled access to Kubernetes cluster operations via Telegram. Uses **Custom Resource Definitions (CRDs)** to store user permissions natively in Kubernetes, eliminating the need for external databases.

## Core Architecture Decisions

### Permission Storage: Kubernetes CRDs
- Permissions are stored as `TelegramBotPermission` custom resources (group: `telegram.k8s.io/v1`)
- Each CR maps a Telegram user ID to granular K8s permissions (namespace, resources, verbs, selectors)
- This makes permissions version-controlled, auditable via K8s events, and GitOps-compatible
- Bootstrap admins are configured via `ADMIN_TELEGRAM_IDS` environment variable

### RBAC Model
Three permission components:
1. **namespace**: Specific namespace or `*` for all
2. **resources**: pods, deployments, services
3. **verbs**: get, list, logs, restart, rollback, scale
4. **selector** (optional): Label selector to restrict access (e.g., `app=frontend`)

Permission validation flow:
```
User command → Extract (userID, namespace, resource, verb, selector)
            → Fetch TelegramBotPermission CR for userID
            → Validate permission matches (namespace, resource, verb)
            → If selector specified in permission, validate against resource labels
            → Execute or deny
```

### Label Selector Enforcement
Critical security feature: If a permission specifies `selector: "app=frontend"`, the bot validates that any accessed resource (pod, deployment) actually has matching labels before allowing the operation. This prevents privilege escalation.

## Project Structure

```
cmd/bot/main.go                 # Entry point
internal/bot/                   # Telegram bot logic and command handlers
internal/k8s/                   # K8s client wrapper (pods, deployments, logs operations)
internal/rbac/                  # RBAC manager (CRD operations, permission validation)
internal/config/                # Configuration loading
manifests/                      # K8s manifests (CRD, deployment, RBAC)
```

## Development Commands

```bash
# Build
go build -o bin/kubectl-bot ./cmd/bot

# Run locally (requires kubeconfig and TELEGRAM_BOT_TOKEN)
export TELEGRAM_BOT_TOKEN=<your-token>
export ADMIN_TELEGRAM_IDS=<comma-separated-telegram-user-ids>
go run ./cmd/bot/main.go

# Test
go test ./...

# Deploy to K8s
kubectl apply -f manifests/crd.yaml
kubectl apply -f manifests/rbac.yaml
kubectl apply -f manifests/deployment.yaml
```

## Key Go Packages

- `github.com/go-telegram-bot-api/telegram-bot-api/v5` - Telegram bot API
- `k8s.io/client-go` - Kubernetes client
- `k8s.io/apiextensions-apiserver` - CRD operations
- `k8s.io/apimachinery` - K8s API types and label selectors

## Bot Commands Reference

### User Commands (RBAC-enforced)
- `/pods [namespace]` - List pods
- `/deployments [namespace]` - List deployments
- `/logs <pod> [-n <ns>] [-l <selector>]` - Get pod logs
- `/restart <deployment> [-n <ns>]` - Rollout restart
- `/rollback <deployment> [-n <ns>]` - Rollout undo
- `/scale <deployment> <replicas> [-n <ns>]` - Scale deployment

### Admin Commands (admin role only)
- `/grant <user_id> <verb> <resource> [-n <ns>] [-l <selector>]` - Grant permission
- `/revoke <user_id> <verb> <resource> [-n <ns>]` - Revoke permission
- `/permissions [user_id]` - Show user permissions

## Permission Validation Implementation

Located in `internal/rbac/validator.go`:

```go
type PermissionCheck struct {
    TelegramUserID int64
    Namespace      string
    Resource       string  // "pods", "deployments", "services"
    Verb           string  // "get", "list", "logs", "restart", etc.
    Selector       string  // Optional label selector
}

func (m *Manager) CheckPermission(check PermissionCheck) (bool, error)
```

The validator:
1. Fetches the user's `TelegramBotPermission` CR by Telegram user ID
2. Returns true immediately if role is "admin"
3. Iterates through the permissions array
4. Matches namespace (exact or `*`), resource, and verb
5. If the permission has a selector, validates the user's query selector is compatible
6. Returns true if any permission grants access

## CRD Spec Structure

```yaml
apiVersion: telegram.k8s.io/v1
kind: TelegramBotPermission
metadata:
  name: user-<telegram-id>
spec:
  telegramUserId: <int64>
  role: admin|operator|viewer
  permissions:
    - namespace: string        # "*" for all
      resources: [string]      # ["pods", "deployments", "services"]
      verbs: [string]          # ["get", "list", "logs", "restart", "rollback", "scale"]
      selector: string         # Optional: "app=frontend,tier=web"
```

## Security Considerations

1. **Bootstrap Admins**: Initial admin users are specified via `ADMIN_TELEGRAM_IDS` environment variable
2. **Selector Validation**: When a permission includes a selector, the bot validates resources against it before allowing access
3. **Audit Logging**: All operations should be logged with Telegram user ID and action
4. **Bot's K8s RBAC**: The bot's ServiceAccount has cluster-wide access to perform operations on behalf of users, but user permissions are enforced at the application layer
5. **Input Validation**: Command parameters must be validated before execution

## Deployment Notes

The bot runs in-cluster with:
- ServiceAccount: `telegram-bot`
- ClusterRole: Access to `telegrambotpermissions` CRDs and K8s resources (pods, deployments, services)
- CRD must be deployed before the bot starts

## Implementation Phases

1. **Foundation**: Bot initialization, K8s client setup, bootstrap admin validation
2. **CRD & RBAC**: Deploy CRD, implement permission validator
3. **Core Commands**: Resource listing with RBAC enforcement
4. **Operations**: Restart, rollback, scale with confirmations
5. **Admin Commands**: Grant/revoke permission management
