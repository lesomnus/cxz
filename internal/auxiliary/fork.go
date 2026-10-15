package auxiliary

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

// InheritSession copies preferences and title metadata, never running jobs or
// their accounting. Existing child preferences make disconnected retries safe.
func (c *Controller) InheritSession(source, target string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if source == "" || target == "" || source == target {
		return fmt.Errorf("distinct source and target required")
	}
	if _, err := os.Stat(c.preferencePath(target)); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	preferences, err := c.preferences(source)
	if err != nil {
		return err
	}
	// Inherited transcript rows are history, not new automatic AI work.
	preferences.Since = time.Now().UnixMilli()
	title, err := c.loadTitle(source)
	if err != nil {
		return err
	}
	title.Job, title.Error = "", ""
	if title.Status == "running" || title.Status == "queued" {
		title.Status = "interrupted"
	}
	if err = c.saveTitle(target, title); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(c.preferencePath(target)), 0700); err != nil {
		return err
	}
	return core.WriteJSON(c.preferencePath(target), preferences)
}
