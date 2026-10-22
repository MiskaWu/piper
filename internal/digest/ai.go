package digest

import (
	"fmt"
	"log"
	"strings"
)

// Commander abstracts command execution for AI summarization.
type Commander interface {
	Output(name string, args ...string) ([]byte, error)
}

// Summarizer generates AI summaries for project digests.
type Summarizer struct {
	cmd   Commander
	model string
}

// NewSummarizer creates a digest summarizer.
// If model is empty, defaults to "sonnet".
func NewSummarizer(cmd Commander, model string) *Summarizer {
	if model == "" {
		model = "sonnet"
	}
	return &Summarizer{cmd: cmd, model: model}
}

// SummarizeProject generates a progress summary for a project's daily work.
// Returns the AI-generated summary string, or empty string on error (non-fatal).
func (s *Summarizer) SummarizeProject(projectName string, commits []string, conversations []string) string {
	if len(commits) == 0 && len(conversations) == 0 {
		return ""
	}

	prompt := buildPrompt(projectName, commits, conversations)

	out, err := s.cmd.Output("claude", "-p", "--model", s.model, "--max-turns", "1", "--output-format", "text", prompt)
	if err != nil {
		log.Printf("WARNING: digest AI summarize failed for project %s: %v", projectName, err)
		return ""
	}

	return strings.TrimSpace(string(out))
}

func buildPrompt(projectName string, commits []string, conversations []string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "你是工作日報歸納助手。根據以下資料，歸納「%s」專案今天的主要進展。\n", projectName)
	b.WriteString("規則：\n")
	b.WriteString("- 聚焦功能/需求層面的推進，不要列出瑣碎的 bug 修復\n")
	b.WriteString("- 使用繁體中文\n")
	b.WriteString("- 直接輸出 2-3 句歸納，不要加標題或項目符號\n")
	b.WriteString("- 如果只有研究/討論沒有具體產出，簡短描述研究方向即可\n")
	b.WriteString("\n[Git Commits]\n")
	for _, c := range commits {
		b.WriteString(c)
		b.WriteByte('\n')
	}
	b.WriteString("\n[工作對話]\n")
	for _, c := range conversations {
		b.WriteString(c)
		b.WriteByte('\n')
	}

	return b.String()
}
