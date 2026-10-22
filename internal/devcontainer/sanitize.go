package devcontainer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Sanitize applies programmatic fixes to generated DevContainer config files.
// It strips JSONC syntax, ensures required fields, and guarantees init-firewall.sh exists.
func Sanitize(files GeneratedFiles, initFirewall string) (GeneratedFiles, error) {
	result := make(GeneratedFiles, len(files))
	for k, v := range files {
		result[k] = v
	}

	// 1. devcontainer.json — JSONC → JSON + required fields
	if dc, ok := result["devcontainer.json"]; ok {
		stripped := stripJSONC(dc)

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(stripped), &parsed); err != nil {
			return nil, fmt.Errorf("sanitize devcontainer.json: invalid JSON after stripping comments: %w", err)
		}

		ensureDevcontainerConfig(parsed)

		out, err := json.MarshalIndent(parsed, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("sanitize devcontainer.json: marshal: %w", err)
		}
		result["devcontainer.json"] = string(out)
	}

	// 2. Dockerfile — basic validation
	if df, ok := result["Dockerfile"]; ok {
		if err := validateDockerfile(df, result); err != nil {
			return nil, err
		}
	}

	// 3. init-firewall.sh — ensure exists
	if content, ok := result["init-firewall.sh"]; !ok || strings.TrimSpace(content) == "" {
		if initFirewall != "" {
			result["init-firewall.sh"] = initFirewall
		}
	}

	return result, nil
}

// stripJSONC removes // line comments and /* */ block comments from JSONC,
// while preserving strings that contain // or /*.
// Also removes trailing commas before } or ].
func stripJSONC(s string) string {
	var buf strings.Builder
	i := 0
	n := len(s)

	for i < n {
		ch := s[i]

		// String literal — copy verbatim
		if ch == '"' {
			buf.WriteByte(ch)
			i++
			for i < n {
				c := s[i]
				buf.WriteByte(c)
				i++
				if c == '\\' && i < n {
					buf.WriteByte(s[i])
					i++
					continue
				}
				if c == '"' {
					break
				}
			}
			continue
		}

		// Line comment
		if ch == '/' && i+1 < n && s[i+1] == '/' {
			// Skip until end of line
			i += 2
			for i < n && s[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment
		if ch == '/' && i+1 < n && s[i+1] == '*' {
			i += 2
			for i+1 < n {
				if s[i] == '*' && s[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			if i >= n {
				// Unterminated block comment — skip rest
			}
			continue
		}

		buf.WriteByte(ch)
		i++
	}

	// Remove trailing commas before } or ]
	result := trailingCommaRe.ReplaceAllString(buf.String(), "$1")
	return result
}

var trailingCommaRe = regexp.MustCompile(`,(\s*[}\]])`)

// ensureDevcontainerConfig checks and fills required fields in the parsed devcontainer.json.
func ensureDevcontainerConfig(parsed map[string]interface{}) {
	// mounts: ensure .claude bind mount + SSH socket
	ensureMount(parsed)
	ensureSSHMount(parsed)

	// containerEnv: CLAUDE_CONFIG_DIR + SSH_AUTH_SOCK + GIT_SSH_COMMAND
	ensureContainerEnv(parsed)
	ensureSSHEnv(parsed)

	// initializeCommand: SSH socket symlink
	ensureInitializeCommand(parsed)

	// postStartCommand: include init-firewall.sh
	ensurePostStartCommand(parsed)

	// runArgs: include --cap-add=NET_ADMIN
	ensureRunArgs(parsed)
}

const claudeMountSource = "${localEnv:HOME}/.claude"

func ensureMount(parsed map[string]interface{}) {
	mounts, _ := parsed["mounts"].([]interface{})
	claudeMount := fmt.Sprintf("type=bind,source=%s,target=/home/node/.claude,consistency=cached", claudeMountSource)

	for _, m := range mounts {
		if s, ok := m.(string); ok && strings.Contains(s, ".claude") {
			return // already has .claude mount
		}
	}

	parsed["mounts"] = append(mounts, claudeMount)
}

func ensureContainerEnv(parsed map[string]interface{}) {
	env, ok := parsed["containerEnv"].(map[string]interface{})
	if !ok {
		env = make(map[string]interface{})
		parsed["containerEnv"] = env
	}
	if _, ok := env["CLAUDE_CONFIG_DIR"]; !ok {
		env["CLAUDE_CONFIG_DIR"] = "/home/node/.claude"
	}
}

func ensurePostStartCommand(parsed map[string]interface{}) {
	cmd, _ := parsed["postStartCommand"].(string)
	if strings.Contains(cmd, "init-firewall.sh") {
		return
	}
	firewall := "sudo /usr/local/bin/init-firewall.sh"
	if cmd == "" {
		parsed["postStartCommand"] = firewall
	} else {
		parsed["postStartCommand"] = firewall + " && " + cmd
	}
}

func ensureRunArgs(parsed map[string]interface{}) {
	args, _ := parsed["runArgs"].([]interface{})
	hasNetAdmin := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "--cap-add=NET_ADMIN" {
			hasNetAdmin = true
			break
		}
	}
	if !hasNetAdmin {
		parsed["runArgs"] = append(args, "--cap-add=NET_ADMIN")
	}
}

func ensureSSHMount(parsed map[string]interface{}) {
	mounts, _ := parsed["mounts"].([]interface{})
	for _, m := range mounts {
		if s, ok := m.(string); ok && strings.Contains(s, "/ssh-agent") {
			return // already has SSH mount
		}
	}
	sshMount := "source=${localEnv:HOME}/.ssh/ssh-agent.sock,target=/ssh-agent,type=bind"
	parsed["mounts"] = append(mounts, sshMount)
}

func ensureSSHEnv(parsed map[string]interface{}) {
	env, ok := parsed["containerEnv"].(map[string]interface{})
	if !ok {
		env = make(map[string]interface{})
		parsed["containerEnv"] = env
	}
	if _, ok := env["SSH_AUTH_SOCK"]; !ok {
		env["SSH_AUTH_SOCK"] = "/ssh-agent"
	}
	if _, ok := env["GIT_SSH_COMMAND"]; !ok {
		env["GIT_SSH_COMMAND"] = "/usr/local/bin/git-ssh-helper.sh"
	}
}

const sshInitializeCommand = `mkdir -p ~/.ssh && if [ -n "$SSH_AUTH_SOCK" ]; then ln -sf "$SSH_AUTH_SOCK" ~/.ssh/ssh-agent.sock; elif [ ! -e ~/.ssh/ssh-agent.sock ]; then touch ~/.ssh/ssh-agent.sock; fi`

func ensureInitializeCommand(parsed map[string]interface{}) {
	cmd, _ := parsed["initializeCommand"].(string)
	if strings.Contains(cmd, "ssh-agent.sock") {
		return
	}
	if cmd == "" {
		parsed["initializeCommand"] = sshInitializeCommand
	} else {
		parsed["initializeCommand"] = sshInitializeCommand + " && " + cmd
	}
}

var dockerfileFromRe = regexp.MustCompile(`(?im)^FROM\s`)

func validateDockerfile(content string, files GeneratedFiles) error {
	if !dockerfileFromRe.MatchString(content) {
		return fmt.Errorf("sanitize Dockerfile: missing FROM instruction")
	}

	if _, hasFirewall := files["init-firewall.sh"]; hasFirewall {
		if !strings.Contains(content, "COPY init-firewall.sh") {
			// Not an error — just a warning condition.
			// The Dockerfile may use a different mechanism.
		}
	}

	return nil
}
