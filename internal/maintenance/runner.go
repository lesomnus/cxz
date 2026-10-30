package maintenance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/lesomnus/cxz/internal/core"
)

type Runner struct {
	Root   string
	Engine Engine
	// Retain is for age-based database cleanup. It runs outside pressure
	// recovery, since deleting rows need not return space to the filesystem.
	Retain    func(context.Context, Policy, time.Time) error
	Configure func(context.Context, Policy) error
	Error     func(error)
	wake      chan struct{}
}

func NewRunner(root string, engine Engine) *Runner {
	return &Runner{Root: root, Engine: engine, wake: make(chan struct{}, 1)}
}
func (r *Runner) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runner) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	var last time.Time
	var fingerprint string
	pending := true
	for {
		current := ""
		for _, name := range []string{"cxz-update.json", "runtime-update.json", "../runtime.json"} {
			if st, err := os.Stat(filepath.Join(r.Root, name)); err == nil {
				current += fmt.Sprint(st.ModTime().UnixNano(), "/")
			}
		}
		if current != fingerprint {
			fingerprint = current
			pending = true
		}
		p, err := Load(r.Root)
		if err == nil && r.Configure != nil {
			sync, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = r.Configure(sync, p)
			cancel()
		}
		if err != nil {
			r.report(err)
		} else if !p.Disabled {
			now := time.Now()
			due := last.IsZero() || now.Sub(last) >= p.Interval() || pending && now.Sub(last) >= 5*time.Minute
			// Cheap probes only between ordinary maintenance opportunities.
			if !due && now.Sub(last) >= time.Minute {
				for _, path := range r.Engine.Roots {
					if s, e := DiskSpace(path); e == nil && (s.low(p) || r.Engine.pressured[s.Device] && !s.recovered(p)) {
						due = true
						break
					}
				}
			}
			if due {
				last = now
				pending = false
				pass, cancel := context.WithTimeout(ctx, 20*time.Second)
				r.once(pass, p, now)
				cancel()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
			pending = true
		case <-tick.C:
		}
	}
}

func (r *Runner) once(ctx context.Context, p Policy, now time.Time) {
	lock := flock.New(filepath.Join(r.Root, "maintenance.lock"))
	defer lock.Close()
	ok, err := lock.TryLock()
	if err != nil {
		r.report(err)
		return
	}
	if !ok {
		return
	}
	result := r.Engine.Pass(ctx, p, now)
	pressured := false
	for _, v := range result.Volumes {
		pressured = pressured || v.Pressure
	}
	if ctx.Err() == nil && !pressured && r.Retain != nil {
		if err = r.Retain(ctx, p, now); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	}
	result.Finished = time.Now()
	if err = core.WriteJSON(filepath.Join(r.Root, "maintenance-status.json"), result); err != nil {
		r.report(err)
	}
	for _, v := range result.Volumes {
		if v.Pressure {
			r.report(fmt.Errorf("disk pressure remains on %s: %d/%d bytes available; safe cleanup candidates exhausted or pass budget reached", v.Path, v.Available, v.Total))
		}
	}
	if len(result.Errors) > 0 {
		r.report(fmt.Errorf("maintenance: %v", result.Errors))
	}
}
func (r *Runner) report(err error) {
	if r.Error != nil && !os.IsNotExist(err) {
		r.Error(err)
	}
}
