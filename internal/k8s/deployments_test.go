package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func deployment(uid, image string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web",
			Namespace: "default",
			UID:       "deploy-uid",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: image}}},
			},
		},
	}
}

func replicaSet(name, revision, image string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Labels:      map[string]string{"app": "web"},
			Annotations: map[string]string{revisionAnnotation: revision},
			OwnerReferences: []metav1.OwnerReference{{
				UID:        "deploy-uid",
				Controller: boolPtr(true),
				Kind:       "Deployment",
				Name:       "web",
			}},
		},
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web", "pod-template-hash": name}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: image}}},
			},
		},
	}
}

func boolPtr(b bool) *bool { return &b }

// TestRollbackPicksPreviousRevision verifies the fix for the rollback no-op
// bug: the deployment's template must be restored from the second-highest
// revision (v2 → v1), not the current one, with pod-template-hash stripped.
func TestRollbackPicksPreviousRevision(t *testing.T) {
	dep := deployment("deploy-uid", "web:v3") // current = revision 3
	rsOld := replicaSet("web-aaa", "1", "web:v1")
	rsPrev := replicaSet("web-bbb", "2", "web:v2") // previous
	rsCur := replicaSet("web-ccc", "3", "web:v3")  // current

	cs := fake.NewSimpleClientset(dep, rsOld, rsPrev, rsCur)
	c := NewClientWithInterfaces(cs, nil)

	if err := c.RollbackDeployment(context.Background(), "default", "web"); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	got, err := c.GetDeployment(context.Background(), "default", "web")
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if img := got.Spec.Template.Spec.Containers[0].Image; img != "web:v2" {
		t.Fatalf("expected rollback to web:v2, got %s", img)
	}
	if _, ok := got.Spec.Template.Labels["pod-template-hash"]; ok {
		t.Fatalf("pod-template-hash should be stripped from restored template")
	}
}

func TestRollbackNoPreviousRevision(t *testing.T) {
	dep := deployment("deploy-uid", "web:v1")
	rsCur := replicaSet("web-ccc", "1", "web:v1")
	cs := fake.NewSimpleClientset(dep, rsCur)
	c := NewClientWithInterfaces(cs, nil)

	if err := c.RollbackDeployment(context.Background(), "default", "web"); err == nil {
		t.Fatalf("expected error when no previous revision exists")
	}
}
