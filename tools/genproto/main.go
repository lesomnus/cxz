// Generate the Go protocol without requiring a system protoc installation.
package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/pluginpb"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	c := protocompile.Compiler{Resolver: &protocompile.SourceResolver{ImportPaths: []string{"internal/legacyproto"}}}
	fs, err := c.Compile(context.Background(), "cxz.proto")
	must(err)
	req := &pluginpb.CodeGeneratorRequest{FileToGenerate: []string{"cxz.proto"}, Parameter: proto.String("module=github.com/lesomnus/cxz")}
	req.ProtoFile = append(req.ProtoFile, protodesc.ToFileDescriptorProto(fs[0]))
	in, err := proto.Marshal(req)
	must(err)
	for _, plugin := range []string{"google.golang.org/protobuf/cmd/protoc-gen-go", "google.golang.org/grpc/cmd/protoc-gen-go-grpc"} {
		cmd := exec.Command("go", "run", plugin)
		cmd.Stdin = bytes.NewReader(in)
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		must(err)
		var res pluginpb.CodeGeneratorResponse
		must(proto.Unmarshal(out, &res))
		if res.GetError() != "" {
			panic(res.GetError())
		}
		for _, f := range res.File {
			name := strings.TrimSuffix(f.GetName(), ".pb.go") + "_wire.go"
			must(os.MkdirAll(filepath.Dir(name), 0755))
			must(os.WriteFile(name, []byte(f.GetContent()), 0644))
			fmt.Println(name)
		}
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
