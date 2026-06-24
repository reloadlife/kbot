package rbac

import (
	"reflect"
	"sort"
	"testing"
)

// normalize sorts a permission list and its inner slices so comparisons are
// order-independent.
func normalize(perms []Permission) []Permission {
	out := make([]Permission, len(perms))
	for i, p := range perms {
		r := cloneSlice(p.Resources)
		v := cloneSlice(p.Verbs)
		sort.Strings(r)
		sort.Strings(v)
		out[i] = Permission{Namespace: p.Namespace, Resources: r, Verbs: v, Selector: p.Selector}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return len(out[i].Resources) < len(out[j].Resources)
	})
	return out
}

func TestRemoveCapability_SplitsBundle(t *testing.T) {
	// Bundle granting {pods,deployments} × {get,list}. Revoke (pods, get).
	// Remaining capability must be everything except (pods,get):
	//   {pods,deployments}×{list}  +  {deployments}×{get}
	in := []Permission{{
		Namespace: "prod",
		Resources: []string{"pods", "deployments"},
		Verbs:     []string{"get", "list"},
	}}

	got := removeCapability(in, "prod", "pods", "get")

	want := []Permission{
		{Namespace: "prod", Resources: []string{"deployments"}, Verbs: []string{"get"}},
		{Namespace: "prod", Resources: []string{"pods", "deployments"}, Verbs: []string{"list"}},
	}

	if !reflect.DeepEqual(normalize(got), normalize(want)) {
		t.Fatalf("got %+v, want %+v", normalize(got), normalize(want))
	}
}

func TestRemoveCapability_RemovesEmptyEntry(t *testing.T) {
	in := []Permission{{
		Namespace: "prod",
		Resources: []string{"pods"},
		Verbs:     []string{"get"},
	}}
	got := removeCapability(in, "prod", "pods", "get")
	if len(got) != 0 {
		t.Fatalf("expected entry fully removed, got %+v", got)
	}
}

func TestRemoveCapability_NamespaceScoped(t *testing.T) {
	in := []Permission{
		{Namespace: "prod", Resources: []string{"pods"}, Verbs: []string{"get"}},
		{Namespace: "staging", Resources: []string{"pods"}, Verbs: []string{"get"}},
	}
	got := removeCapability(in, "prod", "pods", "get")
	want := []Permission{{Namespace: "staging", Resources: []string{"pods"}, Verbs: []string{"get"}}}
	if !reflect.DeepEqual(normalize(got), normalize(want)) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRemoveCapability_NoMatchUnchanged(t *testing.T) {
	in := []Permission{{Namespace: "prod", Resources: []string{"pods"}, Verbs: []string{"list"}}}
	got := removeCapability(in, "prod", "pods", "get") // verb not present
	if !reflect.DeepEqual(normalize(got), normalize(in)) {
		t.Fatalf("expected unchanged, got %+v", got)
	}
}

func TestRemoveCapability_PreservesSelector(t *testing.T) {
	in := []Permission{{
		Namespace: "prod",
		Resources: []string{"pods", "deployments"},
		Verbs:     []string{"get"},
		Selector:  "app=frontend",
	}}
	got := removeCapability(in, "prod", "pods", "get")
	want := []Permission{{Namespace: "prod", Resources: []string{"deployments"}, Verbs: []string{"get"}, Selector: "app=frontend"}}
	if !reflect.DeepEqual(normalize(got), normalize(want)) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
