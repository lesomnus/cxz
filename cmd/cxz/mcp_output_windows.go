package main

import (
	"encoding/json"
	"github.com/lesomnus/xli"
)

func mcpOutput(c *xli.Command, v any) error { return json.NewEncoder(c.Writer).Encode(v) }
