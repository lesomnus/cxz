package accounts

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// RefreshManaged delegates OAuth refresh to the pinned official Codex, never
// to a reimplementation of its OAuth endpoints. Caller holds broker.lock.
func RefreshManaged(ctx context.Context, root, account, binary string) error {
	cmd := exec.CommandContext(ctx, binary, "-c", `cli_auth_credentials_store="file"`, "-c", `model_provider="openai"`, "app-server", "--listen", "stdio://")
	cmd.Env = Environment(os.Environ(), centralRoot(root), account, "codex")
	cmd.Dir = Dir(centralRoot(root), account)
	cmd.Stderr = io.Discard
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("cannot start central Codex")
	}
	defer func() { input.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	enc := json.NewEncoder(input)
	call := func(id, method string, params any) error {
		if err := enc.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return err
		}
		for scanner.Scan() {
			var response struct {
				ID     string          `json:"id"`
				Error  json.RawMessage `json:"error"`
				Result struct {
					Account *struct {
						Type string `json:"type"`
					} `json:"account"`
				} `json:"result"`
			}
			if json.Unmarshal(scanner.Bytes(), &response) != nil {
				return fmt.Errorf("invalid central Codex response")
			}
			if response.ID != id {
				continue
			}
			if len(response.Error) > 0 && string(response.Error) != "null" {
				return fmt.Errorf("central Codex authentication failed; log in again")
			}
			if method == "account/read" && (response.Result.Account == nil || response.Result.Account.Type != "chatgpt") {
				return fmt.Errorf("central Codex is not logged in to ChatGPT")
			}
			return nil
		}
		return fmt.Errorf("central Codex authentication interrupted")
	}
	if err = call("init", "initialize", map[string]any{"clientInfo": map[string]string{"name": "cxz-auth", "version": "1"}}); err != nil {
		return err
	}
	if err = enc.Encode(map[string]any{"method": "initialized"}); err != nil {
		return err
	}
	return call("refresh", "account/read", map[string]any{"refreshToken": true})
}
