package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/agustinfranchetti/circleci-tui/internal/cci"
	"github.com/agustinfranchetti/circleci-tui/internal/config"
)

const tokenURL = "https://app.circleci.com/settings/user/tokens"

// Login runs the `circleci-tui login` flow: prompts for a token (masked,
// piped through `golang.org/x/term`), verifies it by calling an MCP tool that
// requires auth, and persists the token to ~/.config/circleci-tui/config.toml.
//
// Why verify before saving: a typo'd token still passes the JSON-RPC layer
// (the MCP server doesn't validate it on startup), so without a verification
// round-trip the user wouldn't notice they pasted nonsense until next launch.
func Login(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	tokenFlag := fs.String("token", "", "non-interactive token (skips prompt)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "Open this URL to create a Personal API Token:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  "+tokenURL)
	fmt.Fprintln(os.Stderr, "")

	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		t, err := readMaskedToken()
		if err != nil {
			return err
		}
		token = t
	}
	if token == "" {
		return errors.New("no token provided")
	}

	fmt.Fprint(os.Stderr, "verifying… ")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc, err := cci.New(ctx, token)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗")
		return err
	}
	defer svc.Close()
	projects, err := svc.ListFollowedProjects(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗")
		return fmt.Errorf("token verification failed: %w", err)
	}
	fmt.Fprintln(os.Stderr, "✓")

	cfg, _ := config.Load()
	cfg.Token = token
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	path, _ := config.Path()
	fmt.Fprintf(os.Stderr, "✓ saved to %s (mode 0600)\n", path)
	fmt.Fprintf(os.Stderr, "✓ token is valid — you follow %d projects on CircleCI\n", len(projects))
	return nil
}

// Logout clears the persisted token without touching anything else in config
// (watched projects, refresh interval, etc. survive). Idempotent.
func Logout(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "no token to clear")
		return nil
	}
	cfg.Token = ""
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "✓ token cleared")
	return nil
}

func readMaskedToken() (string, error) {
	fmt.Fprint(os.Stderr, "Paste token: ")
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// Piped input — fall back to a plain ReadString. Useful for scripts
		// that pipe a token in without using --token.
		var buf [4096]byte
		n, err := os.Stdin.Read(buf[:])
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(buf[:n])), nil
	}
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
