package multiclient

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/assets"
)

type uploadDaemon struct {
	daemon
	received assets.Upload
	body     string
}

func (d *uploadDaemon) UploadAttachment(_ context.Context, in assets.Upload, src io.Reader) (string, error) {
	d.received = in
	data, err := io.ReadAll(src)
	d.body = string(data)
	return "/cxz/assets/session/attachment/" + in.Name, err
}
func TestUploadRoutesToOwningDaemon(t *testing.T) {
	a, b := &uploadDaemon{}, &uploadDaemon{}
	c := New(t.Context(), []Source{{Name: "home", Client: a}, {Name: "work", Client: b, Remote: true}}, "home")
	defer c.Close()
	request := assets.Upload{SessionID: "work::same", RunID: "run", Name: "report.txt", Size: 4}
	path, err := c.UploadAttachment(c.ContextFor(t.Context(), "home::project"), request, strings.NewReader("data"))
	if err != nil {
		t.Fatal(err)
	}
	if b.received.SessionID != "same" || b.received.RunID != "run" || b.body != "data" || a.body != "" || request.SessionID != "work::same" {
		t.Fatal("wrong daemon or scope", b.received, a.body, b.body)
	}
	if path != "/cxz/assets/session/attachment/report.txt" {
		t.Fatal("agent path was rewritten", path)
	}
}
