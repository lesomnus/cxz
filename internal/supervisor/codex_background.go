package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
)

const (
	codexBackgroundInterval = 5 * time.Second
	codexBackgroundTimeout  = 10 * time.Second
	codexBackgroundRetry    = 30 * time.Second
	codexBackgroundPageSize = 100
	codexBackgroundMaxPages = 100
)

type codexBackground struct {
	id        string
	sequence  uint64
	requested time.Time
	next      time.Time
	disabled  bool
	pages     int
	cursors   map[string]bool
	tasks     map[string]agentview.BackgroundTask
	published []byte
}

type codexBackgroundEvent struct {
	Type     string                     `json:"type"`
	Provider string                     `json:"provider"`
	Subtype  string                     `json:"subtype"`
	Status   string                     `json:"status,omitempty"`
	Tasks    []agentview.BackgroundTask `json:"tasks"`
}

// Poll once per supervisor, not once per browser. The app-server has no native
// background-terminal lifecycle notifications. Call under the supervisor lock.
func (c *codexProtocol) readBackground(now time.Time, force bool) {
	b, s := &c.background, c.s
	if b.disabled || s.stopping || s.snap.VendorID == "" || (s.snap.State != "idle" && s.snap.State != "working" && s.snap.State != "waiting_input") {
		return
	}
	if b.id != "" {
		if now.Sub(b.requested) < codexBackgroundTimeout {
			return
		}
		c.backgroundFailed("timeout", now)
		return
	}
	// Backoff after failure also applies to tool-triggered refreshes.
	if now.Before(b.next) && (!force || b.tasks == nil) {
		return
	}
	b.tasks = map[string]agentview.BackgroundTask{}
	b.cursors = map[string]bool{}
	b.pages = 0
	b.requested = now
	c.backgroundPage("", now)
}

func (c *codexProtocol) backgroundPage(cursor string, now time.Time) {
	b := &c.background
	b.sequence++
	request := fmt.Sprintf("cxz-background-%d", b.sequence)
	b.id = fmt.Sprintf("%q", request)
	b.pages++
	params := map[string]any{"threadId": c.s.snap.VendorID, "limit": codexBackgroundPageSize}
	if cursor != "" {
		params["cursor"] = cursor
	}
	if err := c.s.write(rpc(request, "thread/backgroundTerminals/list", params)); err != nil {
		c.backgroundFailed("error", now)
	}
}

// Handle telemetry replies before the general error path so unsupported APIs,
// failures and timeouts can never finish or interrupt the foreground turn.
func (c *codexProtocol) consumeBackground(id string, result, failure []byte, now time.Time) bool {
	if !strings.HasPrefix(id, `"cxz-background-`) {
		return false
	}
	b := &c.background
	if id != b.id {
		return true // Late reply to a timed-out page; never overwrite a newer list.
	}
	if len(failure) > 0 && string(failure) != "null" {
		var e struct{ Code int }
		_ = json.Unmarshal(failure, &e)
		status := "error"
		if e.Code == -32601 {
			b.disabled = true
			status = "unsupported"
		}
		c.backgroundFailed(status, now)
		return true
	}
	var page struct {
		Data *[]struct {
			ProcessID string `json:"processId"`
			ItemID    string `json:"itemId"`
			Command   string `json:"command"`
		} `json:"data"`
		NextCursor string `json:"nextCursor"`
	}
	if json.Unmarshal(result, &page) != nil || page.Data == nil {
		c.backgroundFailed("error", now)
		return true
	}
	for _, terminal := range *page.Data {
		if terminal.ProcessID == "" {
			c.backgroundFailed("error", now)
			return true
		}
		b.tasks[terminal.ProcessID] = agentview.BackgroundTask{
			ID: terminal.ProcessID, ToolID: terminal.ItemID,
			Description: terminal.Command, Status: "working", Active: true,
		}
	}
	if page.NextCursor != "" {
		if b.cursors[page.NextCursor] || b.pages >= codexBackgroundMaxPages {
			c.backgroundFailed("error", now)
			return true
		}
		b.cursors[page.NextCursor] = true
		c.backgroundPage(page.NextCursor, now)
		return true
	}
	b.id = ""
	b.next = now.Add(codexBackgroundInterval)
	tasks := make([]agentview.BackgroundTask, 0, len(b.tasks))
	for _, task := range b.tasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	c.publishBackground(codexBackgroundEvent{Type: "background", Provider: "codex", Subtype: "background_tasks_changed", Tasks: tasks})
	return true
}

func (c *codexProtocol) backgroundFailed(status string, now time.Time) {
	b := &c.background
	b.id = ""
	b.tasks = nil
	b.next = now.Add(codexBackgroundRetry)
	c.publishBackground(codexBackgroundEvent{Type: "background", Provider: "codex", Subtype: "background_status", Status: status})
}

func (c *codexProtocol) publishBackground(event codexBackgroundEvent) {
	raw, _ := json.Marshal(event)
	if bytes.Equal(raw, c.background.published) {
		return // CPU/RSS samples and unchanged lists do not create UI updates.
	}
	c.background.published = raw
	c.s.updateBackground.Apply(raw)
	c.s.event("background", event.Subtype, "", json.RawMessage(raw), nil)
}
