package k8s

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ListPods lists pods in a namespace with optional label selector
func (c *Client) ListPods(ctx context.Context, namespace string, selector string) (*corev1.PodList, error) {
	opts := metav1.ListOptions{}

	if selector != "" {
		// Validate selector
		if _, err := labels.Parse(selector); err != nil {
			return nil, fmt.Errorf("invalid label selector '%s': %w", selector, err)
		}
		opts.LabelSelector = selector
	}

	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	return c.clientset.CoreV1().Pods(namespace).List(ctx, opts)
}

// GetPod gets a specific pod
func (c *Client) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	return c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

// LogOptions controls how pod logs are fetched.
type LogOptions struct {
	Container    string // empty = default/first container
	TailLines    int64  // number of trailing lines
	Previous     bool   // logs from the previous container instance
	SinceSeconds int64  // 0 = no time bound
}

// GetPodLogs retrieves logs from a pod using the supplied options.
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName string, lo LogOptions) (string, error) {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	opts := &corev1.PodLogOptions{
		Container: lo.Container,
		Previous:  lo.Previous,
	}
	if lo.TailLines > 0 {
		opts.TailLines = &lo.TailLines
	}
	if lo.SinceSeconds > 0 {
		opts.SinceSeconds = &lo.SinceSeconds
	}

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		// Return whatever we managed to read alongside the error so the caller
		// can decide; never silently drop a non-EOF failure.
		return string(data), fmt.Errorf("failed reading log stream: %w", err)
	}
	return string(data), nil
}

// PodMatchesSelector checks if a pod matches the given label selector
func (c *Client) PodMatchesSelector(ctx context.Context, namespace, podName, selector string) (bool, error) {
	if selector == "" {
		return true, nil
	}

	pod, err := c.GetPod(ctx, namespace, podName)
	if err != nil {
		return false, err
	}

	labelSelector, err := labels.Parse(selector)
	if err != nil {
		return false, fmt.Errorf("invalid selector: %w", err)
	}

	return labelSelector.Matches(labels.Set(pod.Labels)), nil
}
