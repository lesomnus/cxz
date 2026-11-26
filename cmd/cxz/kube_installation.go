package main

import (
	"context"
	"fmt"
	"os"

	"github.com/lesomnus/xli"
)

// kubernetesManaged are the commands that install, replace or remove the
// manager itself. On Kubernetes the manager is a Deployment someone else
// declares -- its version is the image tag in it, changed in the deployment's
// own source and rolled out by the cluster -- so cxz rewriting itself would be
// a second, undeclared owner of the same thing.
var kubernetesManaged = []string{"install", "uninstall", "self-update", "use", "purge"}

const kubernetesVersionRefusal = "this cxz manager runs on Kubernetes, where its version is the image tag of its Deployment: change the tag where the deployment is declared (a kustomize overlay's images: newTag) and let the cluster roll it out; project pods take the new runtime on their next start. See docs/kubernetes.md#updates"

func refuseOnKubernetes(root *xli.Command) {
	for _, c := range root.Commands {
		for _, name := range kubernetesManaged {
			if c.Name == name {
				guardTree(c)
			}
		}
	}
}

func guardTree(c *xli.Command) {
	if c.Handler != nil {
		c.Handler = xli.Chain(xli.OnRun(func(ctx context.Context, _ *xli.Command, next xli.Next) error {
			if os.Getenv("CXZ_BACKEND") == "kubernetes" {
				return fmt.Errorf("%s", kubernetesVersionRefusal)
			}
			return next(ctx)
		}), c.Handler)
	}
	for _, sub := range c.Commands {
		guardTree(sub)
	}
}
