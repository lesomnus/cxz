// Deterministic Codex authentication/app-server fixture. Never ship as Codex.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	home := os.Getenv("CODEX_HOME")
	alias := filepath.Base(filepath.Dir(home))
	path := filepath.Join(home, "auth.json")
	emit := func(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
	writeAuth := func() {
		claims, _ := json.Marshal(map[string]string{"sub": "user-" + alias})
		v := map[string]any{"tokens": map[string]string{"id_token": "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".synthetic", "access_token": "synthetic-access-" + alias + fmt.Sprint(time.Now().UnixNano()), "refresh_token": "synthetic-refresh-" + alias, "account_id": "subject-" + alias}}
		raw, _ := json.Marshal(v)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			panic(err)
		}
	}
	if strings.Contains(strings.Join(os.Args[1:], " "), "login --device-auth") {
		writeAuth()
		fmt.Println("Synthetic device login complete; no real authentication")
		return
	}
	authorized := false
	subject := ""
	turn := 0
	finish := func() {
		emit(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]any{"id": "message", "type": "agentMessage", "text": "authenticated account=" + subject}}})
		emit(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"id": "turn", "status": "completed"}}})
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var v struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Type        string `json:"type"`
				AccessToken string `json:"accessToken"`
				AccountID   string `json:"chatgptAccountId"`
				ThreadID    string `json:"threadId"`
				Refresh     bool   `json:"refreshToken"`
				Input       []struct {
					Text string `json:"text"`
				} `json:"input"`
			} `json:"params"`
			Result struct {
				AccessToken string `json:"accessToken"`
				AccountID   string `json:"chatgptAccountId"`
			} `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &v) != nil {
			os.Exit(2)
		}
		reply := func(r any) { emit(map[string]any{"id": v.ID, "result": r}) }
		switch v.Method {
		case "initialize":
			reply(map[string]any{})
		case "account/rateLimits/read":
			reply(map[string]any{"rateLimits": map[string]any{"primary": map[string]any{"usedPercent": 60, "windowDurationMins": 300, "resetsAt": time.Now().Add(time.Hour).Unix()}}})
		case "thread/compact/start":
			reply(map[string]any{})
			emit(map[string]any{"method": "turn/started", "params": map[string]any{"turn": map[string]string{"id": "compact"}}})
			emit(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]string{"type": "contextCompaction", "id": "compact-item"}}})
			finish()
		case "account/read":
			if v.Params.Refresh {
				writeAuth()
			}
			reply(map[string]any{"account": map[string]string{"type": "chatgpt"}})
		case "account/login/start":
			if v.Params.Type != "chatgptAuthTokens" || v.Params.AccessToken == "" || v.Params.AccountID == "" {
				os.Exit(3)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				os.Exit(4)
			}
			if os.Getenv("OPENAI_API_KEY") != "" {
				os.Exit(5)
			}
			authorized = true
			subject = v.Params.AccountID
			reply(map[string]string{"type": "chatgptAuthTokens"})
		case "thread/start", "thread/resume":
			if !authorized {
				os.Exit(6)
			}
			id := v.Params.ThreadID
			if id == "" {
				id = "thread-" + alias
			}
			reply(map[string]any{"thread": map[string]string{"id": id}})
		case "turn/start":
			turn++
			reply(map[string]any{})
			emit(map[string]any{"method": "turn/started", "params": map[string]any{"turn": map[string]string{"id": "turn"}}})
			text := ""
			if len(v.Params.Input) > 0 {
				text = v.Params.Input[0].Text
			}
			switch text {
			case "refresh":
				emit(map[string]any{"id": "refresh", "method": "account/chatgptAuthTokens/refresh", "params": map[string]string{"previousAccountId": subject, "reason": "unauthorized"}})
			case "approval":
				emit(map[string]any{"id": "approval", "method": "item/commandExecution/requestApproval", "params": map[string]string{"command": "echo fixture"}})
			case "wait":
			default:
				finish()
			}
		case "turn/interrupt":
			reply(map[string]any{})
			emit(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]string{"id": "turn", "status": "interrupted"}}})
		case "":
			if string(v.ID) == `"refresh"` {
				if v.Result.AccountID != subject || v.Result.AccessToken == "" {
					os.Exit(7)
				}
				finish()
			}
			if string(v.ID) == `"approval"` {
				finish()
			}
		}
	}
}
