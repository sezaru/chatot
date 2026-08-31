package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// rosterFile is the account-roster JSON living at the manager's base dir; it
// lists only the pairing-added accounts (the "default" account is implicit).
const rosterFile = "accounts.json"

// defaultAccountID is the always-present back-compat account whose state dir is
// the legacy $XDG_STATE_HOME/chatot itself (not accounts/<id>/).
const defaultAccountID = "default"

type rosterEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type roster struct {
	Accounts []rosterEntry `json:"accounts"`
}

// loadRoster reads path, returning an empty roster (no error) when the file is
// absent so first launch behaves exactly like the single-account case.
func loadRoster(path string) (roster, error) {
	var r roster
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return roster{}, nil
		}
		return roster{}, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return roster{}, err
	}
	return r, nil
}

func saveRoster(path string, r roster) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify turns a human label into a filesystem-safe account id slug, falling
// back to "account" when the label has no alphanumerics.
func slugify(label string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	s = slugNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "account"
	}
	return s
}
