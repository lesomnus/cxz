package transport

import (
	"context"
	"fmt"
	"io"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// rawCodec preserves protobuf bytes, including unknown fields across versions.
type rawCodec struct{}

func (rawCodec) Name() string { return "proto" }
func (rawCodec) Marshal(v any) ([]byte, error) {
	p, ok := v.(*[]byte)
	if !ok {
		return nil, fmt.Errorf("invalid proxy message")
	}
	return *p, nil
}
func (rawCodec) Unmarshal(b []byte, v any) error {
	p, ok := v.(*[]byte)
	if !ok {
		return fmt.Errorf("invalid proxy message")
	}
	*p = append((*p)[:0], b...)
	return nil
}

func authenticatedProxy(conn *grpc.ClientConn, token string) *grpc.Server {
	return grpc.NewServer(grpc.ForceServerCodec(rawCodec{}), grpc.MaxRecvMsgSize(8*1024*1024), grpc.MaxSendMsgSize(24*1024*1024), grpc.UnknownServiceHandler(func(_ any, downstream grpc.ServerStream) error {
		if RequireToken(downstream.Context(), token) != nil {
			return status.Error(codes.Unauthenticated, "invalid remote access token")
		}
		method, ok := grpc.MethodFromServerStream(downstream)
		if !ok {
			return status.Error(codes.Internal, "missing method")
		}
		ctx, cancel := context.WithCancel(downstream.Context())
		defer cancel()
		upstream, err := conn.NewStream(ctx, &grpc.StreamDesc{ClientStreams: true, ServerStreams: true}, method, grpc.ForceCodec(rawCodec{}))
		if err != nil {
			return err
		}
		requests := make(chan error, 1)
		go func() {
			for {
				var b []byte
				err := downstream.RecvMsg(&b)
				if err == io.EOF {
					requests <- upstream.CloseSend()
					return
				}
				if err != nil {
					requests <- err
					return
				}
				if err = upstream.SendMsg(&b); err != nil {
					requests <- err
					return
				}
			}
		}()
		responses := make(chan error, 1)
		go func() {
			headers, err := upstream.Header()
			if err != nil {
				responses <- err
				return
			}
			if err = downstream.SendHeader(headers); err != nil {
				responses <- err
				return
			}
			for {
				var b []byte
				err := upstream.RecvMsg(&b)
				if err != nil {
					downstream.SetTrailer(upstream.Trailer())
					if err == io.EOF {
						err = nil
					}
					responses <- err
					return
				}
				if err = downstream.SendMsg(&b); err != nil {
					responses <- err
					return
				}
			}
		}()
		select {
		case err := <-requests:
			if err != nil && err != io.EOF {
				return err
			}
			return <-responses
		case err := <-responses:
			return err
		}
	}))
}

// Expose forwards an existing local installation; no second runtime/DB is opened.
// TCP is plaintext and intended for loopback, a VPN, or another trusted tunnel.
func Expose(ctx context.Context, root, endpoint, token string, ready io.Writer) error {
	e, err := ParseEndpoint(endpoint)
	if err != nil {
		return err
	}
	if e.Scheme != "tcp" || token == "" {
		return fmt.Errorf("expose requires tcp://host:port and an authentication token")
	}
	conn, err := Dial(root)
	if err != nil {
		return err
	}
	defer conn.Close()
	ln, err := net.Listen("tcp", e.Address)
	if err != nil {
		return err
	}
	defer ln.Close()
	g := authenticatedProxy(conn, token)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			g.Stop()
		case <-done:
		}
	}()
	if ready != nil {
		fmt.Fprintf(ready, "cxz remote listener: tcp://%s (token required)\n", ln.Addr())
	}
	err = g.Serve(ln)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
