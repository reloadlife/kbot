package k8s

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ListEvents returns events in a namespace, most recent last.
func (c *Client) ListEvents(ctx context.Context, namespace string) ([]corev1.Event, error) {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}
	list, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	items := list.Items
	sort.Slice(items, func(i, j int) bool {
		return items[i].LastTimestamp.Before(&items[j].LastTimestamp)
	})
	return items, nil
}

// RecordEvent emits a Kubernetes Event for audit purposes. Failures are
// returned but callers typically treat auditing as best-effort.
func (c *Client) RecordEvent(ctx context.Context, namespace, involvedKind, involvedName, reason, message string) error {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}
	now := metav1.Now()
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kbot-",
			Namespace:    namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      involvedKind,
			Namespace: namespace,
			Name:      involvedName,
		},
		Reason:         reason,
		Message:        message,
		Type:           corev1.EventTypeNormal,
		Source:         corev1.EventSource{Component: "kubectl-bot"},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	_, err := c.clientset.CoreV1().Events(namespace).Create(ctx, event, metav1.CreateOptions{})
	return err
}

// PodMetric holds CPU/memory usage for a single pod.
type PodMetric struct {
	Name   string
	CPU    string
	Memory string
}

// podMetricsGVR is the metrics-server resource for pod metrics.
func podMetricsGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
}

// GetPodMetrics returns per-pod resource usage from metrics-server. Returns an
// error (e.g. NotFound) when metrics-server is not installed.
func (c *Client) GetPodMetrics(ctx context.Context, namespace string) ([]PodMetric, error) {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}
	list, err := c.dynamicClient.Resource(podMetricsGVR()).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var out []PodMetric
	for _, item := range list.Items {
		name, _, _ := unstructuredString(item.Object, "metadata", "name")
		containers, ok := item.Object["containers"].([]interface{})
		if !ok {
			continue
		}
		// Sum is non-trivial without quantity math; report the first/primary
		// container's usage, which is the common case for these pods.
		cpu, mem := "?", "?"
		if len(containers) > 0 {
			if cm, ok := containers[0].(map[string]interface{}); ok {
				if usage, ok := cm["usage"].(map[string]interface{}); ok {
					if v, ok := usage["cpu"].(string); ok {
						cpu = v
					}
					if v, ok := usage["memory"].(string); ok {
						mem = v
					}
				}
			}
		}
		out = append(out, PodMetric{Name: name, CPU: cpu, Memory: mem})
	}
	return out, nil
}

// unstructuredString safely walks a nested map for a string value.
func unstructuredString(obj map[string]interface{}, path ...string) (string, bool, error) {
	cur := obj
	for i, key := range path {
		if i == len(path)-1 {
			s, ok := cur[key].(string)
			return s, ok, nil
		}
		next, ok := cur[key].(map[string]interface{})
		if !ok {
			return "", false, fmt.Errorf("path %v not found", path)
		}
		cur = next
	}
	return "", false, nil
}
