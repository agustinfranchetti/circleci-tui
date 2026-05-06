# circleci-tui

A tiny CircleCI TUI. Lazygit-style stacked tree across all the projects you
follow, color-coded statuses, one-keystroke actions, log panel on the right.
Written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
talks to CircleCI through the
[official MCP server](https://github.com/CircleCI-Public/mcp-server-circleci)
plus the v2 REST API.

> **Status: pre-release.** The npm install path requires a tagged GitHub
> Release that hasn't been cut yet — for now, build from source (see below)
> or wait for `v0.1.0`. The release pipeline (`goreleaser` + npm publish) is
> wired up; it activates the moment a `v*` tag is pushed.

## Install (once `v0.1.0` is out)

```bash
npm i -g circleci-tui
circleci-tui login         # paste a Personal API Token
circleci-tui config        # pick which followed projects to watch
circleci-tui               # launch the TUI
```

The npm package is a tiny wrapper — at install time it downloads the matching
prebuilt binary from GitHub Releases for your OS and architecture, verifies
the SHA256, and drops it into your `$PATH`.

## Install today (build from source)

```bash
git clone https://github.com/agustinfranchetti/circleci-tui
cd circleci-tui
go build -o circleci-tui .
./circleci-tui demo        # try the UI on fixture data — no token needed
./circleci-tui login       # paste your Personal API Token
./circleci-tui config      # pick which followed projects to watch
./circleci-tui             # launch the live TUI
```

Or, once a Go-installable tag is up:

```bash
go install github.com/agustinfranchetti/circleci-tui@latest
```

## Quick start

You'll need:
- A CircleCI [Personal API Token](https://app.circleci.com/settings/user/tokens)
- Node.js (the MCP server runs under `npx`; the TUI itself is a Go binary)

```bash
$ circleci-tui login
Open this URL to create a Personal API Token:

  https://app.circleci.com/settings/user/tokens

Paste token: ****************
verifying… ✓
✓ saved to /Users/you/.config/circleci-tui/config.toml (mode 0600)

$ circleci-tui config
fetching followed projects from CircleCI…
[interactive picker — space to toggle, ⏎ to accept]

$ circleci-tui
[the TUI launches]
```

## Keybindings

| Key            | What it does                                                |
|----------------|-------------------------------------------------------------|
| `↑↓` / `j k`   | Navigate the tree                                           |
| `⏎` / `space`  | Expand / collapse the current project or pipeline           |
| `+` / `-`      | Expand all / collapse all                                   |
| `tab`          | Open the project picker (focus on a single repo)            |
| `e`            | Toggle the right-side log panel for the selected job        |
| `a`            | Open the actions menu (rerun / cancel / approve / browser)  |
| `/`            | Filter pipelines by branch / repo / ticket / status         |
| `m`            | Toggle "mine only" — pipelines triggered by you             |
| `r`            | Force a refresh (auto-refresh runs every 30 s)              |
| `esc`          | Peel off the most recent overlay / filter / focus           |
| `q`            | Quit                                                        |

When the log panel is open: `PgUp`/`PgDn`, `Ctrl-U`/`Ctrl-D` for half-page,
`g`/`G` for top/bottom. (`j/k`/`↑↓` keep navigating the tree so you can
glance at other rows without losing the panel.)

## Configuration

Lives at `~/.config/circleci-tui/config.toml` (mode `0600` since it stores
your token). Created and edited by `circleci-tui login` and
`circleci-tui config`; safe to hand-edit too.

```toml
token = "ccp_..."                         # set by `login`
watched_projects = [                      # set by `config`
  "gh/your-org/your-repo",
  "gh/your-org/another-repo",
]
mine = "your-username"                    # optional override
refresh_interval_seconds = 30             # optional
```

Environment variable overrides:

- `CIRCLECI_TOKEN` — overrides the persisted token (handy for CI / one-offs)
- `CIRCLECI_TUI_MINE` — overrides the "mine only" actor match

## Architecture

```
 +-----------------------+
 |  Bubble Tea TUI       |
 +-----------+-----------+
             |
             v
 +-----------------------+
 |  internal/cci.Service |   facade — TUI doesn't care which transport
 +--------+-----+--------+
          |     |
       MCP|     |REST
          v     v
   npx @circleci/      CircleCI v2 API
   mcp-server-circleci
```

The CircleCI MCP server doesn't expose `list_recent_pipelines` or
`cancel_workflow` / `approve_job`, so we use the REST API directly for those
endpoints. The MCP transport handles things it does well: `list_followed_
projects`, `get_build_failure_logs`, `rerun_workflow`, `find_flaky_tests`.
Both transports are authed with the same `CIRCLECI_TOKEN`.

## Cross-compile a release locally

```bash
goreleaser release --snapshot --clean
ls dist/
```

## License

[MIT](./LICENSE)
