//go:build !linux

package memorymigration

func AsOwner(string) error { return nil }
