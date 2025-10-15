# kubectl-bot Helm Chart

This Helm chart deploys the Kubernetes Telegram Bot to your Kubernetes cluster.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- A Telegram bot token from [@BotFather](https://t.me/botfather)
- Your Telegram user ID from [@userinfobot](https://t.me/userinfobot)

## Installation

### Add Helm Repository

```bash
helm repo add kbot https://reloadlife.github.io/kbot
helm repo update
```

### Install Chart

```bash
helm install my-kbot kbot/kubectl-bot \
  --set telegram.token="YOUR_TELEGRAM_BOT_TOKEN" \
  --set telegram.adminIds="YOUR_TELEGRAM_USER_ID" \
  --namespace kbot-system \
  --create-namespace
```

### Install from Local Chart

```bash
cd helm
helm install my-kbot . \
  --set telegram.token="YOUR_TELEGRAM_BOT_TOKEN" \
  --set telegram.adminIds="YOUR_TELEGRAM_USER_ID" \
  --namespace kbot-system \
  --create-namespace
```

## Configuration

The following table lists the configurable parameters of the kubectl-bot chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of bot replicas | `1` |
| `image.repository` | Bot image repository | `ghcr.io/reloadlife/kbot` |
| `image.pullPolicy` | Image pull policy | `Always` |
| `image.tag` | Image tag (defaults to chart appVersion) | `"latest"` |
| `telegram.token` | Telegram bot token | `""` (required) |
| `telegram.adminIds` | Comma-separated admin Telegram user IDs | `""` (required) |
| `telegram.existingSecret` | Use existing secret for bot token | `""` |
| `telegram.existingSecretKey` | Key in existing secret | `"token"` |
| `telegram.existingConfigMap` | Use existing configmap for admin IDs | `""` |
| `telegram.existingConfigMapKey` | Key in existing configmap | `"admin_ids"` |
| `bot.namespace` | Bot namespace (auto-detected) | `""` |
| `bot.deploymentName` | Bot deployment name (auto-detected) | `""` |
| `logLevel` | Log level | `"info"` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name | `""` |
| `rbac.create` | Create RBAC resources | `true` |
| `podAnnotations` | Pod annotations | `{}` |
| `resources.limits.cpu` | CPU limit | `200m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `affinity` | Affinity | `{}` |

## Examples

### Basic Installation

```bash
helm install kbot kbot/kubectl-bot \
  --set telegram.token="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11" \
  --set telegram.adminIds="123456789"
```

### Using Existing Secrets

```bash
# Create secret manually
kubectl create secret generic my-telegram-secret \
  --from-literal=token="YOUR_BOT_TOKEN"

# Install with existing secret
helm install kbot kbot/kubectl-bot \
  --set telegram.existingSecret="my-telegram-secret" \
  --set telegram.adminIds="123456789"
```

### Multiple Admins

```bash
helm install kbot kbot/kubectl-bot \
  --set telegram.token="YOUR_BOT_TOKEN" \
  --set telegram.adminIds="123456789,987654321,555555555"
```

### Custom Resources

```bash
helm install kbot kbot/kubectl-bot \
  --set telegram.token="YOUR_BOT_TOKEN" \
  --set telegram.adminIds="123456789" \
  --set resources.limits.cpu="500m" \
  --set resources.limits.memory="256Mi" \
  --set resources.requests.cpu="200m" \
  --set resources.requests.memory="128Mi"
```

### With Node Selector

```bash
helm install kbot kbot/kubectl-bot \
  --set telegram.token="YOUR_BOT_TOKEN" \
  --set telegram.adminIds="123456789" \
  --set nodeSelector."kubernetes\.io/hostname"="node-1"
```

## Upgrading

```bash
helm upgrade kbot kbot/kubectl-bot \
  --set telegram.token="YOUR_BOT_TOKEN" \
  --set telegram.adminIds="123456789"
```

## Uninstalling

```bash
helm uninstall kbot
```

To remove the CRD:

```bash
kubectl delete crd telegrambotpermissions.kbot.go.mamad.dev
```

## Values File Example

Create a `values.yaml` file:

```yaml
telegram:
  token: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
  adminIds: "123456789,987654321"

logLevel: "debug"

resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 200m
    memory: 128Mi

nodeSelector:
  kubernetes.io/os: linux
```

Install with values file:

```bash
helm install kbot kbot/kubectl-bot -f values.yaml
```

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -l app.kubernetes.io/name=kubectl-bot
```

### View Logs

```bash
kubectl logs -l app.kubernetes.io/name=kubectl-bot -f
```

### Verify CRD Installation

```bash
kubectl get crd telegrambotpermissions.kbot.go.mamad.dev
```

### Check RBAC Permissions

```bash
kubectl auth can-i list telegrambotpermissions \
  --as=system:serviceaccount:default:kubectl-bot-my-kbot
```

## Support

- GitHub Issues: https://github.com/reloadlife/kbot/issues
- Documentation: https://github.com/reloadlife/kbot
