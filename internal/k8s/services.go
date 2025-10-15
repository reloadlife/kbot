package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ListServices lists services in a namespace with optional label selector
func (c *Client) ListServices(ctx context.Context, namespace string, selector string) (*corev1.ServiceList, error) {
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

	return c.clientset.CoreV1().Services(namespace).List(ctx, opts)
}

// GetService gets a specific service
func (c *Client) GetService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	return c.clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
}
