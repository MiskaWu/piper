#!/bin/sh
# Git SSH helper — 攔截認證失敗，提示使用者在 host 端重新解鎖 SSH key
#
# 用法：設定 GIT_SSH_COMMAND 指向本腳本
#   export GIT_SSH_COMMAND=/usr/local/bin/git-ssh-helper.sh

stderr_log=$(mktemp /tmp/git-ssh-stderr.XXXXXX)
trap 'rm -f "$stderr_log"' EXIT

# stdout 直接透傳（保護 git pack protocol），stderr 存檔
ssh "$@" 2>"$stderr_log"
exit_code=$?

# replay stderr 給呼叫端
cat "$stderr_log" >&2

# 認證失敗時附加提示
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
