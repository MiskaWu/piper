#!/usr/bin/env bash
# verify-diff-gate.sh — 檢查 fix subagent 沒有修改到測試檔 / 契約檔
#
# 用法:
#   verify-diff-gate.sh <worktree-path> <pinned-commit>
#
# Exit code:
#   0 = 通過（測試檔 / 契約檔皆未被 fix 階段改動）
#   1 = 命中（fix 階段改動了不該改的檔案，視為作弊）

set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <worktree-path> <pinned-commit>" >&2
  exit 2
fi

WT="$1"
PINNED="$2"

if [[ ! -d "$WT" ]]; then
  echo "error: worktree $WT does not exist" >&2
  exit 2
fi

# 列出 pinned commit 以來工作樹（含未 commit）改動的檔案
changed=$(git -C "$WT" diff --name-only "$PINNED" HEAD; git -C "$WT" diff --name-only HEAD) || true
# 加上未 tracked 的新檔案
untracked=$(git -C "$WT" ls-files --others --exclude-standard)

all_changed=$(printf '%s\n%s\n' "$changed" "$untracked" | awk 'NF' | sort -u)

blocked=()
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  case "$f" in
    *_test.go|*.test.ts|*.test.tsx|*.spec.ts|*.spec.tsx)
      blocked+=("$f") ;;
    contracts/*.yaml|*/contracts/*.yaml)
      blocked+=("$f") ;;
  esac
done <<< "$all_changed"

if [[ ${#blocked[@]} -gt 0 ]]; then
  echo "❌ diff-gate 命中：fix 階段動到以下受保護檔案（視為作弊）"
  for f in "${blocked[@]}"; do
    echo "   $f"
  done
  exit 1
fi

exit 0
