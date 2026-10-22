package vault

import (
	"strings"
	"testing"
	"time"
)

func TestMarshalIdea_基本格式(t *testing.T) {
	idea := &Idea{
		ID:      42,
		Created: time.Date(2026, 3, 17, 10, 30, 0, 0, time.UTC),
		Text:    "把 inbox 也加入版本控制",
	}

	data, err := marshalIdea(idea)
	if err != nil {
		t.Fatalf("marshalIdea: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "---\n") {
		t.Error("expected to start with ---")
	}
	if !strings.Contains(content, "id: 42") {
		t.Error("expected id: 42 in frontmatter")
	}
	if !strings.Contains(content, "把 inbox 也加入版本控制") {
		t.Error("expected idea text in body")
	}
}

func TestMarshalIdea_含上下文(t *testing.T) {
	idea := &Idea{
		ID:      1,
		Created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Text:    "test",
		Context: []IdeaContext{
			{Session: "ragent-piper-abc", Kind: "output", Detail: "running tests", Time: 1000},
		},
	}

	data, err := marshalIdea(idea)
	if err != nil {
		t.Fatalf("marshalIdea: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "session: ragent-piper-abc") {
		t.Error("expected session in context")
	}
	if !strings.Contains(content, "kind: output") {
		t.Error("expected kind in context")
	}
}

func TestMarshalIdea_含enrichedDesc(t *testing.T) {
	idea := &Idea{
		ID:           1,
		Created:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Text:         "original text",
		EnrichedDesc: "AI generated description",
	}

	data, err := marshalIdea(idea)
	if err != nil {
		t.Fatalf("marshalIdea: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "## AI 補充描述") {
		t.Error("expected AI section header")
	}
	if !strings.Contains(content, "AI generated description") {
		t.Error("expected enriched description")
	}
}

func TestParseIdeaFile_基本解析(t *testing.T) {
	input := `---
id: 42
created: 2026-03-17T10:30:00Z
---
把 inbox 也加入版本控制
`
	idea, err := parseIdeaFile([]byte(input))
	if err != nil {
		t.Fatalf("parseIdeaFile: %v", err)
	}

	if idea.ID != 42 {
		t.Errorf("expected ID=42, got %d", idea.ID)
	}
	if idea.Text != "把 inbox 也加入版本控制" {
		t.Errorf("expected text '把 inbox 也加入版本控制', got %q", idea.Text)
	}
}

func TestParseIdeaFile_含enrichedDesc(t *testing.T) {
	input := `---
id: 1
created: 2026-01-01T00:00:00Z
---
original text

---
## AI 補充描述
AI generated description
`
	idea, err := parseIdeaFile([]byte(input))
	if err != nil {
		t.Fatalf("parseIdeaFile: %v", err)
	}

	if idea.Text != "original text" {
		t.Errorf("expected text 'original text', got %q", idea.Text)
	}
	if idea.EnrichedDesc != "AI generated description" {
		t.Errorf("expected enrichedDesc 'AI generated description', got %q", idea.EnrichedDesc)
	}
}

func TestParseIdeaFile_含上下文(t *testing.T) {
	input := `---
id: 5
created: 2026-01-01T00:00:00Z
context:
  - session: ragent-piper-abc
    kind: output
    detail: running tests
    time: 1000
---
idea with context
`
	idea, err := parseIdeaFile([]byte(input))
	if err != nil {
		t.Fatalf("parseIdeaFile: %v", err)
	}

	if len(idea.Context) != 1 {
		t.Fatalf("expected 1 context, got %d", len(idea.Context))
	}
	if idea.Context[0].Session != "ragent-piper-abc" {
		t.Errorf("expected session 'ragent-piper-abc', got %q", idea.Context[0].Session)
	}
}

func TestRoundTrip_marshalThenParse(t *testing.T) {
	original := &Idea{
		ID:           42,
		Created:      time.Date(2026, 3, 17, 10, 30, 0, 0, time.UTC),
		Text:         "round trip test\nwith multiple lines",
		EnrichedDesc: "enriched content\nalso multiline",
		Context: []IdeaContext{
			{Session: "s1", Kind: "output", Detail: "test", Time: 1000},
			{Session: "s2", Kind: "input", Detail: "query", Time: 2000},
		},
	}

	data, err := marshalIdea(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := parseIdeaFile(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.ID != original.ID {
		t.Errorf("ID: expected %d, got %d", original.ID, parsed.ID)
	}
	if parsed.Text != original.Text {
		t.Errorf("Text: expected %q, got %q", original.Text, parsed.Text)
	}
	if parsed.EnrichedDesc != original.EnrichedDesc {
		t.Errorf("EnrichedDesc: expected %q, got %q", original.EnrichedDesc, parsed.EnrichedDesc)
	}
	if len(parsed.Context) != len(original.Context) {
		t.Errorf("Context: expected %d items, got %d", len(original.Context), len(parsed.Context))
	}
}

func TestParseIdeaFile_無frontmatter分隔符報錯(t *testing.T) {
	_, err := parseIdeaFile([]byte("no frontmatter here"))
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParseIdeaFile_無結束分隔符報錯(t *testing.T) {
	_, err := parseIdeaFile([]byte("---\nid: 1\n"))
	if err == nil {
		t.Error("expected error for missing closing delimiter")
	}
}
