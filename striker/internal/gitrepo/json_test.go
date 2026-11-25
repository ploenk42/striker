package gitrepo

import (
	"encoding/json"
	"testing"
)

func TestJSONTags(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{Commit{SHA: "a", Parents: []string{"b"}, Subject: "s", Refs: []int{1}, Fixes: []int{2}},
			`{"sha":"a","parents":["b"],"subject":"s","refs":[1],"fixes":[2]}`},
		{Worktree{Path: "/w", Branch: "x"}, `{"path":"/w","branch":"x"}`},
		{Hunk{OldStart: 1, OldLines: 2, NewStart: 3, NewLines: 4, Removed: []string{"r"}, Added: []string{"a"}},
			`{"old_start":1,"old_lines":2,"new_start":3,"new_lines":4,"removed":["r"],"added":["a"]}`},
	}
	for _, c := range cases {
		b, err := json.Marshal(c.v)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != c.want {
			t.Errorf("got %s, want %s", b, c.want)
		}
	}
}
