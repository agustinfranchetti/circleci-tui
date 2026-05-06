package cci

import (
	"os"
	"os/exec"
	"strings"
)

// CurrentActor returns the best-effort guess at the current user's CircleCI
// actor login — used by the TUI's "mine only" filter.
//
// Order of preference:
//  1. CIRCLECI_TUI_MINE env var (explicit user override)
//  2. The `mine` field from the persisted config file
//  3. git config user.email's local-part (works when the dev's email handle
//     matches their git/GitHub username — common at smaller orgs)
//  4. $USER (last resort)
func CurrentActor() string {
	if v := strings.TrimSpace(os.Getenv("CIRCLECI_TUI_MINE")); v != "" {
		return v
	}
	if cfg, err := configLoad(); err == nil {
		if v := strings.TrimSpace(cfg.Mine); v != "" {
			return v
		}
	}
	if email := gitConfig("user.email"); email != "" {
		if i := strings.Index(email, "@"); i > 0 {
			return email[:i]
		}
	}
	return strings.TrimSpace(os.Getenv("USER"))
}

func gitConfig(key string) string {
	out, err := exec.Command("git", "config", "--global", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
