package resourceclient

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/memorylib"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) Library(ctx context.Context, id string, q memorylib.Request) (memorylib.Reply, error) {
	var out memorylib.Reply
	b, e := json.Marshal(q)
	if e != nil {
		return out, e
	}
	r, e := c.sessions.Library(ctx, resource.SessionLibraryRequest_builder{Ref: sr(id), Request: b}.Build())
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(r.GetData(), &out)
	return out, e
}
