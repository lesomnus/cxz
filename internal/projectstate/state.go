// Package projectstate translates between the words the manager uses for a
// container's state and the resource API's enum.
//
// It exists because the two halves of cxz name the same derived fact
// differently while the transition runs: the manager answers its runtime API in
// strings, the resource API answers clients in an enum. The fact is derived
// from the container inventory either way and stored in neither.
// See docs/plans/project-lifecycle-api.md.
package projectstate

import "github.com/lesomnus/cxz/resource"

// Of is the enum for a word the manager used. Anything it does not recognise
// is unspecified rather than a guess -- including `foreign`, which is not a
// state of a project's container but a container that belongs to no project.
func Of(state string) resource.ContainerState {
	switch state {
	case "absent":
		return resource.ContainerState_CONTAINER_STATE_ABSENT
	case "created":
		return resource.ContainerState_CONTAINER_STATE_CREATED
	case "running":
		return resource.ContainerState_CONTAINER_STATE_RUNNING
	case "stopped":
		return resource.ContainerState_CONTAINER_STATE_STOPPED
	}
	return resource.ContainerState_CONTAINER_STATE_UNSPECIFIED
}

// Word is what the manager's runtime API calls a state. The unspecified enum
// becomes `unknown`, which is what it means: nothing was observed.
func Word(state resource.ContainerState) string {
	switch state {
	case resource.ContainerState_CONTAINER_STATE_ABSENT:
		return "absent"
	case resource.ContainerState_CONTAINER_STATE_CREATED:
		return "created"
	case resource.ContainerState_CONTAINER_STATE_RUNNING:
		return "running"
	case resource.ContainerState_CONTAINER_STATE_STOPPED:
		return "stopped"
	}
	return "unknown"
}
