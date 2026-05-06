package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/agustinfranchetti/circleci-tui/cmd"
	"github.com/agustinfranchetti/circleci-tui/internal/cci"
	"github.com/agustinfranchetti/circleci-tui/internal/config"
	"github.com/agustinfranchetti/circleci-tui/internal/model"
	"github.com/agustinfranchetti/circleci-tui/internal/tui"
)

// version is replaced at build time by goreleaser via -X main.version=...
// `dev` is what you'll see during local `go run` / `go build`.
var version = "dev"

func main() {
	sub := ""
	if len(os.Args) > 1 {
		sub = os.Args[1]
	}
	rest := []string{}
	if len(os.Args) > 2 {
		rest = os.Args[2:]
	}

	var err error
	switch sub {
	case "", "tui":
		err = runTUI()
	case "demo":
		err = tui.Run(model.Fixtures())
	case "probe":
		err = runProbe()
	case "login":
		err = cmd.Login(rest)
	case "logout":
		err = cmd.Logout(rest)
	case "config":
		err = cmd.Configure(rest)
	case "-v", "--version", "version":
		fmt.Println("circleci-tui " + version)
		return
	case "-h", "--help", "help":
		printHelp()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n", sub)
		printHelp()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("circleci-tui — a tiny CircleCI TUI")
	fmt.Println("usage: circleci-tui [tui|demo|probe|login|logout|config]")
	fmt.Println("  tui     (default) live TUI — needs a CircleCI token")
	fmt.Println("  demo    render fixture data — no token needed")
	fmt.Println("  probe   list followed projects via MCP and exit")
	fmt.Println("  login   paste a CircleCI personal API token and persist it")
	fmt.Println("  logout  clear the persisted token")
	fmt.Println("  config  pick which followed projects to watch")
	fmt.Println("  version print the version and exit")
}

// runTUI launches the live TUI: spin up the Service, fetch the followed
// project list, hand both to the TUI which then auto-refreshes on a ticker.
// Token resolution: $CIRCLECI_TOKEN > config file > exit with a clear nudge.
func runTUI() error {
	token := cci.LoadToken()
	if token == "" {
		fmt.Fprintln(os.Stderr, "no CircleCI token configured.")
		fmt.Fprintln(os.Stderr, "  run `circleci-tui login` to set one up,")
		fmt.Fprintln(os.Stderr, "  or `circleci-tui demo` to see the UI on fixture data.")
		os.Exit(2)
	}
	ctx := context.Background()
	svc, err := cci.New(ctx, token)
	if err != nil {
		return err
	}
	defer svc.Close()
	all, err := svc.ListFollowedProjects(ctx)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Fprintln(os.Stderr, "no followed projects found on your CircleCI account.")
		os.Exit(2)
	}
	cfg, _ := config.Load()
	watch := filterWatched(all, cfg.WatchedProjects)
	if len(watch) == 0 {
		fmt.Fprintln(os.Stderr, "no projects in your watch list — run `circleci-tui config` to pick some.")
		os.Exit(2)
	}
	opts := tui.LiveOptions{MineOnAtStartup: cfg.BranchFilter == "mine"}
	if cfg.RefreshInterval > 0 {
		opts.RefreshInterval = time.Duration(cfg.RefreshInterval) * time.Second
	}
	return tui.RunLive(svc, watch, opts)
}

// filterWatched returns the subset of `all` whose slugs appear in the
// configured allowlist. Empty allowlist = watch everything followed (the
// fresh-install case before the user has run `config`).
func filterWatched(all []cci.Project, watchedSlugs []string) []cci.Project {
	if len(watchedSlugs) == 0 {
		return all
	}
	keep := map[string]bool{}
	for _, s := range watchedSlugs {
		keep[s] = true
	}
	var out []cci.Project
	for _, p := range all {
		if keep[p.Slug] {
			out = append(out, p)
		}
	}
	return out
}

// runProbe is a dev command: connects to MCP, lists followed projects,
// prints them. Verifies token + transport end-to-end without launching the TUI.
func runProbe() error {
	ctx := context.Background()
	svc, err := cci.New(ctx, cci.LoadToken())
	if err != nil {
		return err
	}
	defer svc.Close()
	projects, err := svc.ListFollowedProjects(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("followed projects (%d):\n", len(projects))
	for _, p := range projects {
		fmt.Printf("  %s  (%s)\n", p.Slug, p.Name)
	}
	return nil
}
