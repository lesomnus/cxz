package logview

import "fmt"

func Tail(string, int) ([]byte, bool, error) {
	return nil, false, fmt.Errorf("runtime log files must be read by the Linux daemon")
}
