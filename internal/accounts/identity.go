package accounts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// An account alias is a label a person chose, and a project-local OAuth login
// is whatever the provider's consent page approved -- which is whoever the
// browser was already signed in as, with no chooser, whenever that is already
// somebody. So the alias cannot be trusted to say which provider account a
// session will spend, and nothing else in cxz said it either. This derives the
// one thing that can be compared: an opaque token that is equal exactly when
// two credentials belong to the same provider account.
//
// The token is a hash so that pinning an account to its resource reveals no
// subject, no address and no organization. What it costs is that a refusal
// cannot name the account it expected; it can only say that this is a
// different one, and point at clearing the pin.
func CredentialIdentity(agent string, credential []byte) (string, error) {
	if AgentKind(agent) != Claude {
		// A brokered Codex account pins its subject centrally, under its own
		// lock, before the credential is ever installed. See CentralLogin.
		return "", nil
	}
	var v struct {
		OAuth struct {
			Account      string `json:"accountUuid"`
			Organization string `json:"organizationUuid"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(credential, &v) != nil {
		return "", fmt.Errorf("invalid credential file")
	}
	// Absent in credentials written by older Claude releases, and in at least
	// one refreshed in place. Treating that as a failure would turn a provider
	// change into logins that stop working, so it means "cannot tell" instead:
	// an existing pin is left alone and no new one is taken.
	if v.OAuth.Account == "" {
		return "", nil
	}
	sum := sha256.Sum256([]byte("cxz.account-identity:claude\x00" + v.OAuth.Account + "\x00" + v.OAuth.Organization))
	return hex.EncodeToString(sum[:]), nil
}

// PinnedIdentity refuses a login that reached a different provider account than
// the one this account is pinned to. It answers with the identity to record,
// which is empty when there is nothing new to write down.
func PinnedIdentity(pinned, observed string) (string, error) {
	switch {
	case observed == "":
		return "", nil
	case pinned == "":
		return observed, nil
	case pinned != observed:
		return "", fmt.Errorf("this login is a different provider account than the one this cxz account is pinned to; clear its login identity first if the change is intended")
	}
	return "", nil
}
