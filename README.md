# Kubernetes Telegram Bot (kubectl-bot)

A Kubernetes Telegram Bot that provides secure, RBAC-controlled access to Kubernetes cluster operations via Telegram. Built with Go, using **Custom Resource Definitions (CRDs)** to store user permissions natively in Kubernetes.

## Features

- **Kubernetes-Native RBAC**: Permissions stored as CRDs in Kubernetes
- **Fine-Grained Access Control**: Control access by namespace, resource, verb, and label selectors
- **Secure**: Bootstrap admins, role-based permissions, selector enforcement
- **Easy Deployment**: Single binary, Docker image, Kubernetes manifests
- **GitOps-Friendly**: Permissions are version-controlled Kubernetes resources

## Architecture

### Permission Model

Permissions are stored as `TelegramBotPermission` custom resources with three components:

- **Namespace**: Specific namespace or `*` for all
- **Resources**: `pods`, `deployments`, `services`
- **Verbs**: `get`, `list`, `logs`, `restart`, `rollback`, `scale`
- **Selector** (optional): Label selector to restrict access (e.g., `app=frontend`)

### Supported Commands

#### Resource Queries
```
/namespaces                      - List accessible namespaces
/pods [namespace]                - List pods
/deployments [namespace]         - List deployments
/services [namespace]            - List services
```

#### Operations
```
/logs <pod> [-n <namespace>]                        - Get pod logs
/restart <deployment> [-n <namespace>]              - Restart deployment
/rollback <deployment> [-n <namespace>]             - Rollback deployment
/scale <deployment> <replicas> [-n <namespace>]     - Scale deployment
```

#### Admin Commands
```
/grant <user_id> <verb> <resource> [-n <namespace>] [-l <selector>]  - Grant permission
/revoke <user_id> <verb> <resource> [-n <namespace>]                 - Revoke permission
/permissions [user_id]                                                - Show permissions
```

## Quick Start

### Prerequisites

