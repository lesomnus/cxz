//go:build !linux

package server

import "context"

// Containers use Linux. Other platforms retain the 30-second reconciliation.
func observeJournals(ctx context.Context, root string, changed func(string)) error {
	<-ctx.Done()
	return nil
}
