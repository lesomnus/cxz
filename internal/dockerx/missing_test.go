package dockerx

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// Telling "it is not there" apart from "Docker could not be asked" decides
// whether a manager transaction can still be completed or has to be rolled
// back, and the daemon says which one in words. See cxz#221.
func TestMissingReadsTheDaemonsWords(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("docker inspect: exit status 1: error: no such object: deadbeef"), true},
		{errors.New("docker inspect: exit status 1: Error: No such object: deadbeef"), true},
		{errors.New("docker inspect: exit status 1: Cannot connect to the Docker daemon"), false},
		{errors.New("docker inspect: signal: killed"), false},
		{context.DeadlineExceeded, false},
		{fmt.Errorf("wrapped: %w", errors.New("error: no such object: x")), true},
	} {
		if got := Missing(tc.err); got != tc.want {
			t.Fatal(tc.err, "->", got)
		}
	}
}
