package transport

import (
	"context"
	"google.golang.org/grpc/metadata"
	"testing"
)

func TestProjectCapability(t *testing.T) {
	for _, values := range [][]string{nil, {"Bearer wrong"}, {"Bearer secret", "Bearer secret"}} {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{"authorization": values})
		if RequireToken(ctx, "secret") == nil {
			t.Fatal("unauthorized request accepted")
		}
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer secret"))
	if e := RequireToken(ctx, "secret"); e != nil {
		t.Fatal(e)
	}
	if RequireToken(context.Background(), "") == nil {
		t.Fatal("empty capability accepted")
	}
}
