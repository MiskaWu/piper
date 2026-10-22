# 遷移指南：為現有 DevContainer 專案加入 Git SSH Helper

## 背景

當 host 端 SSH key passphrase cache 過期，容器內 git 操作會失敗，
但錯誤訊息不明確。此 helper 會在認證失敗時附加提示，引導使用者回 host 執行 `ssh-add`。

## 找出需要更新的專案

在 host 上執行，找出所有使用 SSH agent forwarding 的 DevContainer 專案：

```bash
grep -rl "SSH_AUTH_SOCK" ~/projects/*/.devcontainer/devcontainer.json 2>/dev/null
```

> 把 `~/projects` 換成你放專案的根目錄。

## 每個專案需要的三步驟

### 1. 複製 `git-ssh-helper.sh`

```bash
cp /path/to/piper/.devcontainer/git-ssh-helper.sh <TARGET_PROJECT>/.devcontainer/
```

或直接建立檔案，內容如下：

```sh
#!/bin/sh
# Git SSH helper — 攔截認證失敗，提示使用者在 host 端重新解鎖 SSH key

stderr_log=$(mktemp /tmp/git-ssh-stderr.XXXXXX)
trap 'rm -f "$stderr_log"' EXIT

ssh "$@" 2>"$stderr_log"
exit_code=$?

cat "$stderr_log" >&2

if [ $exit_code -ne 0 ] && grep -qi 'permission denied\|publickey' "$stderr_log" 2>/dev/null; then
  echo "" >&2
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
  echo "  SSH 認證失敗 — 可能是 host 端的 SSH key passphrase 已過期" >&2
  echo "" >&2
  echo "  請在 host 終端機執行：" >&2
  echo "    ssh-add ~/.ssh/<your-key>" >&2
  echo "" >&2
  echo "  然後回到容器內重試即可（不需重啟容器）" >&2
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
fi

exit $exit_code
```

### 2. Dockerfile 加入 COPY

在 `USER` 指令之前加入：

```dockerfile
# Git SSH helper（攔截認證失敗並提示使用者）
COPY git-ssh-helper.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/git-ssh-helper.sh
```

### 3. devcontainer.json 加入環境變數

在 `containerEnv` 區塊加入：

```json
"GIT_SSH_COMMAND": "/usr/local/bin/git-ssh-helper.sh"
```

### 4. 重建容器

```bash
# VS Code: Ctrl+Shift+P → "Dev Containers: Rebuild Container"
# 或 CLI:
devcontainer up --workspace-folder <TARGET_PROJECT>
```

## 快速批次腳本

如果多個專案結構相同，可用此腳本一次搞定：

```bash
#!/bin/bash
# 用法: ./apply-ssh-helper.sh ~/projects/foo ~/projects/bar

HELPER_SRC="$(dirname "$0")/../.devcontainer/git-ssh-helper.sh"

for project in "$@"; do
  dc_dir="$project/.devcontainer"
  if [ ! -f "$dc_dir/devcontainer.json" ]; then
    echo "[skip] $project — 沒有 .devcontainer"
    continue
  fi

  # 1. 複製 helper
  cp "$HELPER_SRC" "$dc_dir/git-ssh-helper.sh"
  echo "[done] $project — 已複製 git-ssh-helper.sh"

  # 2 & 3 需要手動編輯 Dockerfile 和 devcontainer.json
  #   因為每個專案的 Dockerfile 結構不同，不適合自動 sed
  echo "  → 請手動編輯 Dockerfile 和 devcontainer.json（見上方步驟 2、3）"
done
```

## 驗證

容器重建後，在容器內測試：

```bash
# 確認 helper 存在
which git-ssh-helper.sh

# 確認環境變數
echo $GIT_SSH_COMMAND

# 模擬認證失敗（連一個不存在的 host）
GIT_SSH_COMMAND=/usr/local/bin/git-ssh-helper.sh ssh -o ConnectTimeout=3 git@localhost 2>&1 || true
```
