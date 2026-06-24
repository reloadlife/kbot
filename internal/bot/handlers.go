package bot

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kubectl-bot/internal/k8s"
	"kubectl-bot/internal/rbac"

	corev1 "k8s.io/api/core/v1"
)

// maxReplicas caps the /scale command as a guardrail against fat-fingering.
const maxReplicas = 100

// parsedArgs holds positional arguments plus recognised flags shared across
// commands.
type parsedArgs struct {
	positional []string
	namespace  string
	selector   string
	container  string
	tail       int64
	previous   bool
	since      int64
}

// parseArgs tokenises a command argument string into positionals and flags.
func parseArgs(raw string) parsedArgs {
	p := parsedArgs{tail: 100}
	toks := strings.Fields(raw)
	for i := 0; i < len(toks); i++ {
		switch toks[i] {
		case "-n", "--namespace":
			if i+1 < len(toks) {
				p.namespace = toks[i+1]
				i++
			}
		case "-l", "--selector":
			if i+1 < len(toks) {
				p.selector = toks[i+1]
				i++
			}
		case "-c", "--container":
			if i+1 < len(toks) {
				p.container = toks[i+1]
				i++
			}
		case "--tail":
			if i+1 < len(toks) {
				if n, err := strconv.ParseInt(toks[i+1], 10, 64); err == nil {
					p.tail = n
				}
				i++
			}
		case "--since":
			if i+1 < len(toks) {
				if n, err := strconv.ParseInt(toks[i+1], 10, 64); err == nil {
					p.since = n
				}
				i++
			}
		case "--previous", "-p":
			p.previous = true
		default:
			p.positional = append(p.positional, toks[i])
		}
	}
	return p
}

// combineSelectors ANDs a mandatory selector (from the permission) with an
// optional user-supplied one. Either may be empty.
func combineSelectors(required, user string) string {
	switch {
	case required == "":
		return user
	case user == "":
		return required
	default:
		return required + "," + user
	}
}

// formatAge renders a duration like kubectl's AGE column.
func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

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

// handleNamespaces handles the /namespaces command
func (b *Bot) handleNamespaces(ctx context.Context, message *tgbotapi.Message) {
	namespaces, err := b.validator.ValidateAndGetNamespaces(ctx, message.From.ID)
	if err != nil {
		b.fail(message.Chat.ID, "List namespaces", err)
		return
	}
	if len(namespaces) == 0 {
		b.send(message.Chat.ID, "No accessible namespaces.")
		return
	}

	sort.Strings(namespaces)
	var sb strings.Builder
	sb.WriteString("<b>Accessible Namespaces:</b>\n")
	for _, ns := range namespaces {
		sb.WriteString("• " + code(ns) + "\n")
	}
	b.send(message.Chat.ID, sb.String())
}

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

// handlePods handles the /pods command
func (b *Bot) handlePods(ctx context.Context, message *tgbotapi.Message) {
	p := parseArgs(message.CommandArguments())
	namespace := rbac.NormalizeNamespace(firstOr(p.positional, p.namespace))

	selector, restriction, ok := b.resolveListAccess(ctx, message.Chat.ID, message.From.ID, namespace, "pods", p.selector)
	if !ok {
		return
	}

	pods, err := b.k8sClient.ListPods(ctx, namespace, selector)
	if err != nil {
		b.fail(message.Chat.ID, "List pods", err)
		return
	}
	if len(pods.Items) == 0 {
		b.send(message.Chat.ID, "No pods found in namespace "+code(namespace))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Pods in %s</b> (%d)\n\n", htmlEscape(namespace), len(pods.Items)))
	for i := range pods.Items {
		pod := &pods.Items[i]
		ready, total, restarts := podReadiness(pod)
		sb.WriteString(fmt.Sprintf("📦 %s\n   %s • %d/%d ready • %d restarts • %s\n\n",
			code(pod.Name), htmlEscape(string(pod.Status.Phase)), ready, total, restarts, formatAge(pod.CreationTimestamp.Time)))
	}
	b.send(message.Chat.ID, sb.String()+selectorFooter(restriction))
}

// podReadiness returns ready/total container counts and total restart count.
func podReadiness(pod *corev1.Pod) (ready, total int, restarts int) {
	total = len(pod.Status.ContainerStatuses)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
		restarts += int(cs.RestartCount)
	}
	return ready, total, restarts
}

