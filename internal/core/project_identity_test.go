package core

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/projectid"
	"testing"
)

func TestOldSessionProjectIdentityKeepsAuthenticationScope(t *testing.T) {
	legacy := "0123456789abcdef01234567"
	var s Session
	if err := json.Unmarshal([]byte(`{"project_id":"`+legacy+`","auth_binding":"existing"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.ProjectID != projectid.Canonical(legacy) || s.AuthProjectID() != legacy || s.AuthBinding != "existing" {
		t.Fatal(s)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var next Session
	if err = json.Unmarshal(b, &next); err != nil || next != s {
		t.Fatal(next, err)
	}
	if err = json.Unmarshal([]byte(`{"project_id":"`+projectid.New()+`","legacy_project_id":"`+legacy+`"}`), &next); err == nil {
		t.Fatal("cross-project authentication alias accepted")
	}
}
