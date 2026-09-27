// Package historypolicy configures cxz history only. Provider state is excluded.
package historypolicy

import (
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
)

const MiB = 1 << 20

type Policy struct {
	Disabled    bool `json:"disabled,omitempty"`
	MaxMiB      int  `json:"max_mib,omitempty"`
	WindowMiB   int  `json:"window_mib,omitempty"`
	WindowTurns int  `json:"window_turns,omitempty"`
}

func (p Policy) Limits() (disk int64, bytes, turns int) {
	disk = int64(p.MaxMiB) * MiB
	if disk == 0 {
		disk = 100 * MiB
	}
	if p.Disabled {
		disk = 0
	}
	bytes = p.WindowMiB * MiB
	if bytes == 0 {
		bytes = 10 * MiB
	}
	turns = p.WindowTurns
	if turns == 0 {
		turns = 200
	}
	return
}
func (p Policy) Validate() error {
	if p.MaxMiB < 0 || p.MaxMiB > 10240 || p.WindowMiB < 0 || p.WindowMiB > 1024 || p.WindowTurns < 0 || p.WindowTurns > 10000 {
		return fmt.Errorf("invalid history limits (disk 1–10240 MiB, window 1–1024 MiB / 1–10000 turns; 0 uses default)")
	}
	return nil
}
func Load(root string) (Policy, error) {
	var p Policy
	b, e := os.ReadFile(filepath.Join(root, "history-policy.json"))
	if os.IsNotExist(e) {
		return p, nil
	}
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return p, e
	}
	return p, p.Validate()
}
func Save(root string, p Policy) error {
	if e := p.Validate(); e != nil {
		return e
	}
	return core.WriteJSON(filepath.Join(root, "history-policy.json"), p)
}

// Window is a client-only preference, independent from the Manager disk policy.
type Window struct {
	MiB   int `json:"window_mib,omitempty"`
	Turns int `json:"window_turns,omitempty"`
}

func (w Window) Limits() (int, int) {
	_, b, t := (Policy{WindowMiB: w.MiB, WindowTurns: w.Turns}).Limits()
	return b, t
}
func (w Window) Validate() error { return (Policy{WindowMiB: w.MiB, WindowTurns: w.Turns}).Validate() }