// handleDeployments handles the /deployments command
func (b *Bot) handleDeployments(ctx context.Context, message *tgbotapi.Message) {
	p := parseArgs(message.CommandArguments())
	namespace := rbac.NormalizeNamespace(firstOr(p.positional, p.namespace))

	selector, restriction, ok := b.resolveListAccess(ctx, message.Chat.ID, message.From.ID, namespace, "deployments", p.selector)
	if !ok {
		return
	}

	deployments, err := b.k8sClient.ListDeployments(ctx, namespace, selector)
	if err != nil {
		b.fail(message.Chat.ID, "List deployments", err)
		return
	}
	if len(deployments.Items) == 0 {
		b.send(message.Chat.ID, "No deployments found in namespace "+code(namespace))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Deployments in %s</b> (%d)\n\n", htmlEscape(namespace), len(deployments.Items)))
	for i := range deployments.Items {
		dep := &deployments.Items[i]
		desired := int32(0)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		sb.WriteString(fmt.Sprintf("🚀 %s\n   %d/%d ready • %s\n\n",
			code(dep.Name), dep.Status.ReadyReplicas, desired, formatAge(dep.CreationTimestamp.Time)))
	}
	b.send(message.Chat.ID, sb.String()+selectorFooter(restriction))
}

// handleServices handles the /services command
func (b *Bot) handleServices(ctx context.Context, message *tgbotapi.Message) {
	p := parseArgs(message.CommandArguments())
	namespace := rbac.NormalizeNamespace(firstOr(p.positional, p.namespace))

	selector, restriction, ok := b.resolveListAccess(ctx, message.Chat.ID, message.From.ID, namespace, "services", p.selector)
	if !ok {
		return
	}

	services, err := b.k8sClient.ListServices(ctx, namespace, selector)
	if err != nil {
		b.fail(message.Chat.ID, "List services", err)
		return
	}
	if len(services.Items) == 0 {
		b.send(message.Chat.ID, "No services found in namespace "+code(namespace))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Services in %s</b> (%d)\n\n", htmlEscape(namespace), len(services.Items)))
	for i := range services.Items {
		svc := &services.Items[i]
		sb.WriteString(fmt.Sprintf("🌐 %s\n   %s • %s\n\n",
			code(svc.Name), htmlEscape(string(svc.Spec.Type)), htmlEscape(svc.Spec.ClusterIP)))
	}
	b.send(message.Chat.ID, sb.String()+selectorFooter(restriction))
}

// handleLogs handles the /logs command
func (b *Bot) handleLogs(ctx context.Context, message *tgbotapi.Message) {
	p := parseArgs(message.CommandArguments())
	if len(p.positional) == 0 {
		b.send(message.Chat.ID, "Usage: /logs &lt;pod&gt; [-n ns] [-c container] [--tail N] [--previous] [--since secs]")
		return
	}
	podName := p.positional[0]
	namespace := rbac.NormalizeNamespace(p.namespace)

	dec, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: message.From.ID,
		Namespace:      namespace,
		Resource:       "pods",
		Verb:           "logs",
		ResourceName:   podName,
	})
	if err != nil || !dec.Allowed {
		b.send(message.Chat.ID, rbac.FormatPermissionDenied(dec.Reason))
		return
	}

	logs, err := b.k8sClient.GetPodLogs(ctx, namespace, podName, k8sLogOptions(p))
	if err != nil && logs == "" {
		b.fail(message.Chat.ID, "Get logs", err)
		return
	}
	if strings.TrimSpace(logs) == "" {
		b.send(message.Chat.ID, "No log output for "+code(podName))
		return
	}

	header := fmt.Sprintf("<b>Logs: %s</b> (ns %s)", htmlEscape(podName), htmlEscape(namespace))

	// Short logs render inline; anything that wouldn't fit a single message is
	// uploaded as a .log file so nothing is lost to truncation.
	if len([]rune(logs)) <= 3500 {
		b.send(message.Chat.ID, header+"\n"+pre(logs))
		return
	}

	filename := sanitizeFilename(fmt.Sprintf("%s-%s.log", podName, p.container))
	b.sendDocument(message.Chat.ID, filename, header, []byte(logs))
}

// sanitizeFilename keeps a filename safe and tidy: alphanumerics, dash, dot,
// underscore only; trailing separators trimmed.
func sanitizeFilename(name string) string {
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}
	out := strings.Trim(sb.String(), "-_.")
	if out == "" || out == ".log" {
		return "pod.log"
	}
	return out
}

