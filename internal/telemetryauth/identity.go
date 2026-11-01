// Package telemetryauth authenticates write-only, session-scoped telemetry.
// It is deliberately SDK-free so the standalone Collector can share the wire
// contract without adding Collector dependencies to the cxz executable.
package telemetryauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/resourceid"
)

const Socket = "/cxz/tools/telemetry/collector.sock"

type Identity struct {
	Project     string `json:"project"`
	Session     string `json:"session"`
	Account     string `json:"account"`
	Agent       string `json:"agent"`
	Run         string `json:"run"`
	Source      string `json:"source"`
	UsageSource string `json:"usage_source"`
	Scope       string `json:"scope,omitempty"`
	Parent      string `json:"parent,omitempty"`
}

type Session struct {
	Account string `json:"account"`
	Agent   string `json:"agent"`
	Scope   string `json:"scope,omitempty"`
	Parent  string `json:"parent,omitempty"`
}
type Registry struct {
	Secret   string             `json:"secret"`
	Sessions map[string]Session `json:"sessions"`
}

func ProjectKey(id string) (string, error) {
	id = strings.ReplaceAll(strings.ToLower(id), "-", "")
	b, err := hex.DecodeString(id)
	if err != nil || len(b) != 16 {
		return "", errors.New("invalid telemetry project identity")
	}
	return id, nil
}

func (i Identity) Validate() error {
	if _, err := ProjectKey(i.Project); err != nil {
		return err
	}
	if len(i.Session) > 128 || i.Session == "" || len(i.Account) > 128 || i.Account == "" || len(i.Run) > 128 || i.Run == "" {
		return errors.New("invalid telemetry session identity")
	}
	if i.Source != "native" && i.Source != "supervisor" {
		return errors.New("invalid telemetry source")
	}
	if i.UsageSource != "native" && i.UsageSource != "supervisor" {
		return errors.New("invalid telemetry usage source")
	}
	if len(i.Agent) > 64 || i.Agent == "" || len(i.Scope) > 64 || len(i.Parent) > 128 {
		return errors.New("invalid telemetry agent identity")
	}
	return nil
}

func Sign(secret string, i Identity) (string, error) {
	if secret == "" {
		return "", errors.New("missing telemetry capability")
	}
	if err := i.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(i)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Verify reads the current project registry so removed sessions and rotated
// capabilities stop accepting exports without restarting the Collector.
func Verify(directory, token string) (Identity, error) {
	var i Identity
	invalid := errors.New("unauthorized telemetry")
	if len(token) > 4096 {
		return i, invalid
	}
	payload, signature, ok := strings.Cut(token, ".")
	if !ok {
		return i, invalid
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return i, invalid
	}
	if json.Unmarshal(b, &i) != nil || i.Validate() != nil {
		return Identity{}, invalid
	}
	key, _ := ProjectKey(i.Project)
	f, err := os.Open(filepath.Join(directory, key+".json"))
	if err != nil {
		return Identity{}, invalid
	}
	defer f.Close()
	b, err = io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return Identity{}, invalid
	}
	var registry Registry
	if json.Unmarshal(b, &registry) != nil || registry.Secret == "" {
		return Identity{}, invalid
	}
	sum, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return Identity{}, invalid
	}
	mac := hmac.New(sha256.New, []byte(registry.Secret))
	mac.Write([]byte(payload))
	if !hmac.Equal(sum, mac.Sum(nil)) {
		return Identity{}, invalid
	}
	session, ok := registry.Sessions[i.Session]
	if !ok || session.Account != i.Account || session.Agent != i.Agent || session.Parent != i.Parent || (session.Scope != "" && session.Scope != i.Scope) {
		return Identity{}, invalid
	}
	return i, nil
}

// VerifyProject authenticates the runtime's loopback relay before reading a
// request body. The Collector additionally checks the live session registry.
func VerifyProject(project, secret, token string) (Identity, error) {
	var i Identity
	invalid := errors.New("unauthorized telemetry")
	if len(token) > 4096 {
		return i, invalid
	}
	payload, _, ok := strings.Cut(token, ".")
	if !ok {
		return i, invalid
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(b, &i) != nil || i.Validate() != nil {
		return Identity{}, invalid
	}
	key, err := ProjectKey(project)
	if err != nil {
		return Identity{}, invalid
	}
	other, _ := ProjectKey(i.Project)
	expected, err := Sign(secret, i)
	if err != nil || key != other || !hmac.Equal([]byte(token), []byte(expected)) {
		return Identity{}, invalid
	}
	return i, nil
}

func (i Identity) Attributes() map[string]string {
	project, _ := ProjectKey(i.Project)
	session := i.Session
	if i.Parent != "" {
		session = i.Parent
	}
	return map[string]string{
		"cxz.project.id":       project,
		"cxz.session.id":       hex.EncodeToString(resourceid.Bytes(8, session)),
		"cxz.runtime.id":       session,
		"cxz.account.id":       hex.EncodeToString(resourceid.Bytes(9, i.Account)),
		"cxz.account.alias":    i.Account,
		"cxz.run.id":           i.Run,
		"cxz.agent":            i.Agent,
		"cxz.telemetry.source": i.Source,
		"cxz.usage.source":     i.UsageSource,
		"cxz.scope":            i.Scope,
	}
}

func Bearer(header string) (string, error) {
	kind, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(kind, "Bearer") || token == "" {
		return "", fmt.Errorf("missing telemetry authorization")
	}
	return token, nil
}
