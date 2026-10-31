package telemetrystore

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/lesomnus/cxz/resource"
)

//go:embed stats.sql
var statsSQL string

func (c *Client) Stats(ctx context.Context, scope, id string, from, to int64) (*resource.ConversationStatsReply, error) {
	filter := fmt.Sprintf("TimeUnix >= now() - INTERVAL %d DAY", c.RetentionDays)
	params := map[string]string{"from": strconv.FormatInt(from, 10), "to": strconv.FormatInt(to, 10)}
	if scope != "" {
		if scope != "session" && scope != "project" && scope != "account" {
			return nil, fmt.Errorf("invalid telemetry statistics scope")
		}
		b, err := hex.DecodeString(id)
		if err != nil || len(b) != 16 {
			return nil, fmt.Errorf("invalid telemetry statistics identity")
		}
		filter += " AND ResourceAttributes['cxz." + scope + ".id']={scope:String}"
		params["scope"] = id
	}
	b, err := c.Query(ctx, fmt.Sprintf(statsSQL, filter, filter), params)
	if err != nil {
		return nil, err
	}
	// Best effort exporters cannot prove complete coverage (running legacy
	// processes, queue overflow, TTL and delayed native exports). Never turn a
	// successful query into a claim that no usage was missed.
	now := time.Now().UnixMilli()
	complete := false
	total := resource.ConversationStatsValues_builder{}.Build()
	result := resource.ConversationStatsReply_builder{FromMs: &from, ToMs: &to, GeneratedMs: &now, Total: total, Complete: &complete}.Build()
	days := map[string]*resource.ConversationStatsValues{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		// Explicit wire names avoid relying on underscore/case matching rules.
		var row map[string]float64
		var labels struct{ Date, Session string }
		if err = json.Unmarshal(scanner.Bytes(), &labels); err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(scanner.Bytes(), &fields); err != nil {
			return nil, err
		}
		delete(fields, "date")
		delete(fields, "session")
		raw, _ := json.Marshal(fields)
		if err = json.Unmarshal(raw, &row); err != nil {
			return nil, err
		}
		if days[labels.Date] == nil {
			v := resource.ConversationStatsValues_builder{}.Build()
			days[labels.Date] = v
			date := labels.Date
			result.SetDays(append(result.GetDays(), resource.ConversationStatsDay_builder{Date: &date, Values: v}.Build()))
		}
		for _, target := range []*resource.ConversationStatsValues{total, days[labels.Date]} {
			addRow(target, row)
		}
		if !seen[labels.Session] {
			id, err := hex.DecodeString(labels.Session)
			if err != nil || len(id) != 16 {
				return nil, fmt.Errorf("invalid telemetry session identity in database")
			}
			result.SetSessions(append(result.GetSessions(), resource.ConversationStatsCoverage_builder{SessionId: id, Complete: &complete}.Build()))
			seen[labels.Session] = true
		}
	}
	return result, scanner.Err()
}
func integer(n float64) uint64 {
	if n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	if n >= math.MaxUint64 {
		return math.MaxUint64
	}
	return uint64(n)
}
func addRow(a *resource.ConversationStatsValues, v map[string]float64) {
	a.SetEvents(a.GetEvents() + integer(v["events"]))
	a.SetInputMessages(a.GetInputMessages() + integer(v["inputs"]))
	a.SetAssistantEvents(a.GetAssistantEvents() + integer(v["assistants"]))
	a.SetConversationBytes(a.GetConversationBytes() + integer(v["bytes"]))
	a.SetEventContentBytes(a.GetEventContentBytes() + integer(v["event_bytes"]))
	a.SetFinishedTurns(a.GetFinishedTurns() + integer(v["turns"]))
	a.SetInputTokens(a.GetInputTokens() + integer(v["input_tokens"]))
	a.SetOutputTokens(a.GetOutputTokens() + integer(v["output_tokens"]))
	a.SetCacheReadTokens(a.GetCacheReadTokens() + integer(v["cache_read"]))
	a.SetCacheWriteTokens(a.GetCacheWriteTokens() + integer(v["cache_write"]))
	a.SetTokenReportedTurns(a.GetTokenReportedTurns() + integer(v["token_turns"]))
	a.SetCostReportedTurns(a.GetCostReportedTurns() + integer(v["cost_turns"]))
	a.SetReportedCostUsd(a.GetReportedCostUsd() + v["cost"])
}
