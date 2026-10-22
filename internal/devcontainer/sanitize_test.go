package devcontainer

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStripJSONC_移除行註解(t *testing.T) {
	input := `{
  // This is a comment
  "name": "test"
}`
	got := stripJSONC(input)
	if strings.Contains(got, "comment") {
		t.Errorf("expected comment removed, got: %s", got)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}
	if parsed["name"] != "test" {
		t.Errorf("expected name=test, got %v", parsed["name"])
	}
}

func TestStripJSONC_移除區塊註解(t *testing.T) {
	input := `{
  /* block comment */
  "name": "test"
}`
	got := stripJSONC(input)
	if strings.Contains(got, "block") {
		t.Errorf("expected block comment removed, got: %s", got)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}
}

func TestStripJSONC_移除TrailingComma(t *testing.T) {
	input := `{
  "a": 1,
  "b": 2,
}`
	got := stripJSONC(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("result should be valid JSON after trailing comma removal: %v\ninput: %s", err, got)
	}
}

func TestStripJSONC_字串中的斜線不移除(t *testing.T) {
	input := `{
  "url": "https://example.com/path"
}`
	got := stripJSONC(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}
	if parsed["url"] != "https://example.com/path" {
		t.Errorf("URL should be preserved, got: %v", parsed["url"])
	}
}

func TestStripJSONC_陣列TrailingComma(t *testing.T) {
	input := `{
  "items": [
    "a",
    "b",
  ]
}`
	got := stripJSONC(input)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("result should be valid JSON: %v\ngot: %s", err, got)
	}
}

func TestSanitize_補齊缺少的mounts(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	mounts, ok := parsed["mounts"].([]interface{})
	if !ok || len(mounts) == 0 {
		t.Fatal("expected mounts to be added")
	}
	found := false
	for _, m := range mounts {
		if s, ok := m.(string); ok && strings.Contains(s, ".claude") {
			found = true
		}
	}
	if !found {
		t.Error("expected .claude mount to be added")
	}
}

func TestSanitize_補齊缺少的containerEnv(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	env, ok := parsed["containerEnv"].(map[string]interface{})
	if !ok {
		t.Fatal("expected containerEnv to be added")
	}
	if env["CLAUDE_CONFIG_DIR"] != "/home/node/.claude" {
		t.Errorf("expected CLAUDE_CONFIG_DIR, got %v", env["CLAUDE_CONFIG_DIR"])
	}
}

func TestSanitize_補齊缺少的postStartCommand(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	cmd, ok := parsed["postStartCommand"].(string)
	if !ok {
		t.Fatal("expected postStartCommand to be added")
	}
	if !strings.Contains(cmd, "init-firewall.sh") {
		t.Errorf("expected init-firewall.sh in postStartCommand, got: %s", cmd)
	}
}

func TestSanitize_補齊缺少的runArgs(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	args, ok := parsed["runArgs"].([]interface{})
	if !ok || len(args) == 0 {
		t.Fatal("expected runArgs to be added")
	}
	found := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "--cap-add=NET_ADMIN" {
			found = true
		}
	}
	if !found {
		t.Error("expected --cap-add=NET_ADMIN in runArgs")
	}
}

func TestSanitize_已有欄位不覆蓋(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{
			"name": "test",
			"mounts": ["type=bind,source=/home/user/.claude,target=/home/node/.claude"],
			"containerEnv": {"CLAUDE_CONFIG_DIR": "/custom/path"},
			"postStartCommand": "sudo /usr/local/bin/init-firewall.sh && echo done",
			"runArgs": ["--cap-add=NET_ADMIN", "--cap-add=SYS_ADMIN"]
		}`,
		"Dockerfile": "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	// containerEnv should keep custom value
	env := parsed["containerEnv"].(map[string]interface{})
	if env["CLAUDE_CONFIG_DIR"] != "/custom/path" {
		t.Errorf("expected custom CLAUDE_CONFIG_DIR, got %v", env["CLAUDE_CONFIG_DIR"])
	}

	// mounts should have .claude (already present) + SSH (newly added)
	mounts := parsed["mounts"].([]interface{})
	if len(mounts) != 2 {
		t.Errorf("expected 2 mounts (.claude + SSH), got %d: %v", len(mounts), mounts)
	}
}

func TestSanitize_initFirewall補齊(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "#!/bin/bash\necho firewall")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["init-firewall.sh"] != "#!/bin/bash\necho firewall" {
		t.Errorf("expected init-firewall.sh to be filled, got: %q", result["init-firewall.sh"])
	}
}

func TestSanitize_initFirewall已存在不覆蓋(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
		"init-firewall.sh":  "#!/bin/bash\nexisting",
	}
	result, err := Sanitize(files, "#!/bin/bash\nreplacement")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["init-firewall.sh"] != "#!/bin/bash\nexisting" {
		t.Errorf("existing init-firewall.sh should not be overwritten")
	}
}

func TestSanitize_Dockerfile缺FROM回傳錯誤(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "RUN apt-get update",
	}
	_, err := Sanitize(files, "")
	if err == nil {
		t.Fatal("expected error for Dockerfile without FROM")
	}
	if !strings.Contains(err.Error(), "missing FROM") {
		t.Errorf("expected 'missing FROM' error, got: %v", err)
	}
}

func TestSanitize_JSONC完整處理(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{
  // line comment
  "name": "test", /* inline comment */
  "image": "node:20",
}`,
		"Dockerfile": "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result["devcontainer.json"]), &parsed); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}
	if parsed["name"] != "test" {
		t.Errorf("expected name=test, got %v", parsed["name"])
	}
	if parsed["image"] != "node:20" {
		t.Errorf("expected image=node:20, got %v", parsed["image"])
	}
}

