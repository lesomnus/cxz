package cxzupdate

import (
	"testing"
	"time"
)

// The update path waits in minutes, and those waits are the cost of verifying
// it. An installation under test may shorten them; a bad value is ignored
// rather than argued with, because a typo must not stop an installation from
// updating. See cxz#224.
func TestTunableShortensAWaitAndIgnoresNonsense(t *testing.T) {
	const name = "CXZ_TEST_TUNABLE"
	if got := Tunable(name, time.Minute); got != time.Minute {
		t.Fatal("unset should be the default:", got)
	}
	for _, tc := range []struct {
		set  string
		want time.Duration
	}{
		{"2s", 2 * time.Second},
		{"500ms", 500 * time.Millisecond},
		{"", time.Minute},
		{"soon", time.Minute},
		{"0", time.Minute},
		{"-5s", time.Minute},
	} {
		t.Setenv(name, tc.set)
		if got := Tunable(name, time.Minute); got != tc.want {
			t.Fatal(tc.set, "->", got, "want", tc.want)
		}
	}
}
