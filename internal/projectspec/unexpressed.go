package projectspec

import "slices"

// Unexpressed is a devcontainer feature a Spec cannot carry. The Docker path
// still honours every one of them, because the devcontainer CLI runs the
// project's own configuration next to the Spec. A backend that runs only the
// Spec -- a Kubernetes pod -- has to refuse a configuration that uses them, and
// say what to do instead. See cxz#372, cxz#374.
type Unexpressed struct {
	// Key is the devcontainer.json property.
	Key string
	// Why is what the Spec would need and does not have.
	Why string
	// Instead is what a project can do on a backend that runs only the Spec.
	Instead string
}

var unexpressed = []Unexpressed{
	{"build", "a Spec names a prebuilt image; building is the devcontainer CLI's job", "build and push the image, then name it with \"image\""},
	{"dockerFile", "a Spec names a prebuilt image; building is the devcontainer CLI's job", "build and push the image, then name it with \"image\""},
	{"context", "a Spec names a prebuilt image; building is the devcontainer CLI's job", "build and push the image, then name it with \"image\""},
	{"features", "features are installed by building an image", "bake the features into the image"},
	{"dockerComposeFile", "a Spec is one container; Compose adds services, networks and build", "use an image-based devcontainer; run other services separately"},
	{"service", "a Spec is one container; Compose adds services, networks and build", "use an image-based devcontainer; run other services separately"},
	{"runServices", "a Spec is one container; Compose adds services, networks and build", "use an image-based devcontainer; run other services separately"},
	{"initializeCommand", "lifecycle hooks are run by the devcontainer CLI; the runtime starts as the container's command", "do the work in the image, or from a session"},
	{"onCreateCommand", "lifecycle hooks are run by the devcontainer CLI; the runtime starts as the container's command", "do the work in the image, or from a session"},
	{"updateContentCommand", "lifecycle hooks are run by the devcontainer CLI; the runtime starts as the container's command", "do the work in the image, or from a session"},
	{"postCreateCommand", "lifecycle hooks are run by the devcontainer CLI; the runtime starts as the container's command", "do the work in the image, or from a session"},
	{"postStartCommand", "lifecycle hooks are run by the devcontainer CLI; the runtime starts as the container's command", "do the work in the image, or from a session"},
	{"postAttachCommand", "lifecycle hooks are run by the devcontainer CLI; the runtime starts as the container's command", "do the work in the image, or from a session"},
	{"runArgs", "docker run arguments have no meaning outside Docker", "remove them; resources are declared separately (cxz#375)"},
	{"privileged", "a privileged container is a property of the Docker host", "remove it"},
	{"capAdd", "capabilities are a property of the Docker host", "remove it"},
	{"securityOpt", "security options are a property of the Docker host", "remove it"},
	{"init", "the runtime is the container's first process", "remove it"},
	{"overrideCommand", "the runtime is the container's command", "remove it"},
	{"mounts", "a project mount names a path or volume on the Docker host", "keep what the project needs in the workspace or the image"},
	{"workspaceMount", "the workspace is provided by the backend", "remove it"},
	{"appPort", "published ports belong to the Docker host", "remove it"},
	{"hostRequirements", "resources are declared separately", "declare requests and limits as a project resource setting (cxz#375)"},
}

// UnexpressedIn is every property of a devcontainer configuration that a Spec
// cannot carry, in the order of the list above.
func UnexpressedIn(cfg map[string]any) []Unexpressed {
	var out []Unexpressed
	for _, u := range unexpressed {
		if _, ok := cfg[u.Key]; ok {
			out = append(out, u)
		}
	}
	return out
}

// UnexpressedKeys is the property names UnexpressedIn checks.
func UnexpressedKeys() []string {
	out := make([]string, 0, len(unexpressed))
	for _, u := range unexpressed {
		out = append(out, u.Key)
	}
	return slices.Clip(out)
}
