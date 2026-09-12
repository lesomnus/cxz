package agentview

import (
	"fmt"
	"sort"
	"time"
)

// Window is a provider-reported account limit, never inferred from token usage.
// Remaining is nil if the provider reported only a status, not utilization.
type Window struct {
	Key, Label, Status string
	Remaining          *float64
	Reset              time.Time
	Observed           time.Time
}

func Quota(provider, method string, raw []byte) []Window {
	root := object(raw)
	var result []Window
	add := func(key, label string, f fields, percentKey string, multiplier float64) {
		if len(f) == 0 {
			return
		}
		w := Window{Key: key, Label: label, Status: f.text("status")}
		if used, ok := f.number(percentKey); ok && used >= 0 {
			remaining := max(0, min(100, 100-used*multiplier))
			w.Remaining = &remaining
		}
		if reset, ok := f.number("resetsAt"); ok && reset > 0 {
			w.Reset = time.Unix(int64(reset), 0)
		}
		result = append(result, w)
	}
	if provider == "codex" && method == "account/rateLimits/updated" {
		limits := root.child("rateLimits")
		buckets := root.child("rateLimitsByLimitId")
		if len(buckets) == 0 {
			buckets = fields{}
			id := limits.text("limitId")
			if id == "" {
				id = "codex"
			}
			buckets[id] = root["rateLimits"]
		}
		ids := make([]string, 0, len(buckets))
		for id := range buckets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			limits = buckets.child(id)
			for _, key := range []string{"primary", "secondary"} {
				f := limits.child(key)
				label := key
				if mins, ok := f.number("windowDurationMins"); ok {
					switch {
					case mins == 10080:
						label = "wk"
					case mins >= 1440 && int(mins)%1440 == 0:
						label = fmt.Sprintf("%.0fd", mins/1440)
					case mins >= 60 && int(mins)%60 == 0:
						label = fmt.Sprintf("%.0fh", mins/60)
					default:
						label = fmt.Sprintf("%.0fm", mins)
					}
				}
				if id != "codex" {
					name := limits.text("limitName")
					if name == "" {
						name = id
					}
					label = name + "/" + label
				}
				add(id+"/"+key, label, f, "usedPercent", 1)
			}
		}
	}
	if provider == "claude" && method == "rate_limit_event" {
		f := root.child("rate_limit_info")
		key := f.text("rateLimitType")
		if key == "" {
			key = "quota"
		}
		label := key
		switch key {
		case "five_hour":
			label = "5h"
		case "seven_day":
			label = "wk"
		case "seven_day_opus":
			label = "wk-opus"
		case "seven_day_sonnet":
			label = "wk-sonnet"
		}
		add(key, label, f, "utilization", 100)
	}
	if provider == "claude" && method == "get_usage" {
		limits := root.child("rate_limits")
		for _, entry := range [][2]string{{"five_hour", "5h"}, {"seven_day", "wk"}, {"seven_day_opus", "wk-opus"}, {"seven_day_sonnet", "wk-sonnet"}, {"seven_day_oauth_apps", "wk-apps"}} {
			f := limits.child(entry[0])
			if len(f) == 0 {
				continue
			}
			// get_usage uses percent, unlike rate_limit_event's 0–1 fraction.
			add(entry[0], entry[1], f, "utilization", 1)
			result[len(result)-1].Reset, _ = time.Parse(time.RFC3339Nano, f.text("resets_at"))
		}
	}
	return result
}

// QuotaReplacement distinguishes full snapshots from per-bucket notifications.
func QuotaReplacement(provider, method string, raw []byte) (all bool, prefix string) {
	if provider == "claude" && method == "get_usage" {
		return true, ""
	}
	if provider == "codex" && method == "account/rateLimits/updated" {
		root := object(raw)
		if len(root.child("rateLimitsByLimitId")) > 0 {
			return true, ""
		}
		id := root.child("rateLimits").text("limitId")
		if id == "" {
			id = "codex"
		}
		return false, id + "/"
	}
	return false, ""
}
