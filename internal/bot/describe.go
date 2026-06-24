package bot

import (
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// titleCase upper-cases the first rune of s (ASCII-sufficient for our verbs).
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// formatLabels renders a label map deterministically as k=v,k=v.
func formatLabels(m map[string]string) string {
	if len(m) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
}

// describePod produces an HTML summary of a pod's key fields.
func describePod(pod *corev1.Pod) string {
	var sb strings.Builder
	sb.WriteString("<b>Pod: " + htmlEscape(pod.Name) + "</b>\n")
	sb.WriteString("Namespace: " + code(pod.Namespace) + "\n")
	sb.WriteString("Node: " + code(orNone(pod.Spec.NodeName)) + "\n")
	sb.WriteString("Pod IP: " + code(orNone(pod.Status.PodIP)) + "\n")
	sb.WriteString("Phase: " + htmlEscape(string(pod.Status.Phase)) + "\n")
	sb.WriteString("Age: " + formatAge(pod.CreationTimestamp.Time) + "\n")
	sb.WriteString("Labels: " + code(formatLabels(pod.Labels)) + "\n\n")

	sb.WriteString("<b>Containers:</b>\n")
	for _, c := range pod.Spec.Containers {
		sb.WriteString(fmt.Sprintf("• %s — %s\n", htmlEscape(c.Name), code(c.Image)))
	}
	for _, cs := range pod.Status.ContainerStatuses {
		state := "waiting"
		switch {
		case cs.State.Running != nil:
			state = "running"
		case cs.State.Terminated != nil:
			state = "terminated: " + cs.State.Terminated.Reason
		case cs.State.Waiting != nil:
			state = "waiting: " + cs.State.Waiting.Reason
		}
		sb.WriteString(fmt.Sprintf("   %s: %s (restarts %d)\n", htmlEscape(cs.Name), htmlEscape(state), cs.RestartCount))
	}
	return sb.String()
}

// describeDeployment produces an HTML summary of a deployment's key fields.
func describeDeployment(dep *appsv1.Deployment) string {
	desired := int32(0)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}

	var sb strings.Builder
	sb.WriteString("<b>Deployment: " + htmlEscape(dep.Name) + "</b>\n")
	sb.WriteString("Namespace: " + code(dep.Namespace) + "\n")
	sb.WriteString(fmt.Sprintf("Replicas: %d desired / %d ready / %d available / %d updated\n",
		desired, dep.Status.ReadyReplicas, dep.Status.AvailableReplicas, dep.Status.UpdatedReplicas))
	sb.WriteString("Age: " + formatAge(dep.CreationTimestamp.Time) + "\n")
	if dep.Spec.Selector != nil {
		sb.WriteString("Selector: " + code(formatLabels(dep.Spec.Selector.MatchLabels)) + "\n")
	}
	sb.WriteString("\n<b>Containers:</b>\n")
	for _, c := range dep.Spec.Template.Spec.Containers {
		sb.WriteString(fmt.Sprintf("• %s — %s\n", htmlEscape(c.Name), code(c.Image)))
	}

	sb.WriteString("\n<b>Conditions:</b>\n")
	for _, cond := range dep.Status.Conditions {
		sb.WriteString(fmt.Sprintf("• %s=%s: %s\n", htmlEscape(string(cond.Type)), htmlEscape(string(cond.Status)), htmlEscape(cond.Message)))
	}
	return sb.String()
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
