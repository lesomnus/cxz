package journal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/lesomnus/cxz/internal/core"
)

// Cursor ends at a committed newline, never inside an incomplete batch. Keep it
// only after the derived database transaction commits; it is rebuilt on restart.
type Cursor struct {
	Offset int64
	Seq    uint64
	info   os.FileInfo
}

// ReadSince reads only the appended suffix of an append-only journal. Replacement,
// truncation or an in-place rewrite without growth triggers a validated full read.
// It neither repairs a torn tail nor exposes any part of an uncommitted batch.
func ReadSince(ctx context.Context, path string, previous Cursor) (events []core.Event, next Cursor, reset bool, err error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, Cursor{}, true, nil
	}
	if err != nil {
		return nil, previous, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, previous, false, err
	}
	reset = previous.info == nil || !os.SameFile(previous.info, info) || info.Size() < previous.info.Size() ||
		(info.Size() == previous.info.Size() && info.ModTime() != previous.info.ModTime())
	next = previous
	if reset {
		next = Cursor{}
	}
	next.info = info
	if _, err = f.Seek(next.Offset, io.SeekStart); err != nil {
		return nil, previous, reset, err
	}
	r := bufio.NewReader(io.LimitReader(f, info.Size()-next.Offset))
	for {
		if err = ctx.Err(); err != nil {
			return nil, previous, reset, err
		}
		line, readErr := r.ReadBytes('\n')
		if readErr == io.EOF {
			return events, next, reset, nil
		}
		if readErr != nil {
			return nil, previous, reset, readErr
		}
		batch, decodeErr := decode(line, next.Seq)
		if decodeErr != nil {
			return nil, previous, reset, fmt.Errorf("corrupt committed record at %d: %w", next.Offset, decodeErr)
		}
		events = append(events, batch...)
		next.Offset += int64(len(line))
		next.Seq += uint64(len(batch))
	}
}
