package devcontainer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
)

// GenerateConfig holds input parameters for config generation.
type GenerateConfig struct {
	ProjectName    string
	ProjectPath    string
	FirewallMode   string // "open" | "whitelist"
	AllowedDomains string // comma-separated domain list
}

// GeneratedFiles maps relative file paths to their contents.
// e.g. "Dockerfile" -> "FROM node:22-bookworm-slim\n..."
type GeneratedFiles map[string]string

// Generator produces DevContainer configuration files using Claude.
type Generator struct {
	cmd        Commander
	templateFS fs.FS
	ModelFunc  func() string // returns AI model to use; nil → "opus"
}

// NewGenerator creates a new Generator with the given Commander and template FS.
func NewGenerator(cmd Commander, templateFS fs.FS) *Generator {
	return &Generator{cmd: cmd, templateFS: templateFS}
}

// claudeOutput represents the JSON response from claude -p --output-format json.
type claudeOutput struct {
	Type    string `json:"type"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
}

// AIOverlay is the structured output from the AI: a Dockerfile fragment and VS Code extensions.
type AIOverlay struct {
	DockerfileFragment string   `json:"dockerfileFragment"`
	VSCodeExtensions   []string `json:"vscodeExtensions"`
}

// devcontainerJSONData holds the data for rendering devcontainer.json.
type devcontainerJSONData struct {
	ProjectName        string
	EncodedProjectPath string
	FirewallMode       string
	AllowedDomains     string
	VSCodeExtensions   []string
	TemplateVersion    int
}

// EncodeProjectPath encodes a project path for use as a directory name.
// Replaces "/" with "-", matching Claude's own projects/ encoding.
func EncodeProjectPath(projectPath string) string {
	return strings.ReplaceAll(projectPath, "/", "-")
}

// requiredFiles lists the files that must be present in the generated output.
var requiredFiles = []string{"Dockerfile", "devcontainer.json"}

// extraTemplates lists supporting files copied verbatim into .devcontainer/
// alongside the generated Dockerfile and devcontainer.json.
var extraTemplates = []string{
	"init-firewall.sh",
	"git-ssh-helper.sh",
	"piper-hook.mjs",
	"piper-statusline.mjs",
	"piper-hooks.json",
}

// assembleDevcontainerFiles builds the complete set of DevContainer files: the
// Dockerfile (base + language layer fragment), devcontainer.json (with extensions),
// and supporting scripts. Pass an empty fragment + nil extensions for a plain base.
func assembleDevcontainerFiles(templateFS fs.FS, cfg GenerateConfig, fragment string, extensions []string) (GeneratedFiles, error) {
	baseContent, err := fs.ReadFile(templateFS, "templates/base.Dockerfile")
	if err != nil {
		return nil, fmt.Errorf("read base.Dockerfile: %w", err)
	}

	dcJSON, err := renderDevcontainerJSON(cfg, extensions)
	if err != nil {
		return nil, fmt.Errorf("render devcontainer.json: %w", err)
	}

	files := GeneratedFiles{
		"Dockerfile":        assembleDockerfile(string(baseContent), fragment),
		"devcontainer.json": dcJSON,
	}
	for _, name := range extraTemplates {
		data, err := fs.ReadFile(templateFS, "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		files[name] = string(data)
	}
	return files, nil
}

// parseOverlay extracts an AIOverlay from the claude result string.
func parseOverlay(result string) (*AIOverlay, error) {
	var overlay AIOverlay
	if err := json.Unmarshal([]byte(result), &overlay); err != nil {
		// Claude sometimes includes preamble/trailing text around the JSON object.
		if extracted, ok := extractJSON(result); ok {
			if err2 := json.Unmarshal([]byte(extracted), &overlay); err2 == nil {
				return &overlay, nil
			}
		}
		return nil, fmt.Errorf("parse overlay from result: %w", err)
	}
	return &overlay, nil
}

// languageLayerMarker separates the base image content from the AI-generated language layer.
const languageLayerMarker = "# === LANGUAGE LAYER ==="

// assembleDockerfile combines the base Dockerfile content with the AI-generated fragment.
// The base ends in root context; this function normalizes the fragment (stripping redundant
// USER root/USER node/WORKDIR directives) and appends the fixed USER node + WORKDIR footer.
func assembleDockerfile(baseContent, fragment string) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(baseContent, "\n"))
	sb.WriteString("\n\n")
	sb.WriteString(languageLayerMarker)
	sb.WriteString("\n")
	fragment = normalizeFragment(strings.TrimSpace(fragment))
	if fragment != "" {
		sb.WriteString("\n")
		sb.WriteString(fragment)
		sb.WriteString("\n")
	}
	sb.WriteString("\nUSER node\nWORKDIR /workspace\n")
	return sb.String()
}

// normalizeFragment strips USER/WORKDIR directives that the template provides,
// ensuring backward compatibility with fragments that include them.
// Leading "USER root" is removed (base already has root context).
// Trailing "USER node" and "WORKDIR /workspace" are removed (template appends them).
// Middle "USER node" (e.g. before user-space installs) is preserved.
func normalizeFragment(fragment string) string {
	if fragment == "" {
		return ""
	}

	// Strip leading "USER root"
	if strings.HasPrefix(fragment, "USER root\n") {
		fragment = strings.TrimPrefix(fragment, "USER root\n")
		fragment = strings.TrimLeft(fragment, "\n")
	} else if fragment == "USER root" {
		return ""
	}

	// Strip trailing "USER node" and "WORKDIR /workspace"
	lines := strings.Split(fragment, "\n")
	for len(lines) > 0 {
		last := strings.TrimSpace(lines[len(lines)-1])
		if last == "USER node" || last == "WORKDIR /workspace" || last == "" {
			lines = lines[:len(lines)-1]
		} else {
			break
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// devcontainerJSONTemplate is the fixed template for devcontainer.json.
const devcontainerJSONTemplate = `{
  "name": "{{.ProjectName}} DevContainer",

  "build": {
    "dockerfile": "Dockerfile",
    "args": {
      "CLAUDE_CODE_VERSION": "latest"
    }
  },

  "runArgs": [
    "--cap-add=SYS_ADMIN",
    "--cap-add=NET_ADMIN"
  ],

  "initializeCommand": "mkdir -p ~/.claude-devcontainers/{{.EncodedProjectPath}}/sessions ~/.claude-devcontainers/{{.EncodedProjectPath}}/projects && mkdir -p ~/.ssh && rm -f ~/.ssh/ssh-agent.sock && if [ -n \"$SSH_AUTH_SOCK\" ] && [ -S \"$SSH_AUTH_SOCK\" ]; then ln -sf \"$SSH_AUTH_SOCK\" ~/.ssh/ssh-agent.sock; else touch ~/.ssh/ssh-agent.sock; fi",

  "mounts": [
    "source=${localEnv:HOME}/.claude,target=/home/node/.claude,type=bind,consistency=cached",
    "source=${localEnv:HOME}/.claude-devcontainers/{{.EncodedProjectPath}}/sessions,target=/home/node/.claude/sessions,type=bind",
    "source=${localEnv:HOME}/.claude-devcontainers/{{.EncodedProjectPath}}/projects,target=/home/node/.claude/projects,type=bind",
    "source=${localEnv:HOME}/.ssh/ssh-agent.sock,target=/ssh-agent,type=bind"
  ],

  "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind,consistency=delegated",
  "workspaceFolder": "/workspace",

  "containerEnv": {
    "NODE_OPTIONS": "--max-old-space-size=4096",
    "CLAUDE_CONFIG_DIR": "/home/node/.claude",
    "SSH_AUTH_SOCK": "/ssh-agent",
    "GIT_SSH_COMMAND": "/usr/local/bin/git-ssh-helper.sh",
    "FIREWALL_MODE": "{{.FirewallMode}}",
    "ALLOWED_DOMAINS": "{{.AllowedDomains}}"
  },

  "remoteUser": "node",

  "postStartCommand": "ssh-keyscan github.com gitlab.com bitbucket.org >> ~/.ssh/known_hosts 2>/dev/null && sudo /usr/local/bin/init-firewall.sh && echo '{\"hasCompletedOnboarding\":true,\"installMethod\":\"native\"}' > /home/node/.claude/.claude.json && jq '{statusLine}' /usr/local/share/piper-hooks.json > /home/node/.claude/settings.json && mkdir -p /workspace/.claude && jq 'del(.statusLine)' /usr/local/share/piper-hooks.json > /workspace/.claude/settings.local.json",

  "waitFor": "postStartCommand",

  "postAttachCommand": "echo '\\nalias claude=\"claude --dangerously-skip-permissions\"' >> ~/.bashrc && echo '[claude] 全自動模式已啟用：claude 指令預設帶 --dangerously-skip-permissions'",

  "customizations": {
    "vscode": {
      "extensions": [
        "anthropic.claude-code"{{range .VSCodeExtensions}},
        "{{.}}"{{end}}
      ],
      "settings": {
        "terminal.integrated.defaultProfile.linux": "bash"
      }
    }
  },

  "piper": {
    "templateVersion": {{.TemplateVersion}}
  }
}`

// renderDevcontainerJSON renders the fixed devcontainer.json template with project-specific values.
func renderDevcontainerJSON(cfg GenerateConfig, extensions []string) (string, error) {
	tmpl, err := template.New("devcontainer.json").Parse(devcontainerJSONTemplate)
	if err != nil {
		return "", fmt.Errorf("parse devcontainer.json template: %w", err)
	}

	firewallMode := cfg.FirewallMode
	if firewallMode == "" {
		firewallMode = "open"
	}

	data := devcontainerJSONData{
		ProjectName:        cfg.ProjectName,
		EncodedProjectPath: EncodeProjectPath(cfg.ProjectPath),
		FirewallMode:       firewallMode,
		AllowedDomains:     cfg.AllowedDomains,
		VSCodeExtensions:   extensions,
		TemplateVersion:    ConfigTemplateVersion,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute devcontainer.json template: %w", err)
	}

	return buf.String(), nil
}

// extractJSON finds the outermost {...} in s, respecting nested braces and JSON string literals.
func extractJSON(s string) (string, bool) {
	start := strings.Index(s, "{")
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

// extractFragment extracts the language layer fragment from an assembled Dockerfile.
// It returns everything after the language layer marker comment, normalized
// (USER root/USER node/WORKDIR directives stripped).
// Falls back to removing just the FROM line for pre-marker Dockerfiles.
// Contaminated fragments (containing base-image content) are discarded.
func extractFragment(dockerfile string) string {
	raw := extractFragmentRaw(dockerfile)
	if IsContaminatedFragment(raw) {
		return ""
	}
	return normalizeFragment(raw)
}

// extractFragmentRaw extracts the raw fragment without contamination checking.
// Used by Upgrader to get the original fragment for AI extraction.
func extractFragmentRaw(dockerfile string) string {
	if idx := strings.Index(dockerfile, languageLayerMarker); idx >= 0 {
		after := dockerfile[idx+len(languageLayerMarker):]
		return strings.TrimSpace(after)
	}
	// Fallback for legacy Dockerfiles without marker
	lines := strings.Split(dockerfile, "\n")
	var parts []string
	pastFROM := false
	for _, line := range lines {
		if !pastFROM {
			if strings.HasPrefix(strings.TrimSpace(line), "FROM") {
				pastFROM = true
				continue
			}
			continue
		}
		parts = append(parts, line)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// baseMarkers are patterns that only appear in the base image, never in a legitimate language layer.
var baseMarkers = []string{
	"init-firewall.sh",
	"git-ssh-helper.sh",
	"usermod -aG sudo node",
}

// IsContaminatedFragment detects base-image content accidentally captured as a language fragment.
func IsContaminatedFragment(fragment string) bool {
	if fragment == "" {
		return false
	}
	for _, marker := range baseMarkers {
		if strings.Contains(fragment, marker) {
			return true
		}
	}
	return false
}

// extractPromptData holds the data passed to the extract prompt template.
type extractPromptData struct {
	ContaminatedFragment string
	LanguageLayers       string
}

// ExtractLanguageLayer calls claude -p to extract the real language layer
// from a contaminated fragment that mixes base-image content with language-specific content.
func (g *Generator) ExtractLanguageLayer(ctx context.Context, contaminatedFragment string) (*AIOverlay, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prompt, err := g.buildExtractPrompt(contaminatedFragment)
	if err != nil {
		return nil, fmt.Errorf("build extract prompt: %w", err)
	}

	model := "opus"
	if g.ModelFunc != nil {
		if m := g.ModelFunc(); m != "" {
			model = m
		}
	}
	out, err := g.cmd.Output("claude", "-p", "--model", model, "--output-format", "json", prompt)
	if err != nil {
		return nil, fmt.Errorf("run claude extract: %w", err)
	}

	var co claudeOutput
	if err := json.Unmarshal(out, &co); err != nil {
		return nil, fmt.Errorf("parse claude extract output: %w", err)
	}

	if co.IsError {
		return nil, fmt.Errorf("claude returned error: %s", co.Result)
	}

	overlay, err := parseOverlay(co.Result)
	if err != nil {
		return nil, fmt.Errorf("parse extracted overlay: %w", err)
	}

	return overlay, nil
}

// buildExtractPrompt renders the extract prompt template.
func (g *Generator) buildExtractPrompt(contaminatedFragment string) (string, error) {
	tmplContent, err := fs.ReadFile(g.templateFS, "templates/extract-prompt.md.tmpl")
	if err != nil {
		return "", fmt.Errorf("read extract prompt template: %w", err)
	}

	languageLayers, _ := fs.ReadFile(g.templateFS, "templates/base-dockerfile.md")

	tmpl, err := template.New("extract-prompt").Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("parse extract prompt template: %w", err)
	}

	data := extractPromptData{
		ContaminatedFragment: contaminatedFragment,
		LanguageLayers:       string(languageLayers),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute extract prompt template: %w", err)
	}

	return buf.String(), nil
}

