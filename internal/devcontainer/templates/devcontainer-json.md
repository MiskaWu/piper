# devcontainer.json 參考模板

devcontainer.json 由 piper 程式碼產生，AI 不需要輸出此檔案。
此文件僅供 fix-prompt 參考用。

```jsonc
{
  "name": "${PROJECT_NAME} DevContainer",

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

  "initializeCommand": "mkdir -p ~/.ssh && if [ -n \"$SSH_AUTH_SOCK\" ]; then ln -sf \"$SSH_AUTH_SOCK\" ~/.ssh/ssh-agent.sock; elif [ ! -e ~/.ssh/ssh-agent.sock ]; then touch ~/.ssh/ssh-agent.sock; fi",

  "mounts": [
    "source=${localEnv:HOME}/.claude,target=/home/node/.claude,type=bind,consistency=cached",
    "source=${localEnv:HOME}/.ssh/ssh-agent.sock,target=/ssh-agent,type=bind"
  ],

  "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind,consistency=delegated",
  "workspaceFolder": "/workspace",

  "containerEnv": {
    "NODE_OPTIONS": "--max-old-space-size=4096",
    "CLAUDE_CONFIG_DIR": "/home/node/.claude",
    "SSH_AUTH_SOCK": "/ssh-agent",
    "GIT_SSH_COMMAND": "/usr/local/bin/git-ssh-helper.sh",
    "FIREWALL_MODE": "open",
    "ALLOWED_DOMAINS": ""
  },

  "remoteUser": "node",

  "postStartCommand": "ssh-keyscan github.com gitlab.com bitbucket.org >> ~/.ssh/known_hosts 2>/dev/null && sudo /usr/local/bin/init-firewall.sh && echo '{\"hasCompletedOnboarding\":true,\"installMethod\":\"native\"}' > /home/node/.claude/.claude.json",

  "waitFor": "postStartCommand",

  "postAttachCommand": "echo '\\nalias claude=\"claude --dangerously-skip-permissions\"' >> ~/.bashrc && echo '[claude] 全自動模式已啟用：claude 指令預設帶 --dangerously-skip-permissions'",

  "customizations": {
    "vscode": {
      "extensions": [
        "anthropic.claude-code"
      ],
      "settings": {
        "terminal.integrated.defaultProfile.linux": "bash"
      }
    }
  }
}
```
