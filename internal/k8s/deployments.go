package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ListDeployments lists deployments in a namespace with optional label selector
func (c *Client) ListDeployments(ctx context.Context, namespace string, selector string) (*appsv1.DeploymentList, error) {
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

	return c.clientset.AppsV1().Deployments(namespace).List(ctx, opts)
}

// GetDeployment gets a specific deployment
func (c *Client) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	return c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

// RestartDeployment restarts a deployment by updating its annotation
func (c *Client) RestartDeployment(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	deployment, err := c.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}

	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = metav1.Now().Format("2006-01-02T15:04:05Z07:00")

	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

// RollbackDeployment rolls back a deployment to the previous revision
func (c *Client) RollbackDeployment(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	// Get current deployment
	deployment, err := c.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Get replica sets
	rsList, err := c.clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set(deployment.Spec.Selector.MatchLabels).String(),
	})
	if err != nil {
		return fmt.Errorf("failed to list replica sets: %w", err)
	}

	if len(rsList.Items) < 2 {
		return fmt.Errorf("no previous revision found")
	}

	// Find previous revision (second most recent)
	var previousRS *appsv1.ReplicaSet
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		if rs.Name != deployment.Name && rs.Annotations["deployment.kubernetes.io/revision"] != "" {
			if previousRS == nil || rs.CreationTimestamp.After(previousRS.CreationTimestamp.Time) {
				previousRS = rs
			}
		}
	}

	if previousRS == nil {
		return fmt.Errorf("no previous revision found")
	}

	// Update deployment to use previous template
	deployment.Spec.Template = previousRS.Spec.Template
	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

// ScaleDeployment scales a deployment to the specified number of replicas
func (c *Client) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	deployment, err := c.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}

	deployment.Spec.Replicas = &replicas
	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

// DeploymentMatchesSelector checks if a deployment matches the given label selector
func (c *Client) DeploymentMatchesSelector(ctx context.Context, namespace, deploymentName, selector string) (bool, error) {
	if selector == "" {
		return true, nil
	}

	deployment, err := c.GetDeployment(ctx, namespace, deploymentName)
	if err != nil {
		return false, err
	}

	labelSelector, err := labels.Parse(selector)
	if err != nil {
		return false, fmt.Errorf("invalid selector: %w", err)
	}

	return labelSelector.Matches(labels.Set(deployment.Labels)), nil
}
