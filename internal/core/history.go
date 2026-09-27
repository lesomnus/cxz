package core

import "encoding/json"

const HistoryCheckpointKind = "history_checkpoint"
const HistoryTrimmedKind = "history_trimmed"

// HistoryCheckpoint contains control state only, never provider conversation files.
// Seq is the last removed event. Subsequent events retain their original sequence.
type HistoryCheckpoint struct {
	Version  int      `json:"version"`
	Snapshot Snapshot `json:"snapshot"`
	Resume   []Event  `json:"resume,omitempty"`
}
type HistoryBoundary struct {
	Through uint64 `json:"through"`
}

func HistoryFloor(kind string, payload []byte) uint64 {
	if kind != HistoryTrimmedKind {
		return 0
	}
	var v HistoryBoundary
	if json.Unmarshal(payload, &v) != nil {
		return 0
	}
	return v.Through
}
