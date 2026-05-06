package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	in := Config{
		Token:           "ccp_test_token",
		WatchedProjects: []string{"gh/a/b", "gh/c/d"},
		BranchFilter:    "mine",
		RefreshInterval: 60,
	}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch:\nwant %+v\ngot  %+v", in, out)
	}
}

func TestLoadMissingFileReturnsZero(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Token != "" || len(cfg.WatchedProjects) > 0 {
		t.Errorf("missing file should yield zero Config; got %+v", cfg)
	}
}

func TestPathRespectsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/example")
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/example", "circleci-tui", "config.toml")
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}
