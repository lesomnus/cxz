// Package enginemode translates the shared Docker engine's mode between the
// enum the resource API speaks and the name the settings file and the engine
// use, the same way auxkind and mcpkind do for their own kinds.
//
// off and dind are the whole set, which is what makes an enum right here: the
// operations that used to travel as strings beside it were not a set at all.
package enginemode

import "github.com/lesomnus/cxz/resource"

const (
	Off  = "off"
	Dind = "dind"
)

var names = map[resource.EngineMode]string{
	resource.EngineMode_ENGINE_MODE_OFF:  Off,
	resource.EngineMode_ENGINE_MODE_DIND: Dind,
}

// Name is the empty string for a mode this build does not know, so a
// configuration from a newer client is refused rather than saved as a mode it
// is not.
func Name(m resource.EngineMode) string { return names[m] }

// Of is UNSPECIFIED for a name this build does not know, and for the empty
// name a settings file leaves when it has said nothing.
func Of(name string) resource.EngineMode {
	for m, n := range names {
		if n == name {
			return m
		}
	}
	return resource.EngineMode_ENGINE_MODE_UNSPECIFIED
}
