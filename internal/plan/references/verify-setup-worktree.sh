#!/usr/bin/env bash
# verify-setup-worktree.sh — 為 /verify 在目標專案 repo 中建立隔離 worktree
#
# 用法:
#   verify-setup-worktree.sh <task-id> <target-project-path>
#
# 輸出（stdout，JSON 一行）:
#   {"worktree":"/tmp/piper-verify-<task>","commitHash":"<hash>","baseBranch":"<branch>"}
#
# Exit code:
#   0 = 成功
#   1 = 參數錯誤或 git 操作失敗

set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <task-id> <target-project-path>" >&2
  exit 1
fi

TASK_ID="$1"
TARGET="$2"

if [[ ! -d "$TARGET/.git" ]] && ! git -C "$TARGET" rev-parse --git-dir >/dev/null 2>&1; then
  echo "error: $TARGET is not a git repo" >&2
  exit 1
fi

WT="/tmp/piper-verify-${TASK_ID}"

# 若 worktree 已存在，先嘗試移除（可能是先前失敗遺留）
if [[ -d "$WT" ]]; then
  git -C "$TARGET" worktree remove --force "$WT" >/dev/null 2>&1 || true
  rm -rf "$WT"
fi

# 記錄 base branch（worktree 建立之前取，避免 HEAD 已 detach）
BASE_BRANCH=$(git -C "$TARGET" symbolic-ref --short HEAD 2>/dev/null || echo "")

# 取 HEAD commit hash
COMMIT=$(git -C "$TARGET" rev-parse HEAD)

# 建立 detached worktree（避免污染主分支）
git -C "$TARGET" worktree add --detach "$WT" "$COMMIT" >/dev/null

printf '{"worktree":"%s","commitHash":"%s","baseBranch":"%s"}\n' "$WT" "$COMMIT" "$BASE_BRANCH"
