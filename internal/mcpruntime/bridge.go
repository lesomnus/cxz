package mcpruntime

import (
	"bufio"
"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"io"
	"net"
	"time"
)

type wireMessage struct {
	raw []byte
	err error
}
type envelope struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
}

func messages(ctx context.Context, r io.Reader) <-chan wireMessage {
	ch := make(chan wireMessage)
	go func() {
		defer close(ch)
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 4096), 4<<20)
		for s.Scan() {
			b := append([]byte(nil), s.Bytes()...)
			select {
			case ch <- wireMessage{raw: b}:
			case <-ctx.Done():
				return
			}
		}
		e := s.Err()
		if e == nil {
			e = io.EOF
		}
		select {
		case ch <- wireMessage{err: e}:
		case <-ctx.Done():
		}
	}()
	return ch
}

// Bridge reconnects on the next request after a runtime interruption. Only the
// MCP handshake is replayed. In-flight calls fail with an unknown-outcome error.
func Bridge(ctx context.Context, root, session, server string, in io.Reader, out io.Writer) error {
	if !sessionID(session) || mcpconfig.ValidateID(server) != nil {
		return fmt.Errorf("invalid MCP bridge identity")
	}
	launch, e := ReadLaunch(root, session)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var c net.Conn
	defer func() {
		if c != nil {
			c.Close()
		}
	}()
	incoming := messages(ctx, in)
	var remote <-chan wireMessage
	var remoteCancel context.CancelFunc
	pending := map[string]json.RawMessage{}
	var init, initialized []byte
	initializedOK := false
	send := func(w io.Writer, b []byte) error {
 b=bytes.TrimRight(b,"\r\n") _, e := w.Write(append(append([]byte(nil), b...), '\n')); return e }
	failure := func(id json.RawMessage) error {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": "MCP connection interrupted. The operation outcome may be unknown; verify state before retrying. A later request reconnects without restarting the agent."}})
		return send(out, b)
	}
	disconnect := func() error {
		if remoteCancel != nil {
			remoteCancel()
			remoteCancel = nil
		}
		if c != nil {
			c.Close()
		}
		c = nil
		remote = nil
		for _, id := range pending {
			if e := failure(id); e != nil {
				return e
			}
		}
		pending = map[string]json.RawMessage{}
		return nil
	}
	connect := func() error {
		q, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		d := net.Dialer{}
		conn, e := d.DialContext(q, "unix", Socket(root))
		if e != nil {
			return e
		}
		closeOnError := true
		defer func() {
			if closeOnError {
				conn.Close()
			}
		}()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if e = json.NewEncoder(conn).Encode(Hello{session, server, launch.Token}); e != nil {
			return e
		}
		reader := bufio.NewReader(conn)
		if initializedOK {
			if e = send(conn, init); e != nil {
				return e
			}
			var request envelope
			json.Unmarshal(init, &request)
			for {
				line, e := reader.ReadBytes('\n')
				if e != nil {
					return e
				}
				if len(line) > 4<<20 {
					return fmt.Errorf("MCP response too large")
				}
				var response struct {
					ID    json.RawMessage `json:"id"`
					Error json.RawMessage `json:"error"`
				}
				if json.Unmarshal(line, &response) != nil {
					return fmt.Errorf("invalid MCP handshake")
				}
				if string(response.ID) == string(request.ID) {
					if len(response.Error) > 0 {
						return fmt.Errorf("MCP reinitialization failed")
					}
					break
				}
				if e = send(out, line); e != nil {
					return e
				}
			}
			if len(initialized) > 0 {
				if e = send(conn, initialized); e != nil {
					return e
				}
			}
		}
		conn.SetDeadline(time.Time{})
		c = conn
		remoteCtx, stop := context.WithCancel(ctx)
		remoteCancel = stop
		remote = messages(remoteCtx, reader)
		closeOnError = false
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case v, ok := <-incoming:
			if !ok || v.err != nil {
				return nil
			}
			var msg envelope
			if json.Unmarshal(v.raw, &msg) != nil {
				return fmt.Errorf("invalid MCP message")
			}
			if msg.Method == "initialize" {
				init = append([]byte(nil), v.raw...)
				initializedOK = false
			}
			if msg.Method == "notifications/initialized" {
				initialized = append([]byte(nil), v.raw...)
				initializedOK = true
			}
			if c == nil {
				if msg.Method == "" || len(msg.ID) == 0 {
					continue
				}
				if e = connect(); e != nil {
					if e = failure(msg.ID); e != nil {
						return e
					}
					continue
				}
			}
			if msg.Method != "" && len(msg.ID) > 0 {
				pending[string(msg.ID)] = msg.ID
			}
			if e = send(c, v.raw); e != nil {
				if e = disconnect(); e != nil {
					return e
				}
			}
		case v := <-remote:
			if v.err != nil {
				if e = disconnect(); e != nil {
					return e
				}
				continue
			}
			var msg envelope
			if json.Unmarshal(v.raw, &msg) != nil {
				return fmt.Errorf("invalid MCP server message")
			}
			if msg.Method == "" {
				delete(pending, string(msg.ID))
			}
			if e = send(out, v.raw); e != nil {
				return e
			}
		}
	}
}