func TestSanitize_postStartCommand追加到現有命令前(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test", "postStartCommand": "echo hello"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	cmd := parsed["postStartCommand"].(string)
	if !strings.HasPrefix(cmd, "sudo /usr/local/bin/init-firewall.sh") {
		t.Errorf("expected init-firewall.sh at start, got: %s", cmd)
	}
	if !strings.Contains(cmd, "echo hello") {
		t.Errorf("expected original command preserved, got: %s", cmd)
	}
}

// --- SSH Ensure Tests ---

func TestSanitize_補齊SSH_mount(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	mounts, _ := parsed["mounts"].([]interface{})
	found := false
	for _, m := range mounts {
		if s, ok := m.(string); ok && strings.Contains(s, "/ssh-agent") {
			found = true
		}
	}
	if !found {
		t.Error("expected SSH agent mount to be added")
	}
}

func TestSanitize_已有SSH_mount不重複(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{
			"name": "test",
			"mounts": ["source=${localEnv:HOME}/.ssh/ssh-agent.sock,target=/ssh-agent,type=bind"]
		}`,
		"Dockerfile": "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	mounts := parsed["mounts"].([]interface{})
	sshCount := 0
	for _, m := range mounts {
		if s, ok := m.(string); ok && strings.Contains(s, "/ssh-agent") {
			sshCount++
		}
	}
	if sshCount != 1 {
		t.Errorf("expected 1 SSH mount (no duplicate), got %d", sshCount)
	}
}

func TestSanitize_補齊SSH_AUTH_SOCK和GIT_SSH_COMMAND(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	env := parsed["containerEnv"].(map[string]interface{})
	if env["SSH_AUTH_SOCK"] != "/ssh-agent" {
		t.Errorf("expected SSH_AUTH_SOCK=/ssh-agent, got %v", env["SSH_AUTH_SOCK"])
	}
	if env["GIT_SSH_COMMAND"] != "/usr/local/bin/git-ssh-helper.sh" {
		t.Errorf("expected GIT_SSH_COMMAND, got %v", env["GIT_SSH_COMMAND"])
	}
}

func TestSanitize_已有SSH_env不覆蓋(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{
			"name": "test",
			"containerEnv": {
				"SSH_AUTH_SOCK": "/custom-socket",
				"GIT_SSH_COMMAND": "/custom-ssh"
			}
		}`,
		"Dockerfile": "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	env := parsed["containerEnv"].(map[string]interface{})
	if env["SSH_AUTH_SOCK"] != "/custom-socket" {
		t.Errorf("expected custom SSH_AUTH_SOCK preserved, got %v", env["SSH_AUTH_SOCK"])
	}
	if env["GIT_SSH_COMMAND"] != "/custom-ssh" {
		t.Errorf("expected custom GIT_SSH_COMMAND preserved, got %v", env["GIT_SSH_COMMAND"])
	}
}

func TestSanitize_補齊initializeCommand(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{"name": "test"}`,
		"Dockerfile":        "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	initCmd, ok := parsed["initializeCommand"].(string)
	if !ok {
		t.Fatal("expected initializeCommand to be set")
	}
	if !strings.Contains(initCmd, "ssh-agent.sock") {
		t.Errorf("expected SSH socket setup in initializeCommand, got: %s", initCmd)
	}
}

func TestSanitize_已有initializeCommand不覆蓋(t *testing.T) {
	files := GeneratedFiles{
		"devcontainer.json": `{
			"name": "test",
			"initializeCommand": "mkdir -p ~/.ssh && ln -sf \"$SSH_AUTH_SOCK\" ~/.ssh/ssh-agent.sock"
		}`,
		"Dockerfile": "FROM node:20",
	}
	result, err := Sanitize(files, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal([]byte(result["devcontainer.json"]), &parsed)

	initCmd := parsed["initializeCommand"].(string)
	// Should not duplicate
	if strings.Count(initCmd, "ssh-agent.sock") > 1 {
		t.Errorf("expected no duplicate SSH setup, got: %s", initCmd)
	}
}

