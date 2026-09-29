package auxiliary

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercises the installed cxz helper entry point, profile layout, read-only
// container and stdio adapter without real credentials or billable model calls.
func TestAuxiliaryHelperDocker(t *testing.T) {
	if os.Getenv("CXZ_TEST_USE_DOCKER") != "1" {
		t.Skip("set CXZ_TEST_USE_DOCKER=1")
	}
	dir := t.TempDir()
	name := "cxz-aux-test-" + core.ID()
	image := name + ":test"
	run := func(bin string, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), bin, args...)
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("%s: %v\n%s", bin, e, b)
		}
		return b
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", filepath.Join(dir, "cxz"), "./cmd/cxz")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build helper: %v\n%s", e, b)
	}
	source := `package main
import("os";"bufio";"encoding/json")
func main(){
 for _,p:=range []string{"/workspace","/var/run/docker.sock","/cxz/state"}{if _,e:=os.Stat(p);e==nil{os.Exit(10)}}
 send:=func(s string){os.Stdout.WriteString(s+"\n")}
 scanner:=bufio.NewScanner(os.Stdin)
 for scanner.Scan(){var v struct{ID,Method string};json.Unmarshal(scanner.Bytes(),&v)
 switch v.Method{
 case "initialize":send("{\"id\":\"init\",\"result\":{}}")
 case "model/list":send("{\"id\":\"models\",\"result\":{\"data\":[{\"model\":\"test\",\"supportedReasoningEfforts\":[{\"reasoningEffort\":\"low\"}]}]}}")
 case "thread/start":send("{\"id\":\"thread\",\"result\":{\"thread\":{\"id\":\"thread\"}}}")
 case "turn/start":
 send("{\"method\":\"item/completed\",\"params\":{\"item\":{\"type\":\"agentMessage\",\"text\":\"{\\\"summary\\\":\\\"container verified\\\",\\\"suggestion\\\":\\\"\\\",\\\"checkpoint\\\":\\\"\\\"}\"}}}")
 send("{\"method\":\"turn/completed\",\"params\":{\"turn\":{\"status\":\"completed\"}}}")
 }
 }
}`
	if e := os.WriteFile(filepath.Join(dir, "probe.go"), []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	cmd = exec.CommandContext(t.Context(), "go", "build", "-o", filepath.Join(dir, "probe"), filepath.Join(dir, "probe.go"))
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build fake: %v\n%s", e, b)
	}
	for path, data := range map[string]string{"Dockerfile": "FROM scratch\nCOPY cxz probe /cxz/tools/\nCOPY auth.json /cxz/aux/accounts/work/config/auth.json\n", "auth.json": `{"tokens":{"access_token":"synthetic-test-only"}}`} {
		if e := os.WriteFile(filepath.Join(dir, path), []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	run("docker", "build", "-q", "-t", image, dir)
	defer exec.Command("docker", "image", "rm", image).Run()
	run("docker", "volume", "create", name)
	defer exec.Command("docker", "volume", "rm", name).Run()
	q := HelperInput{Input: Input{Profile: Profile{Account: "work", Agent: "codex", Backend: accounts.ProjectLocalOAuth, Model: "test", Effort: "low"}, Task: "summary", Text: "fake turn"}, Binary: "/cxz/tools/probe"}
	data, _ := json.Marshal(q)
	cmd = exec.CommandContext(t.Context(), "docker", "run", "--rm", "-i", "--read-only", "--network", "none", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m", "--mount", "type=volume,source="+name+",target=/cxz/aux", "--entrypoint", "/cxz/tools/cxz", image, "--state", "/cxz/aux", "_auxiliary-job")
	cmd.Stdin = strings.NewReader(string(data))
	b, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("isolated helper: %v\n%s", e, b)
	}
	var out Output
	if e = json.Unmarshal(b, &out); e != nil || out.Summary != "container verified" {
		t.Fatalf("helper result: %v %s", e, b)
	}
}
