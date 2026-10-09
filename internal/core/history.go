package core

import "encoding/json"

const HistoryCheckpointKind = "history_checkpoint"
const HistoryTrimmedKind = "history_trimmed"

// HistoryShedKind records that the verbatim vendor stream was removed up to a
// point while the conversation around it was kept. It is deliberately not
// HistoryTrimmedKind: that one carries a floor, and a floor says everything
// below it is gone.
const HistoryShedKind = "history_shed"

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
