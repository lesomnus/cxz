package taskapi

import (
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// WatchReadyHeader is sent after a changes-only subscription is listening.
const WatchReadyHeader = "cxz-watch-ready"

func WatchReady(out grpc.ServerStreamingServer[resource.TaskWatchResponse]) error {
	if err := out.SendHeader(metadata.Pairs(WatchReadyHeader, "1")); err != nil {
		return err
	}
	// Connect's transcoder flushes response headers with its first message.
	// A control frame makes readiness observable even on a quiet subscription.
	return out.Send(&resource.TaskWatchResponse{})
}
