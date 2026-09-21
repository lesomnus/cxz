package projectconfig

import _ "embed"

//go:embed docker-compose.yaml
var initialCompose []byte

func Template() []byte { return append([]byte(nil), initialCompose...) }