// k8sLogOptions maps parsed flags onto the k8s log option struct.
func k8sLogOptions(p parsedArgs) k8s.LogOptions {
	return k8s.LogOptions{Container: p.container, TailLines: p.tail, Previous: p.previous, SinceSeconds: p.since}
}

// handleDescribe handles /describe <pod|deployment> <name>
func (b *Bot) handleDescribe(ctx context.Context, message *tgbotapi.Message) {
	p := parseArgs(message.CommandArguments())
	if len(p.positional) < 2 {
		b.send(message.Chat.ID, "Usage: /describe &lt;pod|deployment&gt; &lt;name&gt; [-n ns]")
		return
	}
	kind := strings.ToLower(p.positional[0])
	name := p.positional[1]
	namespace := rbac.NormalizeNamespace(p.namespace)

	resource := ""
	switch kind {
	case "pod", "pods":
		resource = "pods"
	case "deployment", "deployments", "deploy":
		resource = "deployments"
	default:
		b.send(message.Chat.ID, "First argument must be 'pod' or 'deployment'.")
		return
	}

	dec, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: message.From.ID,
		Namespace:      namespace,
		Resource:       resource,
		Verb:           "get",
		ResourceName:   name,
	})
	if err != nil || !dec.Allowed {
		b.send(message.Chat.ID, rbac.FormatPermissionDenied(dec.Reason))
		return
	}

	if resource == "pods" {
		pod, err := b.k8sClient.GetPod(ctx, namespace, name)
		if err != nil {
			b.fail(message.Chat.ID, "Describe pod", err)
			return
		}
		b.send(message.Chat.ID, describePod(pod))
		return
	}

	dep, err := b.k8sClient.GetDeployment(ctx, namespace, name)
	if err != nil {
		b.fail(message.Chat.ID, "Describe deployment", err)
		return
	}
	b.send(message.Chat.ID, describeDeployment(dep))
}

// handleEvents handles /events [namespace]
func (b *Bot) handleEvents(ctx context.Context, message *tgbotapi.Message) {
	p := parseArgs(message.CommandArguments())
	namespace := rbac.NormalizeNamespace(firstOr(p.positional, p.namespace))

	if _, _, ok := b.resolveListAccess(ctx, message.Chat.ID, message.From.ID, namespace, "pods", ""); !ok {
		return
	}

	events, err := b.k8sClient.ListEvents(ctx, namespace)
	if err != nil {
		b.fail(message.Chat.ID, "List events", err)
		return
	}
	if len(events) == 0 {
		b.send(message.Chat.ID, "No events in namespace "+code(namespace))
		return
	}

	// Show the most recent 20.
	if len(events) > 20 {
		events = events[len(events)-20:]
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Recent events in %s</b>\n\n", htmlEscape(namespace)))
	for i := range events {
		e := &events[i]
		icon := "ℹ️"
		if e.Type == "Warning" {
			icon = "⚠️"
		}
		sb.WriteString(fmt.Sprintf("%s %s/%s — %s\n   %s\n\n",
			icon, htmlEscape(e.InvolvedObject.Kind), htmlEscape(e.InvolvedObject.Name),
			htmlEscape(e.Reason), htmlEscape(e.Message)))
	}
	b.send(message.Chat.ID, sb.String())
}

// handleTop handles /top [namespace]
func (b *Bot) handleTop(ctx context.Context, message *tgbotapi.Message) {
	p := parseArgs(message.CommandArguments())
	namespace := rbac.NormalizeNamespace(firstOr(p.positional, p.namespace))

	if _, _, ok := b.resolveListAccess(ctx, message.Chat.ID, message.From.ID, namespace, "pods", ""); !ok {
		return
	}

	metrics, err := b.k8sClient.GetPodMetrics(ctx, namespace)
	if err != nil {
		b.send(message.Chat.ID, "📊 Pod metrics unavailable. Is metrics-server installed in the cluster?")
		return
	}
	if len(metrics) == 0 {
		b.send(message.Chat.ID, "No pod metrics in namespace "+code(namespace))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Resource usage in %s</b>\n\n", htmlEscape(namespace)))
	for _, m := range metrics {
		sb.WriteString(fmt.Sprintf("📦 %s\n   CPU %s • Mem %s\n", code(m.Name), htmlEscape(m.CPU), htmlEscape(m.Memory)))
	}
	b.send(message.Chat.ID, sb.String())
}

// handleRestart handles the /restart command
func (b *Bot) handleRestart(ctx context.Context, message *tgbotapi.Message) {
	b.requestDeploymentAction(ctx, message, actionRestart)
}

// handleRollback handles the /rollback command
func (b *Bot) handleRollback(ctx context.Context, message *tgbotapi.Message) {
	b.requestDeploymentAction(ctx, message, actionRollback)
}

// requestDeploymentAction validates a restart/rollback and prompts for
// confirmation before executing.
func (b *Bot) requestDeploymentAction(ctx context.Context, message *tgbotapi.Message, kind string) {
	p := parseArgs(message.CommandArguments())
	if len(p.positional) == 0 {
		b.send(message.Chat.ID, fmt.Sprintf("Usage: /%s &lt;deployment&gt; [-n ns]", kind))
		return
	}
	name := p.positional[0]
	namespace := rbac.NormalizeNamespace(p.namespace)

	dec, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: message.From.ID,
		Namespace:      namespace,
		Resource:       "deployments",
		Verb:           kind,
		ResourceName:   name,
	})
	if err != nil || !dec.Allowed {
		b.send(message.Chat.ID, rbac.FormatPermissionDenied(dec.Reason))
		return
	}

	summary := fmt.Sprintf("%s deployment %s in namespace %s?", titleCase(kind), code(name), code(namespace))
	b.requestConfirmation(message.Chat.ID, message.From.ID, &pendingAction{
		kind: kind, namespace: namespace, name: name,
	}, summary)
}

