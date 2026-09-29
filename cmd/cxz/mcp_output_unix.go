//go:build !windows

package main

import "github.com/lesomnus/xli"

func mcpOutput(c *xli.Command, v any) error { return writeOutput(c, v) }
