package telemetryauth

import _ "embed"

// Source is the same SDK-free identity contract compiled into the standalone
// Collector. Shipping it avoids maintaining a second authentication parser.
//
//go:embed identity.go
var Source []byte
