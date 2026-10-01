package projectconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func render(t *testing.T, raw string) (ImageOverride, error) {
	t.Helper()
	return Spec{Compose: json.RawMessage(raw)}.RenderImage()
}

func TestComposeOverrideTranslatesMountsAndEnvironment(t *testing.T) {
	for name, tc := range map[string]struct {
		compose string
		mounts  []string
		env     map[string]string
	}{
		"short bind": {
			compose: `{"services":{"${DEVCONTAINER_SERVICE}":{"volumes":["/host/src:/workspaces"]}}}`,
			mounts:  []string{"type=bind,source=/host/src,target=/workspaces"},
		},
		"short bind read only": {
			compose: `{"services":{"dev":{"volumes":["/host/src:/workspaces:ro"]}}}`,
			mounts:  []string{"type=bind,source=/host/src,target=/workspaces,readonly"},
		},
		// A source that is not a path is a named volume in Compose, and must not
		// become a bind mount of a directory that does not exist.
		"short named volume": {
			compose: `{"services":{"dev":{"volumes":["cache:/home/dev/.cache"]}}}`,
			mounts:  []string{"type=volume,source=cache,target=/home/dev/.cache"},
		},
		"long form": {
			compose: `{"services":{"dev":{"volumes":[{"type":"volume","source":"go-mod","target":"/go/pkg/mod","read_only":true}]}}}`,
			mounts:  []string{"type=volume,source=go-mod,target=/go/pkg/mod,readonly"},
		},
		"long form defaults to bind": {
			compose: `{"services":{"dev":{"volumes":[{"source":"/host","target":"/guest"}]}}}`,
			mounts:  []string{"type=bind,source=/host,target=/guest"},
		},
		"environment mapping": {
			compose: `{"services":{"dev":{"environment":{"A":"1","B":""}}}}`,
			env:     map[string]string{"A": "1", "B": ""},
		},
		"environment list": {
			compose: `{"services":{"dev":{"environment":["A=1","B=has=equals"]}}}`,
			env:     map[string]string{"A": "1", "B": "has=equals"},
		},
		// Declaring volume names is Compose bookkeeping; a volume mount creates
		// them, so there is nothing to translate and nothing to refuse.
		"volume declarations alone": {
			compose: `{"volumes":{"go-mod":null}}`,
		},
		"no services": {compose: `{"name":"anything"}`},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := render(t, tc.compose)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(out.Mounts, "|") != strings.Join(tc.mounts, "|") {
				t.Fatal("mounts", out.Mounts, "want", tc.mounts)
			}
			if len(out.Env) != len(tc.env) {
				t.Fatal("env", out.Env, "want", tc.env)
			}
			for k, v := range tc.env {
				if out.Env[k] != v {
					t.Fatal("env", k, out.Env[k], "want", v)
				}
			}
			if tc.mounts == nil && tc.env == nil && !out.Empty() {
				t.Fatal("expected an empty override", out)
			}
		})
	}
}

// Refusing by name is the point: an override that quietly did less than it says
// would leave the caller believing a mount or a sidecar was in place.
func TestComposeOverrideRefusesWhatAnImageCannotExpress(t *testing.T) {
	for name, tc := range map[string]struct{ compose, want string }{
		"privileged":        {`{"services":{"dev":{"privileged":true}}}`, "only volumes and environment translate"},
		"command":           {`{"services":{"dev":{"command":"sleep infinity"}}}`, "only volumes and environment translate"},
		"sidecar":           {`{"services":{"dev":{"volumes":["/a:/b"]},"cache":{"image":"redis"}}}`, "sidecars need a Compose devcontainer"},
		"top level key":     {`{"networks":{"shared":null}}`, "cannot apply to an image or Dockerfile devcontainer"},
		"tmpfs volume type": {`{"services":{"dev":{"volumes":[{"type":"tmpfs","source":"x","target":"/t"}]}}}`, "volume type \"tmpfs\""},
		"missing target":    {`{"services":{"dev":{"volumes":[{"source":"/host"}]}}}`, "requires source and target"},
		"bad short form":    {`{"services":{"dev":{"volumes":["/only-one"]}}}`, "must be source:target[:ro]"},
		"bad short option":  {`{"services":{"dev":{"volumes":["/a:/b:rw"]}}}`, "is not supported; use ro"},
		"bad environment":   {`{"services":{"dev":{"environment":["NOVALUE"]}}}`, "must be NAME=value"},
		"volumes not list":  {`{"services":{"dev":{"volumes":{"a":"b"}}}}`, "volumes must be a list"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := render(t, tc.compose)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal("want", tc.want, "got", err)
			}
		})
	}
}

// The message has to name the same key every run, or a report of it is useless.
func TestComposeOverrideRefusalIsDeterministic(t *testing.T) {
	const compose = `{"networks":{"a":null},"configs":{"b":null},"secrets":{"c":null}}`
	first, err := render(t, compose)
	if err == nil {
		t.Fatal("accepted", first)
	}
	for i := 0; i < 20; i++ {
		if _, again := render(t, compose); again.Error() != err.Error() {
			t.Fatal("message varies between runs:", err, "then", again)
		}
	}
	if !strings.Contains(err.Error(), "configs") {
		t.Fatal("expected the first key in sorted order", err)
	}
}

func TestNoOverrideTranslatesToNothing(t *testing.T) {
	out, err := Spec{}.RenderImage()
	if err != nil || !out.Empty() {
		t.Fatal(out, err)
	}
}
