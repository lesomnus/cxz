package workspace

import (
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEraseAccountClearsAuxiliaryProfilesAndOnlyItsOwnedVolume(t *testing.T) {
	for _, scenario := range []string{"owned", "unowned", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			unowned := scenario == "unowned"
			root := t.TempDir()
			bin := t.TempDir()
			log := filepath.Join(root, "docker.log")
			t.Setenv("CXZ_ACCOUNT_ERASE_LOG", log)
			t.Setenv("CXZ_ACCOUNT_ERASE_MISSING", scenario)
			if unowned {
				t.Setenv("CXZ_ACCOUNT_ERASE_UNOWNED", "1")
			} else {
				t.Setenv("CXZ_ACCOUNT_ERASE_UNOWNED", "0")
			}
			script := `#!/bin/sh
printf '%s\n' "$*" >> "$CXZ_ACCOUNT_ERASE_LOG"
case "$1 $2" in
 "volume ls")
  if [ "$CXZ_ACCOUNT_ERASE_MISSING" = missing ]; then printf '%s\n' 'cxz-owner-aux-other'; else printf '%s\n' 'cxz-owner-aux-work'; fi ;;
 "volume inspect")
  if [ "$CXZ_ACCOUNT_ERASE_UNOWNED" = 1 ]; then owner=foreign; else owner=owner; fi
  printf '[{"Labels":{"cxz.owner":"%s","cxz.project":"auxiliary:work"}}]\n' "$owner" ;;
 "volume rm") exit 0 ;;
 *) exit 90 ;;
esac
`
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			m := &Manager{Root: root, Owner: "owner"}
			c, err := m.auxiliaryController()
			if err != nil {
				t.Fatal(err)
			}
			profile := auxiliary.Profile{Enabled: true, Account: "work", Agent: "claude", Backend: accounts.ProjectLocalOAuth, Model: "model"}
			if _, err := c.Save("summary", profile); err != nil {
				t.Fatal(err)
			}
			profile.Account = "other"
			if _, err := c.Save("suggestion", profile); err != nil {
				t.Fatal(err)
			}
			if err := accounts.Prepare(filepath.Join(root, "central"), "work", "codex"); err != nil {
				t.Fatal(err)
			}
			subject := filepath.Join(accounts.Dir(filepath.Join(root, "central"), "work"), "subject.json")
			if err := os.WriteFile(subject, []byte("private identity"), 0600); err != nil {
				t.Fatal(err)
			}
			err = m.EraseAccount(t.Context(), "work", "claude", accounts.ProjectLocalOAuth)
			if unowned && err == nil {
				t.Fatal("unowned auth volume erased")
			}
			if !unowned && err != nil {
				t.Fatal(err)
			}
			if c.Config().Summary.Account != "" || c.Config().Suggestion.Account != "other" {
				t.Fatal("profile cleanup affected other accounts", c.Config())
			}
			calls, _ := os.ReadFile(log)
			if strings.Contains(string(calls), "volume rm") != (scenario == "owned") {
				t.Fatal("volume deletion mismatch", string(calls))
			}
			if _, err := os.Stat(subject); !os.IsNotExist(err) {
				t.Fatal("private profile retained", err)
			}
		})
	}
}
