// Package maintenance bounds disposable storage without measuring every file or
// treating retained user data as a disk-pressure escape hatch.
package maintenance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Policy struct {
	Disabled            bool  `json:"disabled,omitempty"`
	AuditDays           int   `json:"audit_days"`
	TrashDays           int   `json:"trash_days"`
	NotificationDays    int   `json:"notification_days"`
	TemporaryHours      int   `json:"temporary_hours"`
	BinaryDays          int   `json:"binary_days"`
	RecentVersions      int   `json:"recent_versions"`
	IntervalMinutes     int   `json:"interval_minutes"`
	RuntimeLogMiB       int   `json:"runtime_log_mib"`
	ComponentLogMiB     int   `json:"component_log_mib"`
	PressureLowPercent  int   `json:"pressure_low_percent"`
	PressureHighPercent int   `json:"pressure_high_percent"`
	MinimumFreeMiB      int64 `json:"minimum_free_mib"`
}

func Default() Policy {
	return Policy{AuditDays: 90, TrashDays: 30, NotificationDays: 30, TemporaryHours: 24, BinaryDays: 7, RecentVersions: 2, IntervalMinutes: 30, RuntimeLogMiB: 32, ComponentLogMiB: 8, PressureLowPercent: 5, PressureHighPercent: 10}
}

// Missing fields use defaults; explicit zero disables an age rule or pressure
// detection. A file is server-local and reloaded at every opportunity.
func Load(root string) (Policy, error) {
	p := Default()
	b, err := os.ReadFile(filepath.Join(root, "maintenance.json"))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if !bytes.HasPrefix(bytes.TrimSpace(b), []byte("{")) {
		return p, fmt.Errorf("maintenance policy must be one JSON object")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&p); err != nil {
		return p, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return p, fmt.Errorf("maintenance policy must be one JSON object")
	}
	return p, p.Validate()
}

func (p Policy) Validate() error {
	for _, v := range []int{p.AuditDays, p.TrashDays, p.NotificationDays, p.TemporaryHours, p.BinaryDays} {
		if v < 0 || v > 36500 {
			return fmt.Errorf("maintenance age must be between 0 and 36500")
		}
	}
	if p.IntervalMinutes < 1 || p.IntervalMinutes > 1440 || p.RecentVersions < 1 || p.RecentVersions > 100 || p.RuntimeLogMiB < 1 || p.RuntimeLogMiB > 1024 || p.ComponentLogMiB < 1 || p.ComponentLogMiB > 1024 {
		return fmt.Errorf("invalid maintenance interval, version count or log size")
	}
	if p.PressureLowPercent < 0 || p.PressureHighPercent < p.PressureLowPercent || p.PressureHighPercent > 50 || p.PressureLowPercent > 0 && p.PressureHighPercent == p.PressureLowPercent || p.MinimumFreeMiB < 0 || p.MinimumFreeMiB > 1<<30 {
		return fmt.Errorf("invalid maintenance pressure watermarks")
	}
	return nil
}

func (p Policy) Interval() time.Duration { return time.Duration(p.IntervalMinutes) * time.Minute }
