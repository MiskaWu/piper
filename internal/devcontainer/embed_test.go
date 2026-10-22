package devcontainer

import (
	"strings"
	"testing"
)

func TestTemplateFS_所有預期檔案可讀取(t *testing.T) {
	expectedFiles := []struct {
		path    string
		contain string // 檔案內容應包含的字串
	}{
		{"templates/base-dockerfile.md", "語言層"},
		{"templates/devcontainer-json.md", "devcontainer.json"},
		{"templates/init-firewall.sh", "#!/bin/bash"},
		{"templates/base.Dockerfile", "FROM node:22-bookworm-slim"},
		{"templates/git-ssh-helper.sh", "ssh"},
	}

	for _, f := range expectedFiles {
		t.Run(f.path, func(t *testing.T) {
			data, err := templateFS.ReadFile(f.path)
			if err != nil {
				t.Fatalf("ReadFile(%s) 失敗: %v", f.path, err)
			}
			if len(data) == 0 {
				t.Fatalf("ReadFile(%s) 回傳空內容", f.path)
			}
			if !strings.Contains(string(data), f.contain) {
				t.Errorf("ReadFile(%s) 內容未包含 %q", f.path, f.contain)
			}
		})
	}
}

func TestInitFirewall_是可執行腳本而非Markdown(t *testing.T) {
	data, err := templateFS.ReadFile("templates/init-firewall.sh")
	if err != nil {
		t.Fatalf("讀取 init-firewall.sh 失敗: %v", err)
	}

	content := string(data)

	if !strings.HasPrefix(content, "#!/bin/bash") {
		t.Error("init-firewall.sh 應以 #!/bin/bash 開頭（不是 markdown）")
	}

	if strings.Contains(content, "```") {
		t.Error("init-firewall.sh 不應包含 markdown 代碼區塊標記")
	}

	if !strings.Contains(content, "setup_firewall") {
		t.Error("init-firewall.sh 應包含 setup_firewall 函數")
	}
	if !strings.Contains(content, "protect_credentials") {
		t.Error("init-firewall.sh 應包含 protect_credentials 函數")
	}
}

func TestGitSSHHelper_是可執行腳本(t *testing.T) {
	data, err := templateFS.ReadFile("templates/git-ssh-helper.sh")
	if err != nil {
		t.Fatalf("讀取 git-ssh-helper.sh 失敗: %v", err)
	}

	content := string(data)

	if !strings.HasPrefix(content, "#!/bin/sh") {
		t.Error("git-ssh-helper.sh 應以 #!/bin/sh 開頭")
	}

	if !strings.Contains(content, "permission denied") {
		t.Error("git-ssh-helper.sh 應包含 SSH 認證失敗偵測")
	}
}

func TestBaseDockerfile_包含基底設定(t *testing.T) {
	data, err := templateFS.ReadFile("templates/base.Dockerfile")
	if err != nil {
		t.Fatalf("讀取 base.Dockerfile 失敗: %v", err)
	}

	content := string(data)

	checks := []string{
		"FROM node:22-bookworm-slim",
		"openssh-client",
		"git-ssh-helper.sh",
		"init-firewall.sh",
		"claude-code",
	}

	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("base.Dockerfile 應包含 %q", check)
		}
	}
}
