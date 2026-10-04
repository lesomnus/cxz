//go:build js && wasm

// The design sandbox exposes simulated services only through a browser Worker.
package main

import (
	"context"
	"log"
	"net/url"
	"strconv"
	"syscall/js"
	"time"

	"github.com/lesomnus/cxz/internal/websandbox"
	drpc "github.com/lesomnus/grpc-dgram"
	"github.com/lesomnus/grpc-dgram/transport/jsport"
)

func main() {
	u, _ := url.Parse(js.Global().Get("location").Get("href").String())
	seed, _ := strconv.ParseUint(u.Query().Get("seed"), 10, 64)
	delay, err := strconv.Atoi(u.Query().Get("delay"))
	if err != nil {
		delay = 400
	}
	delay = max(20, min(delay, 2000))
	fake := websandbox.New(seed, time.Duration(delay)*time.Millisecond)
	defer fake.Close()
	gateway := jsport.NewGateway()
	server := drpc.NewServer(gateway)
	fake.Register(server)
	log.Fatal(gateway.Serve(context.Background(), server))
}
