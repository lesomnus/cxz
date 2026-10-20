//go:build !linux

package main

import "context"

// Only the Linux installation has a manager transaction or a rollout lock.
func rolloutDetail(context.Context, string) map[string]any { return nil }
