package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// The helper protocol is one request in, and a line per answer out.
//
// It is a stream rather than a reply because the point of the feature is that
// results appear as they are found: a search over every project visits a lot of
// journals, and waiting for the last one before showing the first is the thing
// to avoid. One JSON object per line is protocol enough for that, and it is
// what every other helper here already speaks.
//
// The index comes first, before anything is read. A caller searching several
// projects at once needs the order before it has the contents, or it cannot
// merge them without buffering everything.
type ScanRequest struct {
	// Project is the installation's project id, and the helper refuses any
	// session that does not claim it. A helper runs with one project's storage
	// mounted, so this only says out loud what the mount already decided.
	Project string    `json:"project,omitempty"`
	Query   ScanQuery `json:"query"`
}

type ScanMessage struct {
	Index   []ScanSession `json:"index,omitempty"`
	Visit   *ScanVisit    `json:"visit,omitempty"`
	Result  *ScanResult   `json:"result,omitempty"`
	Error   string        `json:"error,omitempty"`
	Indexed bool          `json:"indexed,omitempty"` // an index with nothing in it
}

// ServeScan reads one request and writes the index, a line per session visited
// and finally the result. An error is both written and returned: the caller may
// already have used what came before it.
func ServeScan(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	var request ScanRequest
	if err := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&request); err != nil {
		return fmt.Errorf("invalid scan request: %w", err)
	}
	encoder := json.NewEncoder(out)
	fail := func(err error) error {
		_ = encoder.Encode(ScanMessage{Error: err.Error()})
		return err
	}
	s := &Scanner{Root: root, Project: request.Project}
	index, err := s.Index(ctx, request.Query)
	if err != nil {
		return fail(err)
	}
	if err = encoder.Encode(ScanMessage{Index: index, Indexed: true}); err != nil {
		return err
	}
	result, err := s.ScanIndexed(ctx, request.Query, index, func(v ScanVisit) error {
		return encoder.Encode(ScanMessage{Visit: &v})
	})
	if err != nil {
		return fail(err)
	}
	return encoder.Encode(ScanMessage{Result: &result})
}

// ReadScan turns the helper's lines back into calls, for a caller that wants
// one project rather than all of them. A line that is none of the expected
// shapes is an error rather than something to skip: dropping an answer is
// indistinguishable from finding nothing.
func ReadScan(r io.Reader, visit func(ScanVisit) error) (ScanResult, error) {
	var out ScanResult
	decoder := json.NewDecoder(r)
	hits := 0
	for {
		var m ScanMessage
		err := decoder.Decode(&m)
		if err == io.EOF {
			out.Hits = max(out.Hits, hits)
			return out, nil
		}
		if err != nil {
			return out, fmt.Errorf("unreadable scan output: %w", err)
		}
		switch {
		case m.Error != "":
			return out, fmt.Errorf("%s", m.Error)
		case m.Indexed:
		case m.Visit != nil:
			hits += len(m.Visit.Hits)
			if err = visit(*m.Visit); err != nil {
				return out, err
			}
		case m.Result != nil:
			out = *m.Result
		default:
			return out, fmt.Errorf("unrecognised scan output")
		}
	}
}
