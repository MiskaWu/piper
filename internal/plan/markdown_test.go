package plan

import (
	"testing"
)

func TestExtractSection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		heading string
		want    string
	}{
		{
			name: "extract mid section",
			content: `# ADR-001

## 背景

一些背景內容。

## 決定

我們決定用方案 A。
理由如下。

## 後果

需要遷移舊資料。
`,
			heading: "決定",
			want:    "我們決定用方案 A。\n理由如下。",
		},
		{
			name: "extract last section",
			content: `## 結論

最終結論。
`,
			heading: "結論",
			want:    "最終結論。",
		},
		{
			name:    "heading not found",
			content: "## Foo\n\nbar\n",
			heading: "不存在",
			want:    "",
		},
		{
			name: "nested headings included",
			content: `## 決定

主要決定。

### 細節

一些細節。

## 後果

後果內容。
`,
			heading: "決定",
			want:    "主要決定。\n\n### 細節\n\n一些細節。",
		},
		{
			name: "h1 extraction",
			content: `# 結論

結論內容在這裡。

# 附錄

不要包含。
`,
			heading: "結論",
			want:    "結論內容在這裡。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSection(tt.content, tt.heading)
			if got != tt.want {
				t.Errorf("ExtractSection() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
