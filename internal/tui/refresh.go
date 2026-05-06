package tui

import (
	"context"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agustinfranchetti/circleci-tui/internal/cci"
	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

// loadedMsg carries the result of a full refresh — either a freshly populated
// project list, or the error from whichever fetch failed first.
type loadedMsg struct {
	projects []model.Project
	at       time.Time
	err      error
}

// tickMsg is the periodic refresh trigger.
type tickMsg struct{}

// loadAllCmd kicks off a refresh: for each watched project, hydrate its tree
// in parallel via the Service. A 25-pipeline cap per project keeps the UI snappy
// on busy repos.
func loadAllCmd(svc *cci.Service, watch []cci.Project) tea.Cmd {
	const perProjectLimit = 25
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		type result struct {
			i   int
			p   model.Project
			err error
		}
		ch := make(chan result, len(watch))
		for i, w := range watch {
			i, w := i, w
			go func() {
				p, err := svc.LoadProject(ctx, w.Slug, w.Name, perProjectLimit)
				ch <- result{i: i, p: p, err: err}
			}()
		}
		out := make([]model.Project, len(watch))
		var firstErr error
		for range watch {
			r := <-ch
			if r.err != nil && firstErr == nil {
				firstErr = r.err
			}
			out[r.i] = r.p
		}
		// Filter empties (failed loads) and stable-sort: failed projects first,
		// then running, then everything else; secondary sort by name.
		filtered := out[:0]
		for _, p := range out {
			if p.Slug != "" {
				filtered = append(filtered, p)
			}
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].WorstStatus() != filtered[j].WorstStatus() {
				return statusRank(filtered[i].WorstStatus()) > statusRank(filtered[j].WorstStatus())
			}
			return filtered[i].Name < filtered[j].Name
		})
		return loadedMsg{projects: filtered, at: time.Now(), err: firstErr}
	}
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

func statusRank(s model.Status) int {
	switch s {
	case model.StatusFailed:
		return 5
	case model.StatusOnHold:
		return 4
	case model.StatusRunning:
		return 3
	case model.StatusCancelled:
		return 2
	case model.StatusSuccess:
		return 1
	default:
		return 0
	}
}
