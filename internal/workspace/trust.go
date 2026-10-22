package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/lesomnus/cxz/internal/configtrust"
)

// trust is what one provision rendered: the documents it is about to act on,
// and what those documents ask for that needs a person to say yes.
//
// The decision is made once, against the configuration as a whole, because
// that is what the answer is about. Checking document by document asked the
// same question in waves, and -- worse -- the answer used to be a boolean on
// the project that never expired: a repository that later added `privileged`
// or a docker.sock mount was never asked again. See cxz#274.
type trust struct {
	rendered []byte
	findings []string
}

// saw records a document the provision rendered. The label travels with the
// findings so a person can tell which file is asking.
func (t *trust) saw(label string, doc map[string]any) error {
	b, err := json.Marshal(canonical(doc))
	if err != nil {
		return err
	}
	t.rendered = append(append(t.rendered, label...), b...)
	for _, f := range Findings(doc) {
		t.findings = append(t.findings, label+f)
	}
	return nil
}

// Digest identifies the configuration as rendered. Two uses, one value: a
// provision refuses to apply a changed configuration to a container that
// already exists, and consent is remembered for the content it was given for.
func (t *trust) Digest() string {
	sum := sha256.Sum256(t.rendered)
	return hex.EncodeToString(sum[:])
}

// decide answers whether this provision may proceed, and records consent when
// it is given. Nothing to say yes to means nothing to ask.
func (t *trust) decide(p *Project, consent bool) error {
	if len(t.findings) == 0 {
		return nil
	}
	digest := t.Digest()
	if consent {
		p.TrustedConfig = digest
		return nil
	}
	if p.TrustedConfig == digest {
		return nil
	}
	return &configtrust.Required{Findings: t.findings}
}

// canonical orders maps so that the same configuration renders to the same
// bytes. encoding/json already sorts map keys; this exists for the slices it
// does not touch and to keep the digest readable as a decision about content.
func canonical(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(x))
		for _, k := range keys {
			out[k] = canonical(x[k])
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, e := range x {
			out = append(out, canonical(e))
		}
		return out
	}
	return v
}

// Findings names what a rendered configuration asks for that cxz will not do
// unless it is told to. It reports JSON Pointer locations, never configuration
// values, and detection stays conservative (including string markers) because
// it is a question to ask a person, not a sandbox.
func Findings(cfg map[string]any) []string {
	var findings []string
	add := func(path, reason string) { findings = append(findings, fmt.Sprintf("  - %q: %s", path, reason)) }
	var walk func(any, string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for key := range x {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				value := x[key]
				child := path + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
				switch {
				case key == "privileged" && value == true:
					add(child, "privileged container requested")
				case key == "initializeCommand":
					add(child, "host-side initialization command configured")
				case key == "network_mode" && value == "host":
					add(child, "host network namespace requested")
				case key == "pid" && value == "host":
					add(child, "host PID namespace requested")
				}
				walk(value, child)
			}
		case []any:
			for i, value := range x {
				child := path + "/" + strconv.Itoa(i)
				if flag, ok := value.(string); ok && i+1 < len(x) && x[i+1] == "host" {
					switch flag {
					case "--network", "--net":
						add(child, "host network namespace requested (next array item)")
					case "--pid":
						add(child, "host PID namespace requested (next array item)")
					}
				}
				walk(value, child)
			}
		case string:
			for _, rule := range []struct{ marker, reason string }{
				{"docker.sock", "Docker socket reference detected"},
				{"--privileged", "privileged container flag detected"},
				{"--network=host", "host network flag detected"},
				{"--pid=host", "host PID flag detected"},
				{"SYS_ADMIN", "SYS_ADMIN capability reference detected"},
			} {
				if strings.Contains(x, rule.marker) {
					add(path, rule.reason)
				}
			}
		}
	}
	walk(cfg, "")
	return findings
}
