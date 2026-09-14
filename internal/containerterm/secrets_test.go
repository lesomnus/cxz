package containerterm

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"testing"
)

func TestOldWispRejectsSecretsBeforeTransfer(t *testing.T) {
	var wire bytes.Buffer
	c := &wispClient{enc: json.NewEncoder(&wire), gate: make(chan struct{}, 1), done: make(chan struct{}), stop: func() {}}
	p := &WispPool{clients: map[string]*wispClient{"p/c/u": c}}
	_, err := p.PutSecret(context.Background(), context.Background(), &api.Project{Id: "p", ContainerId: "c", RemoteUser: "u"}, "s", []byte("must-not-transfer"))
	if err == nil || wire.Len() != 0 {
		t.Fatal("old runtime received secret", err)
	}
}
