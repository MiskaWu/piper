package git

import (
	"fmt"
	"testing"
)

func TestCreateTag_Success(t *testing.T) {
	cmd := &mockCommander{output: []byte(""), err: nil}
	g := NewGraphFetcher(cmd)
	err := g.CreateTag("/repo", "v1.0.0", "abc1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateTag_MissingParams(t *testing.T) {
	g := NewGraphFetcher(&mockCommander{})
	tests := []struct {
		name            string
		dir, tag, hash  string
	}{
		{"empty dir", "", "v1", "abc"},
		{"empty name", "/repo", "", "abc"},
		{"empty hash", "/repo", "v1", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := g.CreateTag(tc.dir, tc.tag, tc.hash); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestCreateTag_GitError(t *testing.T) {
	cmd := &mockCommander{
		output: []byte("fatal: tag 'v1' already exists"),
		err:    fmt.Errorf("exit 128"),
	}
	g := NewGraphFetcher(cmd)
	err := g.CreateTag("/repo", "v1", "abc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteTag_Success(t *testing.T) {
	cmd := &mockCommander{output: []byte("Deleted tag 'v1'"), err: nil}
	g := NewGraphFetcher(cmd)
	err := g.DeleteTag("/repo", "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteTag_MissingParams(t *testing.T) {
	g := NewGraphFetcher(&mockCommander{})
	if err := g.DeleteTag("", "v1"); err == nil {
		t.Error("expected error for empty dir")
	}
	if err := g.DeleteTag("/repo", ""); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestListTags_Success(t *testing.T) {
	cmd := &mockCommander{
		output: []byte("v1.0.0\x00abc1234\nv2.0.0\x00def5678\n"),
	}
	g := NewGraphFetcher(cmd)
	tags, err := g.ListTags("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2", len(tags))
	}
	if tags[0].Name != "v1.0.0" || tags[0].Hash != "abc1234" {
		t.Errorf("tag[0] = %+v, want {v1.0.0 abc1234}", tags[0])
	}
	if tags[1].Name != "v2.0.0" || tags[1].Hash != "def5678" {
		t.Errorf("tag[1] = %+v, want {v2.0.0 def5678}", tags[1])
	}
}

func TestListTags_Empty(t *testing.T) {
	cmd := &mockCommander{output: []byte("")}
	g := NewGraphFetcher(cmd)
	tags, err := g.ListTags("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("got %d tags, want 0", len(tags))
	}
}
