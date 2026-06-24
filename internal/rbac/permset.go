package rbac

// Pure helpers for manipulating permission bundles. A Permission grants the
// cross product of its Resources and Verbs within a namespace/selector. These
// functions keep grant/revoke semantically correct over that bundle model.

// exactContains reports whether slice holds item verbatim (no wildcard magic).
func exactContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// without returns a copy of slice with all occurrences of item removed.
func without(slice []string, item string) []string {
	out := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			out = append(out, s)
		}
	}
	return out
}

func cloneSlice(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}

// removeCapability removes the single (resource, verb) capability from every
// permission bundle in namespace ns. Because a bundle is a cross product,
// removing one pair may split a bundle into two complementary bundles:
//
//	R × V  minus (r,v)  ==  R × (V∖{v})  ∪  (R∖{r}) × {v}
//
// Entries that become empty are dropped. Entries that do not match ns, or do
// not contain both r and v, are preserved unchanged.
func removeCapability(perms []Permission, ns, resource, verb string) []Permission {
	out := make([]Permission, 0, len(perms))
	for _, p := range perms {
		if p.Namespace != ns || !exactContains(p.Resources, resource) || !exactContains(p.Verbs, verb) {
			out = append(out, p)
			continue
		}

		if verbsLeft := without(p.Verbs, verb); len(verbsLeft) > 0 {
			out = append(out, Permission{
				Namespace: p.Namespace,
				Resources: cloneSlice(p.Resources),
				Verbs:     verbsLeft,
				Selector:  p.Selector,
			})
		}
		if resLeft := without(p.Resources, resource); len(resLeft) > 0 {
			out = append(out, Permission{
				Namespace: p.Namespace,
				Resources: resLeft,
				Verbs:     []string{verb},
				Selector:  p.Selector,
			})
		}
	}
	return out
}
