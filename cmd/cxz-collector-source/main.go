// cxz-collector-source writes the OCB context used by published server images.
package main

import (
	"fmt"
	"github.com/lesomnus/cxz/internal/collectorbuild"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: cxz-collector-source DIRECTORY")
		os.Exit(2)
	}
	if err := collectorbuild.ValidateDirectory(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := collectorbuild.Write(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