1. **Telegram Bot Token**: Create a bot via [@BotFather](https://t.me/botfather)
2. **Kubernetes Cluster**: Access to a K8s cluster
3. **Your Telegram User ID**: Get it from [@userinfobot](https://t.me/userinfobot)

### Deployment

#### Option 1: Using kubectl

1. **Clone the repository**
```bash
git clone https://github.com/reloadlife/kbot.git
cd kbot
```

2. **Deploy the CRD**
```bash
kubectl apply -f manifests/crd.yaml
```

3. **Configure the bot**

Edit `manifests/deployment.yaml` and update:
- `admin_ids`: Your Telegram user ID (comma-separated for multiple admins)
- `token`: Your Telegram bot token from BotFather

Or create them separately:
```bash
# Create ConfigMap with your admin Telegram ID
kubectl create configmap telegram-bot-config \
  --from-literal=admin_ids="YOUR_TELEGRAM_USER_ID"

# Create Secret with your bot token
kubectl create secret generic telegram-bot-secrets \
  --from-literal=token="YOUR_TELEGRAM_BOT_TOKEN"
```

4. **Deploy RBAC and the bot**
```bash
kubectl apply -f manifests/rbac.yaml
kubectl apply -f manifests/deployment.yaml
```

5. **Verify deployment**
```bash
kubectl get pods -l app=telegram-bot
kubectl logs -l app=telegram-bot
```

#### Option 2: Using Rancher

1. **Access Rancher UI**
   - Navigate to your cluster in Rancher
   - Go to **Cluster** → **Projects/Namespaces**

2. **Deploy CRD via Rancher**
   - Go to **Cluster Tools** → **CRD**
   - Click **Create from YAML**
   - Paste contents of `manifests/crd.yaml`
   - Click **Create**

3. **Create Namespace (Optional)**
   - Go to **Cluster** → **Projects/Namespaces**
   - Click **Create Namespace**
   - Name it `telegram-bot` (or use `default`)

4. **Create ConfigMap**
   - Go to **Storage** → **ConfigMaps**
   - Click **Create**
   - Name: `telegram-bot-config`
   - Add Key-Value:
     - Key: `admin_ids`
     - Value: Your Telegram user ID

5. **Create Secret**
   - Go to **Storage** → **Secrets**
   - Click **Create**
   - Type: **Opaque**
   - Name: `telegram-bot-secrets`
   - Add Key-Value:
     - Key: `token`
     - Value: Your Telegram bot token

6. **Deploy RBAC**
   - Go to **Cluster** → **More Resources** → **RBAC**
   - Create **ServiceAccount**: Import `manifests/rbac.yaml` (ServiceAccount section)
   - Create **ClusterRole**: Import `manifests/rbac.yaml` (ClusterRole section)
   - Create **ClusterRoleBinding**: Import `manifests/rbac.yaml` (ClusterRoleBinding section)

7. **Deploy the Bot**
   - Go to **Workloads** → **Deployments**
   - Click **Create**
   - Switch to **Edit as YAML**
   - Paste the Deployment section from `manifests/deployment.yaml`
   - Update image to: `ghcr.io/reloadlife/kbot:latest`
   - Click **Create**

8. **Verify in Rancher**
   - Go to **Workloads** → **Deployments**
   - Check that `telegram-bot` deployment is running
   - Click on deployment → **View Logs**

#### Option 3: Using Helm (Coming Soon)

```bash
helm repo add kbot https://reloadlife.github.io/kbot
helm install kbot kbot/kubectl-bot \
  --set telegram.token="YOUR_BOT_TOKEN" \
  --set telegram.adminIds="YOUR_TELEGRAM_ID"
```

### Using the Bot

1. **Start conversation**
   - Open Telegram and search for your bot
   - Send `/start`
   - You should see your role (admin if you're a bootstrap admin)

2. **Grant permissions to other users**
```
# Grant logs access to production namespace with selector
/grant 987654321 logs pods -n production -l app=frontend

# Grant deployment restart access
/grant 987654321 restart deployments -n staging

# Grant full access to dev namespace
/grant 987654321 * * -n dev
```

3. **Use the bot**
```
# List pods
/pods production

# Get logs
/logs frontend-web-5d7f8-abc -n production

# Restart deployment
/restart api-deployment -n staging

# Scale deployment
/scale api-deployment 3 -n staging
```

## Permission Examples

### Admin User (Full Access)
```yaml
apiVersion: telegram.k8s.io/v1
kind: TelegramBotPermission
metadata:
  name: admin-user-123456
spec:
  telegramUserId: 123456789
  role: admin
  permissions:
    - namespace: "*"
      resources: ["pods", "deployments", "services"]
      verbs: ["get", "list", "logs", "restart", "rollback", "scale"]
```

### Developer (Logs only, specific app)
```yaml
apiVersion: telegram.k8s.io/v1
kind: TelegramBotPermission
metadata:
  name: developer-user-987654
spec:
  telegramUserId: 987654321
  role: viewer
  permissions:
    - namespace: "production"
      resources: ["pods"]
      verbs: ["logs"]
      selector: "app=frontend"
```

### Operator (Restart deployments in staging)
```yaml
apiVersion: telegram.k8s.io/v1
kind: TelegramBotPermission
metadata:
  name: operator-user-555555
spec:
  telegramUserId: 555555555
  role: operator
  permissions:
    - namespace: "staging"
      resources: ["deployments"]
      verbs: ["get", "list", "restart", "rollback"]
    - namespace: "staging"
      resources: ["pods"]
      verbs: ["get", "list", "logs"]
```

Apply permissions:
```bash
kubectl apply -f permissions.yaml
```

## Development

### Local Development

1. **Prerequisites**
   - Go 1.21+
   - Access to a Kubernetes cluster
   - kubeconfig configured

2. **Clone and build**
```bash
git clone https://github.com/reloadlife/kbot.git
cd kbot
go mod download
go build -o bin/kubectl-bot ./cmd/bot
```

3. **Run locally**
```bash
export TELEGRAM_BOT_TOKEN="your-bot-token"
export ADMIN_TELEGRAM_IDS="your-telegram-id"
./bin/kubectl-bot
```

### Building Docker Image

```bash
docker build -t kubectl-bot:latest .
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with coverage report
make test-coverage

# Or use go directly
go test -v ./...
```

**Test Coverage:**
- ✅ Config package: 100% coverage
- ✅ RBAC validator: Core functions tested
- ✅ RBAC manager: DeepCopy and utility functions tested
- ✅ K8s client: GVR and utility functions tested
- ✅ Bot handlers: Command parsing and formatting tested

## Configuration

### Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `TELEGRAM_BOT_TOKEN` | Telegram bot token from BotFather | Yes | - |
| `ADMIN_TELEGRAM_IDS` | Comma-separated list of admin Telegram user IDs | Yes | - |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | No | info |

## Security Considerations

1. **Bootstrap Admins**: Initial admins are specified via `ADMIN_TELEGRAM_IDS` environment variable
2. **Selector Enforcement**: When a permission includes a selector, resources must match it
3. **Audit Logging**: All operations are logged with Telegram user ID
4. **In-Cluster RBAC**: The bot's ServiceAccount has minimal required K8s permissions
5. **No External Database**: All data stored in Kubernetes (secure, auditable)

## Troubleshooting

### Bot not responding
```bash
# Check bot logs
kubectl logs -l app=telegram-bot -f

# Check if CRD is installed
kubectl get crd telegrambotpermissions.telegram.k8s.io

# Check if bot has RBAC permissions
kubectl auth can-i list telegrambotpermissions --as=system:serviceaccount:default:telegram-bot
```

### Permission denied errors
```bash
# Check user permissions
/permissions YOUR_TELEGRAM_ID

# Check if CRD exists for user
kubectl get telegrambotpermissions user-YOUR_TELEGRAM_ID -o yaml
```

### Container image pull errors
```bash
# Verify image exists
docker pull ghcr.io/reloadlife/kbot:latest

# Check if using correct image tag
kubectl describe deployment telegram-bot
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see LICENSE file for details

## Support

- GitHub Issues: https://github.com/reloadlife/kbot/issues
- Telegram: Contact the bot directly for testing

## Roadmap

- [ ] Helm chart
- [ ] Multi-container support for logs
- [ ] Exec into pods
- [ ] Port-forwarding via Telegram
- [ ] StatefulSet operations
- [ ] Namespace creation/deletion
- [ ] Resource metrics and monitoring
- [ ] Alert integration
