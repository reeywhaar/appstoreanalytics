package hooks

import "testing"

// Every kind is in the catalog once, so choosing it is one box, and every group has some.
func TestEveryKindIsListedOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, service := range Catalog {
		for _, g := range service.Groups {
			if len(g.Kinds) == 0 {
				t.Errorf("%s, %s: empty", service.Name, g.Name)
			}
			for _, k := range g.Kinds {
				if seen[k.Name] {
					t.Errorf("%s is listed twice", k.Name)
				}
				seen[k.Name] = true
				if k.Meaning == "" {
					t.Errorf("%s means nothing", k.Name)
				}
			}
		}
	}
	if len(seen) != 37 {
		t.Errorf("%d kinds, want Apple's 24 notification types and 13 webhook events", len(seen))
	}
}