// handleScale handles the /scale command
func (b *Bot) handleScale(ctx context.Context, message *tgbotapi.Message) {
	p := parseArgs(message.CommandArguments())
	if len(p.positional) < 2 {
		b.send(message.Chat.ID, "Usage: /scale &lt;deployment&gt; &lt;replicas&gt; [-n ns]")
		return
	}
	name := p.positional[0]
	replicas, err := strconv.ParseInt(p.positional[1], 10, 32)
	if err != nil || replicas < 0 {
		b.send(message.Chat.ID, "❌ Replica count must be a non-negative integer.")
		return
	}
	if replicas > maxReplicas {
		b.send(message.Chat.ID, fmt.Sprintf("❌ Replica count exceeds the safety limit of %d.", maxReplicas))
		return
	}
	namespace := rbac.NormalizeNamespace(p.namespace)

	dec, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
		TelegramUserID: message.From.ID,
		Namespace:      namespace,
		Resource:       "deployments",
		Verb:           "scale",
		ResourceName:   name,
	})
	if err != nil || !dec.Allowed {
		b.send(message.Chat.ID, rbac.FormatPermissionDenied(dec.Reason))
		return
	}

	summary := fmt.Sprintf("Scale deployment %s to %d replicas in namespace %s?", code(name), replicas, code(namespace))
	b.requestConfirmation(message.Chat.ID, message.From.ID, &pendingAction{
		kind: actionScale, namespace: namespace, name: name, replicas: int32(replicas),
	}, summary)
}

// handleGrant handles the /grant command (admin only)
func (b *Bot) handleGrant(ctx context.Context, message *tgbotapi.Message) {
	if !b.isAdmin(ctx, message.From.ID) {
		b.send(message.Chat.ID, "❌ Admin access required.")
		return
	}
	p := parseArgs(message.CommandArguments())
	if len(p.positional) < 3 {
		b.send(message.Chat.ID, "Usage: /grant &lt;user_id&gt; &lt;verb&gt; &lt;resource&gt; [-n ns] [-l selector]")
		return
	}
	targetUserID, err := strconv.ParseInt(p.positional[0], 10, 64)
	if err != nil {
		b.send(message.Chat.ID, "❌ Invalid user ID.")
		return
	}
	verb, resource := p.positional[1], p.positional[2]
	namespace := p.namespace
	if namespace == "" {
		namespace = "*"
	}

	if err := b.rbac.GrantPermission(ctx, targetUserID, namespace, resource, verb, p.selector); err != nil {
		b.fail(message.Chat.ID, "Grant permission", err)
		return
	}
	b.auditPermissionChange(ctx, message.From.ID, targetUserID, "PermissionGranted",
		fmt.Sprintf("granted %s %s in %s", verb, resource, namespace))

	resp := fmt.Sprintf("✅ Granted to user %s\n\nNamespace: %s\nResource: %s\nVerb: %s",
		code(p.positional[0]), code(namespace), code(resource), code(verb))
	if p.selector != "" {
		resp += "\nSelector: " + code(p.selector)
	}
	b.send(message.Chat.ID, resp)
}

