package main

import (
	"reflect"
	"testing"
)

func TestProjectFlagOrdering(t *testing.T) {
	got := optionsFirst([]string{".", "--agent", "codex", "--no-attach", "--config=x"}, map[string]bool{"no-attach": true})
	want := []string{"--agent", "codex", "--no-attach", "--config=x", "."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}
