package websandbox

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Simulate a small shell over the same Terminal stream. No command execution,
// filesystem, Manager, Docker or event journal is involved.
func (p *Projects) Terminal(stream grpc.BidiStreamingServer[resource.ProjectTerminalRequest, resource.ProjectTerminalReply]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	validSize := func(r *resource.ProjectTerminalRequest) bool {
		return r.GetColumns() >= 1 && r.GetColumns() <= 500 && r.GetRows() >= 1 && r.GetRows() <= 100
	}
	if !first.HasRef() || first.HasInput() || !validSize(first) {
		return status.Error(codes.InvalidArgument, "project and terminal size required")
	}
	project, err := p.Get(stream.Context(), resource.ProjectGetRequest_builder{Ref: first.GetRef()}.Build())
	if err != nil {
		return err
	}
	ready := true
	if err = stream.Send(resource.ProjectTerminalReply_builder{Ready: &ready}.Build()); err != nil {
		return err
	}
	write := func(text string) error {
		return stream.Send(resource.ProjectTerminalReply_builder{Output: []byte(text)}.Build())
	}
	prompt := "sandbox@" + project.GetRuntimeId() + ":/workspace$ "
	if err = write("WASM simulated shell (help for commands)\r\n" + prompt); err != nil {
		return err
	}
	line, pending := "", ""
	for {
		r, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if r.HasRef() || ((r.HasColumns() || r.HasRows()) && (!validSize(r) || r.HasInput())) || (r.HasInput() && len(r.GetInput()) > 32768) || (!r.HasInput() && !r.HasColumns() && !r.HasRows()) {
			return status.Error(codes.InvalidArgument, "invalid terminal frame")
		}
		pending += string(r.GetInput())
		clear(r.GetInput())
		for pending != "" && utf8.FullRuneInString(pending) {
			ch, n := utf8.DecodeRuneInString(pending)
			pending = pending[n:]
			output := ""
			switch ch {
			case '\r', '\n':
				command := strings.TrimSpace(line)
				line = ""
				output = "\r\n"
				fields := strings.Fields(command)
				if len(fields) > 0 {
					switch fields[0] {
					case "help":
						output += "Simulated commands: pwd, ls, cat <file>, echo <text>, clear, exit\r\n"
					case "pwd":
						output += "/workspace\r\n"
					case "ls":
						output += ".devcontainer  README.md  src\r\n"
					case "echo":
						output += strings.TrimSpace(strings.TrimPrefix(command, "echo")) + "\r\n"
					case "cat":
						if len(fields) != 2 {
							output += "Usage: cat <file>\r\n"
						} else {
							name := fields[1]
							if !strings.HasPrefix(name, "/") {
								name = "/workspace/" + name
							}
							if text, ok := files(project.GetRuntimeId())[name]; ok {
								output += strings.ReplaceAll(text, "\n", "\r\n")
							} else {
								output += "File not found\r\n"
							}
						}
					case "clear":
						output = "\x1b[2J\x1b[H"
					case "exit":
						if err = write(output); err != nil {
							return err
						}
						exited := true
						return stream.Send(resource.ProjectTerminalReply_builder{Exited: &exited}.Build())
					default:
						output += fmt.Sprintf("%s: unavailable in simulated shell\r\n", fields[0])
					}
				}
				output += prompt
			case 3:
				line = ""
				output = "^C\r\n" + prompt
			case 4:
				if line == "" {
					exited := true
					return stream.Send(resource.ProjectTerminalReply_builder{Exited: &exited}.Build())
				}
			case 127, 8:
				if len(line) > 0 {
					_, size := utf8.DecodeLastRuneInString(line)
					line = line[:len(line)-size]
					output = "\b \b"
				}
			case 23: // Ctrl+W: erase trailing whitespace and the preceding word.
				trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
				end := len(trimmed)
				for end > 0 {
					ch, size := utf8.DecodeLastRuneInString(trimmed[:end])
					if unicode.IsSpace(ch) {
						break
					}
					end -= size
				}
				if end < len(line) {
					line = line[:end]
					// Redraw the line: erasing one cell per rune fails for wide glyphs.
					output = "\r\x1b[2K" + prompt + line
				}
			default:
				if ch >= 32 && ch != utf8.RuneError && len(line) < 8192 {
					line += string(ch)
					output = string(ch)
				}
			}
			if output != "" {
				if err = write(output); err != nil {
					return err
				}
			}
		}
	}
}
