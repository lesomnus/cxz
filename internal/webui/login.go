package webui

import (
	"context"
	"github.com/lesomnus/cxz/internal/projectid"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Login is an ephemeral byte bridge to the existing provider login streams.
// No input or output enters the journal, resource store or gateway logs.
type loginProxy struct{ client resource.ProjectServiceClient }
type loginFrame struct {
	Output string `json:"output,omitempty"`
	Done   bool   `json:"done,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (p *loginProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimPrefix(r.URL.Path, "/login/")
	account, project, key := r.URL.Query().Get("account"), r.URL.Query().Get("project"), r.URL.Query().Get("key")
	if r.Method != http.MethodGet || r.Header.Get("Origin") == "" || (kind != "session" && kind != "auxiliary") || len(account) == 0 || len(account) > 63 || len(key) > 1024 || (kind == "session" && (!terminalProjectID.MatchString(project) || key == "")) || (kind == "auxiliary" && (project != "" || key != "")) {
		http.Error(w, "invalid login connection", http.StatusBadRequest)
		return
	}
	socket, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer socket.CloseNow()
	socket.SetReadLimit(8192)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	var upstream grpc.BidiStreamingClient[resource.ProjectLoginRequest, resource.ProjectLoginOutput]
	if kind == "session" {
		upstream, err = p.client.SessionLogin(ctx)
	} else {
		upstream, err = p.client.AuxiliaryLogin(ctx)
	}
	alias := account
	first := resource.ProjectLoginRequest_builder{Account: resource.AccountRef_builder{Alias: &alias}.Build()}
	if kind == "session" {
		first.Ref = projectid.Ref(project)
		first.SessionKey = &key
	}
	if err == nil {
		err = upstream.Send(first.Build())
	}
	if err != nil {
		_ = wsjson.Write(ctx, socket, loginFrame{Error: status.Convert(err).Message()})
		return
	}
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		defer cancel()
		total := 0
		for {
			typ, bytes, err := socket.Read(ctx)
			if err != nil {
				return
			}
			total += len(bytes)
			if typ != websocket.MessageText || total > 64*1024 {
				clear(bytes)
				return
			}
			err = upstream.Send(resource.ProjectLoginRequest_builder{Input: bytes}.Build())
			clear(bytes)
			if err != nil {
				return
			}
		}
	}()
	defer func() { cancel(); socket.CloseNow(); <-inputDone }()
	for {
		reply, err := upstream.Recv()
		if err != nil {
			frame := loginFrame{Done: err == io.EOF}
			if err != io.EOF {
				frame.Error = status.Convert(err).Message()
			}
			_ = wsjson.Write(ctx, socket, frame)
			_ = socket.Close(websocket.StatusNormalClosure, "")
			return
		}
		if err := wsjson.Write(ctx, socket, loginFrame{Output: string(reply.GetOutput())}); err != nil {
			return
		}
	}
}
