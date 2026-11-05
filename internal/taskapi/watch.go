package taskapi

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// WatchReadyHeader is sent after a changes-only subscription is listening.
const WatchReadyHeader = "cxz-watch-ready"

func WatchReady(out grpc.ServerStream) error {
	return out.SendHeader(metadata.Pairs(WatchReadyHeader, "1"))
}