// handleRevoke handles the /revoke command (admin only)
func (b *Bot) handleRevoke(ctx context.Context, message *tgbotapi.Message) {
	if !b.isAdmin(ctx, message.From.ID) {
		b.send(message.Chat.ID, "❌ Admin access required.")
		return
	}
	p := parseArgs(message.CommandArguments())
	if len(p.positional) < 3 {
		b.send(message.Chat.ID, "Usage: /revoke &lt;user_id&gt; &lt;verb&gt; &lt;resource&gt; -n ns")
		return
	}
	targetUserID, err := strconv.ParseInt(p.positional[0], 10, 64)
	if err != nil {
		b.send(message.Chat.ID, "❌ Invalid user ID.")
		return
	}
	verb, resource := p.positional[1], p.positional[2]
	if p.namespace == "" {
		b.send(message.Chat.ID, "❌ Namespace is required (-n &lt;namespace&gt;).")
		return
	}

	if err := b.rbac.RevokePermission(ctx, targetUserID, p.namespace, resource, verb); err != nil {
		b.fail(message.Chat.ID, "Revoke permission", err)
		return
	}
	b.auditPermissionChange(ctx, message.From.ID, targetUserID, "PermissionRevoked",
		fmt.Sprintf("revoked %s %s in %s", verb, resource, p.namespace))
	b.send(message.Chat.ID, "✅ Revoked "+code(verb+" "+resource)+" in "+code(p.namespace)+" from user "+code(p.positional[0]))
}

// handleRole sets or removes a user's role binding (admin only).
//
//	/role <user_id> <admin|operator|viewer> [-n ns] [-l selector]
//	/role <user_id> none [-n ns]
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

// handlePermissions handles the /permissions command
func (b *Bot) handlePermissions(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	p := parseArgs(message.CommandArguments())

	targetUserID := userID
	if len(p.positional) > 0 {
		if !b.isAdmin(ctx, userID) {
			b.send(message.Chat.ID, "❌ Admin access required to view other users' permissions.")
			return
		}
		var err error
		targetUserID, err = strconv.ParseInt(p.positional[0], 10, 64)
		if err != nil {
			b.send(message.Chat.ID, "❌ Invalid user ID.")
			return
		}
	}

	summary, err := b.rbac.GetPermissionSummary(ctx, targetUserID)
	if err != nil {
		b.send(message.Chat.ID, "No permissions found for that user.")
		return
	}
	b.send(message.Chat.ID, "<b>Permissions</b>\n"+pre(summary))
}

// handleUsers handles the /users command (admin only)
func (b *Bot) handleUsers(ctx context.Context, message *tgbotapi.Message) {
	if !b.isAdmin(ctx, message.From.ID) {
		b.send(message.Chat.ID, "❌ Admin access required.")
		return
	}
	perms, err := b.rbac.ListUserPermissions(ctx)
	if err != nil {
		b.fail(message.Chat.ID, "List users", err)
		return
	}
	if len(perms) == 0 {
		b.send(message.Chat.ID, "No users have permissions yet.")
		return
	}

	sort.Slice(perms, func(i, j int) bool {
		return perms[i].Spec.TelegramUserID < perms[j].Spec.TelegramUserID
	})
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Users with permissions</b> (%d)\n\n", len(perms)))
	for i := range perms {
		s := perms[i].Spec
		sb.WriteString(fmt.Sprintf("👤 %s — role %s — %d rule(s)\n",
			code(strconv.FormatInt(s.TelegramUserID, 10)), htmlEscape(s.Role), len(s.Permissions)))
	}
	b.send(message.Chat.ID, sb.String())
}

// handleSelfUpdate handles the /selfupdate command (admin only)
func (b *Bot) handleSelfUpdate(ctx context.Context, message *tgbotapi.Message) {
	if !b.isAdmin(ctx, message.From.ID) {
		b.send(message.Chat.ID, "❌ Admin access required.")
		return
	}
	summary := fmt.Sprintf("Restart the bot deployment %s in %s to pull the latest image?",
		code(b.config.BotDeploymentName), code(b.config.BotNamespace))
	b.requestConfirmation(message.Chat.ID, message.From.ID, &pendingAction{
		kind: actionSelfUpdate, namespace: b.config.BotNamespace, name: b.config.BotDeploymentName,
	}, summary)
}

// firstOr returns the first positional argument, or fallback if none.
func firstOr(positional []string, fallback string) string {
	if len(positional) > 0 {
		return positional[0]
	}
	return fallback
}
