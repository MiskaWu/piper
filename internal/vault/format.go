package vault

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// IdeaContext captures activity context at the time an idea was recorded.
type IdeaContext struct {
	Session string `json:"session" yaml:"session"`
	Kind    string `json:"kind" yaml:"kind"`
	Detail  string `json:"detail" yaml:"detail"`
	Time    int64  `json:"time" yaml:"time"`
}

// Idea represents a quick-capture inbox item stored as Markdown with YAML frontmatter.
type Idea struct {
	ID           int           `json:"id" yaml:"id"`
	Created      time.Time     `json:"created" yaml:"created"`
	Context      []IdeaContext `json:"context,omitempty" yaml:"context,omitempty"`
	Text         string        `json:"text" yaml:"-"`
	EnrichedDesc string        `json:"enrichedDesc,omitempty" yaml:"-"`
}

// marshalIdea serializes an Idea to Markdown with YAML frontmatter.
// Format:
//
//	---
//	id: 42
//	created: 2026-03-17T10:30:00+08:00
//	context:
//	  - session: ragent-piper-a3f1b2c9d0
//	    kind: output
//	    detail: "running tests"
//	    time: 1742193000
//	---
//	idea text here
//
//	---
//	## AI 補充描述
//	(enrichedDesc content)
func marshalIdea(idea *Idea) ([]byte, error) {
	// Build frontmatter struct
	fm := struct {
		ID      int           `yaml:"id"`
		Created time.Time     `yaml:"created"`
		Context []IdeaContext `yaml:"context,omitempty"`
	}{
		ID:      idea.ID,
		Created: idea.Created,
		Context: idea.Context,
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(fm); err != nil {
		return nil, fmt.Errorf("marshal idea frontmatter: %w", err)
	}
	enc.Close()
	buf.WriteString("---\n")
	buf.WriteString(idea.Text)

	if idea.EnrichedDesc != "" {
		buf.WriteString("\n\n---\n## AI 補充描述\n")
		buf.WriteString(idea.EnrichedDesc)
	}

	// Ensure trailing newline
	if buf.Len() > 0 && buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}

	return buf.Bytes(), nil
}

// parseIdeaFile parses Markdown with YAML frontmatter back into an Idea.
func parseIdeaFile(data []byte) (*Idea, error) {
	content := string(data)

	// Must start with ---
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("parse idea: missing frontmatter delimiter")
	}

	// Find closing ---
	rest := content[4:] // skip opening "---\n"
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return nil, fmt.Errorf("parse idea: missing closing frontmatter delimiter")
	}

	fmContent := rest[:idx]
	bodyContent := rest[idx+5:] // skip "\n---\n"

	// Parse frontmatter
	var fm struct {
		ID      int           `yaml:"id"`
		Created time.Time     `yaml:"created"`
		Context []IdeaContext `yaml:"context,omitempty"`
	}
	if err := yaml.Unmarshal([]byte(fmContent), &fm); err != nil {
		return nil, fmt.Errorf("parse idea frontmatter: %w", err)
	}

	idea := &Idea{
		ID:      fm.ID,
		Created: fm.Created,
		Context: fm.Context,
	}

	// Split body into text and enriched desc
	parts := strings.SplitN(bodyContent, "\n---\n## AI 補充描述\n", 2)
	idea.Text = strings.TrimRight(parts[0], "\n")
	if len(parts) == 2 {
		idea.EnrichedDesc = strings.TrimRight(parts[1], "\n")
	}

	return idea, nil
}
