package maintenance

import (
	"context"
	"errors"
	"sort"
	"time"
)

type Kind int

const (
	Residue Kind = iota
	Temporary
	Logs
	Binaries
	Cache
)

// Candidates must recheck ownership, references and locks in Remove. Merely
// collecting a path never grants permission to delete it later.
type Candidate struct {
	Path   string
	Kind   Kind
	Age    time.Time
	Remove func(context.Context) (bool, error)
}
type Collect func(context.Context, Policy, time.Time, bool) ([]Candidate, error)

type Space struct {
	Device    string `json:"device"`
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
}

func (s Space) low(p Policy) bool {
	return s.Total > 0 && (p.PressureLowPercent > 0 && float64(s.Available)/float64(s.Total)*100 < float64(p.PressureLowPercent) || s.Available < uint64(p.MinimumFreeMiB)<<20)
}
func (s Space) recovered(p Policy) bool {
	return s.Total > 0 && (p.PressureLowPercent == 0 || float64(s.Available)/float64(s.Total)*100 >= float64(p.PressureHighPercent)) && s.Available >= uint64(p.MinimumFreeMiB)<<20
}

type Volume struct {
	Space
	Path            string `json:"path"`
	Pressure        bool   `json:"pressure"`
	TargetAvailable uint64 `json:"target_available"`
	Shortfall       uint64 `json:"shortfall"`
}
type Report struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Removed  int       `json:"removed"`
	Examined int       `json:"examined"`
	Volumes  []Volume  `json:"volumes,omitempty"`
	Errors   []string  `json:"errors,omitempty"`
}
type Engine struct {
	Roots     []string
	Collect   Collect
	Probe     func(string) (Space, error)
	pressured map[string]bool
}

// Pass has a fixed candidate budget as well as the caller's time budget. A
// shortage that cannot be satisfied is returned as Pressure=true, not retried
// in a loop. Each filesystem must recover on its own.
func (e *Engine) Pass(ctx context.Context, p Policy, now time.Time) Report {
	r := Report{Started: now}
	if p.Disabled {
		r.Finished = time.Now()
		return r
	}
	if e.Probe == nil {
		e.Probe = DiskSpace
	}
	if e.pressured == nil {
		e.pressured = map[string]bool{}
	}
	addErr := func(err error) {
		if err != nil && len(r.Errors) < 20 {
			r.Errors = append(r.Errors, err.Error())
		}
	}
	run := func(pressure bool, device string) {
		if r.Examined >= 128 || ctx.Err() != nil {
			return
		}
		cs, err := e.Collect(ctx, p, now, pressure)
		addErr(err)
		sort.SliceStable(cs, func(i, j int) bool {
			if cs[i].Kind != cs[j].Kind {
				return cs[i].Kind < cs[j].Kind
			}
			return cs[i].Age.Before(cs[j].Age)
		})
		for _, c := range cs {
			if ctx.Err() != nil || r.Examined >= 128 {
				break
			}
			if pressure {
				s, err := e.Probe(c.Path)
				if err != nil {
					addErr(err)
					continue
				}
				if s.Device != device {
					continue
				}
				if s.recovered(p) {
					e.pressured[device] = false
					break
				}
			}
			r.Examined++
			removed, err := c.Remove(ctx)
			addErr(err)
			if removed {
				r.Removed++
			}
		}
	}
	run(false, "")
	seen := map[string]bool{}
	for _, root := range e.Roots {
		if ctx.Err() != nil {
			break
		}
		s, err := e.Probe(root)
		if err != nil {
			addErr(err)
			continue
		}
		if seen[s.Device] {
			continue
		}
		seen[s.Device] = true
		pressure := s.low(p) || e.pressured[s.Device] && !s.recovered(p)
		if pressure {
			e.pressured[s.Device] = true
			run(true, s.Device)
			if fresh, err := e.Probe(root); err == nil {
				s = fresh
			} else {
				addErr(err)
			}
			pressure = !s.recovered(p)
		}
		e.pressured[s.Device] = pressure
		target := uint64(p.MinimumFreeMiB) << 20
		if p.PressureLowPercent > 0 {
			target = max(target, uint64(float64(s.Total)*float64(p.PressureHighPercent)/100))
		}
		shortfall := uint64(0)
		if pressure && target > s.Available {
			shortfall = target - s.Available
		}
		r.Volumes = append(r.Volumes, Volume{Space: s, Path: root, Pressure: pressure, TargetAvailable: target, Shortfall: shortfall})
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		addErr(err)
	}
	r.Finished = time.Now()
	return r
}
