package main

import (
	"reflect"
	"testing"

	"github.com/agustinfranchetti/circleci-tui/internal/cci"
)

func TestFilterWatchedAllowlist(t *testing.T) {
	all := []cci.Project{
		{Slug: "gh/a/x", Name: "x"},
		{Slug: "gh/a/y", Name: "y"},
		{Slug: "gh/a/z", Name: "z"},
	}
	got := filterWatched(all, []string{"gh/a/y", "gh/a/z"})
	want := []cci.Project{{Slug: "gh/a/y", Name: "y"}, {Slug: "gh/a/z", Name: "z"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterWatched = %+v, want %+v", got, want)
	}
}

func TestFilterWatchedEmptyAllowlistKeepsAll(t *testing.T) {
	all := []cci.Project{{Slug: "gh/a/x"}, {Slug: "gh/a/y"}}
	got := filterWatched(all, nil)
	if !reflect.DeepEqual(got, all) {
		t.Errorf("empty allowlist should keep all; got %+v", got)
	}
}
