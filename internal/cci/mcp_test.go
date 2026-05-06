package cci

import "testing"

func TestParseFollowedProjects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []apiFollowedProject
	}{
		{
			name: "happy path matches upstream MCP server output",
			in: "Projects followed:\n" +
				"1. my-app (projectSlug: gh/acme/my-app)\n" +
				"2. another-service (projectSlug: gh/acme/another-service)\n" +
				"\n" +
				"Please have the user choose one of these projects by name or number.",
			want: []apiFollowedProject{
				{Slug: "gh/acme/my-app", Name: "my-app"},
				{Slug: "gh/acme/another-service", Name: "another-service"},
			},
		},
		{
			name: "tolerates leading WARNING line and trailing prose",
			in: "WARNING: Not all projects were included due to pagination limits or timeout.\n\n" +
				"Projects followed:\n" +
				"1. only-one (projectSlug: bb/team/only-one)\n" +
				"\n" +
				"trailing instructions for the LLM",
			want: []apiFollowedProject{{Slug: "bb/team/only-one", Name: "only-one"}},
		},
		{
			name: "names with spaces and parens preserved",
			in: "Projects followed:\n" +
				"1. my service (web) (projectSlug: gh/acme/my-service)\n",
			want: []apiFollowedProject{{Slug: "gh/acme/my-service", Name: "my service (web)"}},
		},
		{
			name: "empty input returns nil",
			in:   "",
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseFollowedProjects(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("len(got)=%d want %d; got=%+v", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("idx %d: got %+v want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}
