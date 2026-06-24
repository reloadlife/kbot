package k8s

import (
	"context"
	"fmt"
	"sort"
	"strconv"

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

const revisionAnnotation = "deployment.kubernetes.io/revision"

// rsRevision returns the integer revision of a ReplicaSet, or -1 if unset.
func rsRevision(rs *appsv1.ReplicaSet) int64 {
	v, ok := rs.Annotations[revisionAnnotation]
	if !ok {
		return -1
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// RollbackDeployment rolls a deployment back to its previous revision by
// restoring the pod template from the ReplicaSet with the second-highest
// revision number among the ReplicaSets this deployment owns.
func (c *Client) RollbackDeployment(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		namespace = corev1.NamespaceDefault
	}

	deployment, err := c.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}

	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return fmt.Errorf("invalid deployment selector: %w", err)
	}

	rsList, err := c.clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to list replica sets: %w", err)
	}

	// Keep only ReplicaSets actually owned by this deployment.
	owned := make([]*appsv1.ReplicaSet, 0, len(rsList.Items))
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		if metav1.IsControlledBy(rs, deployment) {
			owned = append(owned, rs)
		}
	}

	// Highest revision first.
	sort.Slice(owned, func(i, j int) bool {
		return rsRevision(owned[i]) > rsRevision(owned[j])
	})

	if len(owned) < 2 {
		return fmt.Errorf("no previous revision to roll back to")
	}

	previousRS := owned[1]

	// Restore the previous template, dropping the controller-managed
	// pod-template-hash label so the deployment controller recomputes it.
	template := *previousRS.Spec.Template.DeepCopy()
	delete(template.Labels, "pod-template-hash")
	deployment.Spec.Template = template

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
