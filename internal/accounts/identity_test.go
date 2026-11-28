package accounts

import (
	"strings"
	"testing"
)

func claudeCredential(account, organization string) []byte {
	v := `{"claudeAiOauth":{"accessToken":"synthetic"`
	if account != "" {
		v += `,"accountUuid":"` + account + `"`
	}
	if organization != "" {
		v += `,"organizationUuid":"` + organization + `"`
	}
	return []byte(v + `}}`)
}

func TestCredentialIdentityTellsProviderAccountsApart(t *testing.T) {
	one, err := CredentialIdentity("claude", claudeCredential("account-one", "org-one"))
	if err != nil || one == "" {
		t.Fatal("identity not derived", one, err)
	}
	again, err := CredentialIdentity("claude", claudeCredential("account-one", "org-one"))
	if err != nil || again != one {
		t.Fatal("identity is not stable", again, err)
	}
	// Both halves have to count: the same person in a different organization
	// spends a different weekly limit.
	for _, other := range [][2]string{{"account-two", "org-one"}, {"account-one", "org-two"}} {
		v, err := CredentialIdentity("claude", claudeCredential(other[0], other[1]))
		if err != nil {
			t.Fatal(err)
		}
		if v == one {
			t.Fatal("different provider account read as the same one", other)
		}
	}
	// Nothing about the account may be recoverable from what gets pinned.
	if strings.Contains(one, "account-one") || strings.Contains(one, "org-one") {
		t.Fatal("identity exposes the account it describes", one)
	}
}

func TestCredentialIdentityIsEmptyWhenItCannotTell(t *testing.T) {
	// A Claude credential that carries no account of its own, as older releases
	// wrote and as a refresh has been seen to leave behind.
	v, err := CredentialIdentity("claude", claudeCredential("", ""))
	if err != nil || v != "" {
		t.Fatal("absent account should read as unknown, not as a pin", v, err)
	}
	// Codex pins its subject centrally instead; see CentralLogin.
	v, err = CredentialIdentity("codex", []byte(`{"tokens":{"id_token":"x.y.z"}}`))
	if err != nil || v != "" {
		t.Fatal("codex identity should not be pinned here", v, err)
	}
	if _, err := CredentialIdentity("claude", []byte("not json")); err == nil {
		t.Fatal("unreadable credential accepted")
	}
}

func TestPinnedIdentityRefusesADifferentProviderAccount(t *testing.T) {
	one, _ := CredentialIdentity("claude", claudeCredential("account-one", "org-one"))
	two, _ := CredentialIdentity("claude", claudeCredential("account-two", "org-one"))

	record, err := PinnedIdentity("", one)
	if err != nil || record != one {
		t.Fatal("first login should pin what it reached", record, err)
	}
	record, err = PinnedIdentity(one, one)
	if err != nil || record != "" {
		t.Fatal("same account should be accepted with nothing to write", record, err)
	}
	// The pin is what makes an account alias mean an account, so a login that
	// silently reached somebody else is refused rather than installed.
	if _, err = PinnedIdentity(one, two); err == nil {
		t.Fatal("different account accepted")
	} else if !strings.Contains(err.Error(), "clear") {
		t.Fatal("refusal does not say how to change accounts deliberately", err)
	}
	// Unknown leaves the pin as it was rather than dropping it.
	record, err = PinnedIdentity(one, "")
	if err != nil || record != "" {
		t.Fatal("unknown identity must not disturb the pin", record, err)
	}
}
