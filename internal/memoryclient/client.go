// Package memoryclient is the native client's connection-aware resource access.
package memoryclient

import (
	"context"
	"github.com/lesomnus/cxz/resource"
)

type Client interface {
	MemoryResources(context.Context, string) (resource.MemoryServiceClient, string, error)
}
