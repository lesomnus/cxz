package lifecycle

import (
	"testing"

	"github.com/lesomnus/cxz/resource"
)

// The installation, the skills library, the MCP registrations and what is
// running from them have no entity behind them, so they are not in
// `resource.Server` and a test asks the stack for the layer that answers them
// -- the way the gRPC registration does. See docs/plans/service-boundaries.md.
func layerOf(t *testing.T, stack resource.Server) Layer {
	t.Helper()
	layer, ok := resource.Find[Layer](stack)
	if !ok {
		t.Fatal("no lifecycle layer in the stack")
	}
	return layer
}
