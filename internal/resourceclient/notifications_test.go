package resourceclient_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/sessionattention"
	"github.com/lesomnus/cxz/internal/websandbox"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestNotificationClientsShareReadStateOverGRPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	sandbox := websandbox.New(19, time.Millisecond)
	defer sandbox.Close()
	listener := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	sandbox.Register(g)
	go g.Serve(listener)
	defer g.Stop()
	connect := func() *grpc.ClientConn {
		conn, err := grpc.NewClient("passthrough:///notifications", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	one, two := connect(), connect()
	first, second := resourceclient.New(one), resourceclient.New(two)
	ref := resource.SessionRef_builder{RuntimeId: ptr("session-2")}.Build()
	sessions := resource.NewSessionServiceClient(one)
	s, err := sessions.Get(ctx, resource.SessionGetRequest_builder{Ref: ref}.Build())
	if err != nil {
		t.Fatal(err)
	}
	covered := []*api.Session{{Id: s.GetRuntimeId(), ProjectId: projectid.String(s.GetProject().GetId())}}
	updates := []chan sessionattention.Update{make(chan sessionattention.Update, 16), make(chan sessionattention.Update, 16)}
	for i, client := range []*resourceclient.Client{first, second} {
		go func() {
			if err := client.WatchAttention(ctx, covered, func(update sessionattention.Update) {
				select {
				case updates[i] <- update:
				case <-ctx.Done():
				}
			}); err != nil && ctx.Err() == nil {
				t.Errorf("watch: %v", err)
			}
		}()
	}
	for _, channel := range updates {
		select {
		case update := <-channel:
			if len(update.Unread) != 0 || len(update.Alerts) != 0 {
				t.Fatal("initial history replayed", update)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	_, err = sessions.Send(ctx, resource.SessionSendRequest_builder{Ref: ref, RunId: ptr(s.GetStatus().GetRunId()), ClientId: ptr("attention-send"), Text: ptr("Tell me a short story.")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	through := uint64(0)
	for _, channel := range updates {
		for {
			select {
			case update := <-channel:
				if entry, ok := update.Unread[s.GetRuntimeId()]; ok {
					through = max(through, entry.Seq)
					goto received
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	received:
	}
	if err := first.MarkConversationRead(ctx, s.GetRuntimeId(), through); err != nil {
		t.Fatal(err)
	}
	for _, channel := range updates {
		for {
			select {
			case update := <-channel:
				if len(update.Unread) == 0 {
					goto cleared
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	cleared:
	}
	state, err := resource.NewSessionServiceClient(two).ReadState(ctx, resource.SessionReadStateRequest_builder{Ref: ref}.Build())
	if err != nil || state.GetThroughSeq() != through {
		t.Fatal("read state did not cross clients", state, err)
	}
}

func ptr[T any](v T) *T { return &v }
