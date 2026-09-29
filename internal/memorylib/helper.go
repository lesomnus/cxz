package memorylib

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"io"
)

type HelperRequest struct {
	Session core.Session
	Request Request
}

func Serve(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	var q HelperRequest
	if e := json.NewDecoder(io.LimitReader(in, MaxBundle+65536)).Decode(&q); e != nil {
		return e
	}
	if e := helperIdentity(root, q.Session); e != nil {
		return e
	}
	result, e := New(root, q.Session).Do(ctx, q.Request)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(result)
}
