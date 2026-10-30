//go:build !linux

package maintenance

import "fmt"

func DiskSpace(string) (Space, error) {
	return Space{}, fmt.Errorf("disk-pressure maintenance is supported on Linux servers")
}
