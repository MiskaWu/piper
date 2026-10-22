package devcontainer

import (
	"strings"
	"testing"
	"testing/fstest"
)

func testTemplateFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/base-dockerfile.md": &fstest.MapFile{
			Data: []byte("# Language layers reference\nGo, Python, Rust, Node.js"),
		},
		"templates/base.Dockerfile": &fstest.MapFile{
			Data: []byte("FROM node:22-bookworm-slim\n\nRUN apt-get update"),
		},
		"templates/init-firewall.sh": &fstest.MapFile{
			Data: []byte("#!/bin/bash\necho firewall"),
		},
		"templates/git-ssh-helper.sh": &fstest.MapFile{
			Data: []byte("#!/bin/sh\nssh \"$@\""),
		},
		"templates/piper-hook.mjs": &fstest.MapFile{
			Data: []byte("#!/usr/bin/env node\n// piper hook"),
		},
		"templates/piper-statusline.mjs": &fstest.MapFile{
			Data: []byte("#!/usr/bin/env node\n// piper statusline"),
		},
		"templates/piper-hooks.json": &fstest.MapFile{
			Data: []byte(`{"hooks":{},"statusLine":{"type":"command","command":"node /usr/local/bin/piper-statusline.mjs"}}`),
		},
	}
}

func TestExtractFragment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "有marker的Dockerfile_正規化去除USER_root和USER_node",
			input:    "FROM node:22-bookworm-slim\nRUN apt-get update\n\n" + languageLayerMarker + "\n\nUSER root\nRUN pip install numpy\nUSER node",
			expected: "RUN pip install numpy",
		},
		{
			name:     "marker後無內容",
			input:    "FROM node:22-bookworm-slim\n\n" + languageLayerMarker + "\n",
			expected: "",
		},
		{
			name:     "保留中間USER_node",
			input:    "FROM node:22-bookworm-slim\n\n" + languageLayerMarker + "\n\nUSER root\nRUN curl ... | tar ...\nUSER node\nRUN go install gopls@latest\nUSER node",
			expected: "RUN curl ... | tar ...\nUSER node\nRUN go install gopls@latest",
		},
		{
			name:     "舊格式Dockerfile提取語言層_正規化（fallback）",
			input:    "FROM piper-devcontainer-base\n\nUSER root\nRUN apt-get update\nUSER node",
			expected: "RUN apt-get update",
		},
		{
			name:     "舊格式只有FROM行（fallback）",
			input:    "FROM piper-devcontainer-base\n",
			expected: "",
		},
		{
			name:     "去除尾部WORKDIR",
			input:    "FROM node:22-bookworm-slim\n\n" + languageLayerMarker + "\n\nARG GO_VERSION=1.26.0\nRUN curl ...\nUSER node\nWORKDIR /workspace",
			expected: "ARG GO_VERSION=1.26.0\nRUN curl ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFragment(tt.input)
			if got != tt.expected {
				t.Errorf("extractFragment() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAssembleDockerfile(t *testing.T) {
	base := "FROM node:22-bookworm-slim\n\nRUN apt-get update"

	t.Run("有fragment_正規化USER並附加footer", func(t *testing.T) {
		got := assembleDockerfile(base, "USER root\nRUN pip install numpy\nUSER node")
		if !strings.HasPrefix(got, "FROM node:22-bookworm-slim") {
			t.Errorf("should start with base content")
		}
		if !strings.Contains(got, languageLayerMarker) {
			t.Errorf("should contain language layer marker")
		}
		if !strings.Contains(got, "pip install numpy") {
			t.Errorf("should contain fragment")
		}
		if !strings.HasSuffix(got, "USER node\nWORKDIR /workspace\n") {
			t.Errorf("should end with USER node + WORKDIR footer, got: %q", got[len(got)-40:])
		}
		markerIdx := strings.Index(got, languageLayerMarker)
		afterMarker := got[markerIdx+len(languageLayerMarker):]
		if strings.Contains(afterMarker, "USER root") {
			t.Errorf("should not contain redundant USER root after marker")
		}
	})

	t.Run("保留中間USER_node", func(t *testing.T) {
		got := assembleDockerfile(base, "RUN curl ... | tar ...\nUSER node\nRUN go install gopls@latest")
		if !strings.Contains(got, "USER node\nRUN go install gopls@latest") {
			t.Errorf("should preserve middle USER node")
		}
	})

	t.Run("空fragment_仍附加footer", func(t *testing.T) {
		got := assembleDockerfile(base, "")
		if !strings.Contains(got, languageLayerMarker) {
			t.Errorf("should contain language layer marker")
		}
		if !strings.HasSuffix(got, "USER node\nWORKDIR /workspace\n") {
			t.Errorf("should end with USER node + WORKDIR footer")
		}
	})
}

func TestNormalizeFragment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "去除開頭USER_root",
			input:    "USER root\nARG GO_VERSION=1.22.0\nRUN curl ...",
			expected: "ARG GO_VERSION=1.22.0\nRUN curl ...",
		},
		{
			name:     "去除尾部USER_node",
			input:    "RUN pip install numpy\nUSER node",
			expected: "RUN pip install numpy",
		},
		{
			name:     "去除尾部USER_node和WORKDIR",
			input:    "RUN curl ...\nUSER node\nWORKDIR /workspace",
			expected: "RUN curl ...",
		},
		{
			name:     "保留中間USER_node",
			input:    "RUN curl ... | tar ...\nUSER node\nRUN go install gopls@latest",
			expected: "RUN curl ... | tar ...\nUSER node\nRUN go install gopls@latest",
		},
		{
			name:     "只有USER_root_回傳空",
			input:    "USER root",
			expected: "",
		},
		{
			name:     "空字串",
			input:    "",
			expected: "",
		},
		{
			name:     "無USER指令_原樣回傳",
			input:    "ARG GO_VERSION=1.22.0\nRUN curl ...\nENV PATH=...",
			expected: "ARG GO_VERSION=1.22.0\nRUN curl ...\nENV PATH=...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFragment(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeFragment() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRenderDevcontainerJSON(t *testing.T) {
	result, err := renderDevcontainerJSON(GenerateConfig{
		ProjectName:    "my-app",
		FirewallMode:   "whitelist",
		AllowedDomains: "api.anthropic.com,github.com",
	}, []string{"golang.go", "ms-python.python"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{
		"my-app DevContainer",
		"golang.go",
		"ms-python.python",
		"anthropic.claude-code",
		"SSH_AUTH_SOCK",
		"GIT_SSH_COMMAND",
		"whitelist",
		"api.anthropic.com,github.com",
		"ssh-agent.sock",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("devcontainer.json should contain %q", check)
		}
	}
}
