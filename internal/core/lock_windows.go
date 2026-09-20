package core

import (
	"fmt"
	"os"
)

// Runtime locks belong to the Linux daemon, never to a Windows frontend.
func Lock(string) (*os.File, error) {
	return nil, fmt.Errorf("runtime locking requires the Linux daemon")
}
