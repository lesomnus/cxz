package webui

import (
	"net/http"
	"regexp"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/encoding/protojson"
)

const connectionListPath = resource.ConnectionService_List_FullMethodName

var connectionAlias = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Connection catalog metadata is available before login so the browser can
// resolve a bookmark and show that connection's sign-in page.
func (c Config) listConnection(w http.ResponseWriter) {
	name := c.ConnectionAlias
	if name == "" {
		name = "local"
	}
	id := c.ConnectionID
	if id == uuid.Nil {
		id = uuid.NewSHA1(uuid.NameSpaceURL, []byte(c.Origin))
	}
	entry := resource.Connection_builder{
		Id: id[:], Name: name, Target: c.Origin, Transport: "origin",
		IsDefault: true, ReadOnly: true,
	}.Build()
	reply := resource.ConnectionListResponse_builder{Items: []*resource.Connection{entry}}.Build()
	data, err := protojson.Marshal(reply)
	if err != nil {
		http.Error(w, "invalid connection catalogue", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
