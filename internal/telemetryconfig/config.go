// Package telemetryconfig contains non-secret installation telemetry preferences.
package telemetryconfig

import "fmt"

type Config struct {
	Enabled       *bool `json:"enabled,omitempty"`
	RetentionDays int   `json:"retention_days,omitempty"`
	QueueMiB      int   `json:"queue_mib,omitempty"`
}

func (c Config) Active() bool { return c.Enabled == nil || *c.Enabled }
func (c Config) Days() int {
	if c.RetentionDays == 0 {
		return 90
	}
	return c.RetentionDays
}
func (c Config) QueueBytes() int {
	if c.QueueMiB == 0 {
		return 64 << 20
	}
	return c.QueueMiB << 20
}
func (c Config) Validate() error {
	if c.RetentionDays < 0 || c.RetentionDays > 3660 {
		return fmt.Errorf("telemetry.retention_days must be between 1 and 3660 (or 0 for 90)")
	}
	if c.QueueMiB < 0 || c.QueueMiB > 1024 {
		return fmt.Errorf("telemetry.queue_mib must be between 1 and 1024 (or 0 for 64)")
	}
	return nil
}
