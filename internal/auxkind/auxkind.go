// Package auxkind translates an aux kind between the enum the resource API
// speaks and the name the runtime API and the controller use.
//
// The two surfaces name the same thing differently on purpose: a public API
// should refuse a kind it does not know, which an enum does by itself, and the
// runtime view models have always carried kinds as strings.
package auxkind

import "github.com/lesomnus/cxz/resource"

const (
	Summary    = "summary"
	Suggestion = "suggestion"
	Title      = "title"
)

var names = map[resource.AuxKind]string{
	resource.AuxKind_AUX_KIND_SUMMARY:    Summary,
	resource.AuxKind_AUX_KIND_SUGGESTION: Suggestion,
	resource.AuxKind_AUX_KIND_TITLE:      Title,
}

// Name is the empty string for a kind this build does not know, which is how a
// caller from a newer client is refused rather than silently given a summary.
func Name(k resource.AuxKind) string { return names[k] }

func Of(name string) resource.AuxKind {
	for k, n := range names {
		if n == name {
			return k
		}
	}
	return resource.AuxKind_AUX_KIND_UNSPECIFIED
}

func Names(kinds []resource.AuxKind) []string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		if n := Name(k); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func Enums(names []string) []resource.AuxKind {
	out := make([]resource.AuxKind, 0, len(names))
	for _, n := range names {
		if k := Of(n); k != resource.AuxKind_AUX_KIND_UNSPECIFIED {
			out = append(out, k)
		}
	}
	return out
}
