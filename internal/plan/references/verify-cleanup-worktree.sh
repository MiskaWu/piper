#!/usr/bin/env bash
# verify-cleanup-worktree.sh — 清理 /verify 建立的 worktree
#
# 用法:
#   verify-cleanup-worktree.sh <task-id> <target-project-path>
#   verify-cleanup-worktree.sh --all                         # 清理所有 /tmp/piper-verify-*
#
# Exit code:
#   0 = 成功（或找不到也算成功）

set -euo pipefail

if [[ $# -eq 1 && "$1" == "--all" ]]; then
  for wt in /tmp/piper-verify-*; do
    [[ -d "$wt" ]] || continue
    # 嘗試從 worktree 本身的 git 設定推回主 repo
    main_repo=$(git -C "$wt" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || echo "")
    if [[ -n "$main_repo" ]]; then
      repo_root=$(dirname "$main_repo")
      git -C "$repo_root" worktree remove --force "$wt" >/dev/null 2>&1 || true
    fi
    rm -rf "$wt"
  done
  exit 0
fi

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <task-id> <target-project-path> | $0 --all" >&2
  exit 1
fi

TASK_ID="$1"
TARGET="$2"
WT="/tmp/piper-verify-${TASK_ID}"

if [[ ! -d "$WT" ]]; then
  exit 0
fi

git -C "$TARGET" worktree remove --force "$WT" >/dev/null 2>&1 || true
rm -rf "$WT"
