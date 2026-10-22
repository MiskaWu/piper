#!/usr/bin/env bash
# check-briefs.sh — 檢查 brief 檔案中是否有未勾選的驗收條件
#
# 用法:
#   check-briefs.sh [task-id...]        # 檢查指定 tasks
#   check-briefs.sh                      # 掃描 .plan/briefs/ 下所有 task-*.md
#
# Exit code:
#   0 = 全部通過（無未勾選 checkbox）
#   1 = 有未勾選的 checkbox（輸出詳細清單）
#   2 = 找不到任何 brief 檔案

set -euo pipefail

BRIEFS_DIR=".plan/briefs"
has_failure=0
file_count=0

# 收集要檢查的檔案
files=()
if [[ $# -gt 0 ]]; then
  for id in "$@"; do
    # 支援帶或不帶 .md 後綴、帶或不帶 task- 前綴
    candidate="$id"
    [[ "$candidate" != task-* ]] && candidate="task-$candidate"
    [[ "$candidate" != *.md ]] && candidate="$candidate.md"
    path="$BRIEFS_DIR/$candidate"
    if [[ -f "$path" ]]; then
      files+=("$path")
    else
      echo "⚠️  找不到 brief: $path"
    fi
  done
else
  # 掃描所有未完成的 brief（排除 done/）
  while IFS= read -r -d '' f; do
    files+=("$f")
  done < <(find "$BRIEFS_DIR" -maxdepth 1 -name 'task-*.md' -print0 2>/dev/null | sort -z)
fi

if [[ ${#files[@]} -eq 0 ]]; then
  echo "❌ 找不到任何 brief 檔案"
  exit 2
fi

for f in "${files[@]}"; do
  task_id=$(basename "$f" .md)
  unchecked=()
  missing_evidence=()

  # 掃描「## 驗收條件」區段：
  #   - 未勾選 [ ] → unchecked
  #   - 已勾選 [x] 但接下來 5 行內沒有 `**evidence:**` → missing_evidence
  # 每個 checkbox 視為一個「block」到下一個 checkbox 或段落結束為止。
  while IFS=$'\t' read -r kind line_num content; do
    case "$kind" in
      UNCHECKED)
        unchecked+=("${line_num}:${content}") ;;
      NO_EVIDENCE)
        missing_evidence+=("${line_num}:${content}") ;;
    esac
  done < <(awk '
    /^## 驗收條件/ { in_section = 1; next }
    in_section && /^## / { exit }
    {
      if (in_section) {
        if (match($0, /^- \[ \]/)) {
          print "UNCHECKED\t" NR "\t" $0
          # reset pending evidence tracking
          pending_line = 0
          next
        }
        if (match($0, /^- \[[xX]\]/)) {
          # finalize previous pending if any
          if (pending_line > 0) {
            print "NO_EVIDENCE\t" pending_line "\t" pending_content
          }
          pending_line = NR
          pending_content = $0
          has_evidence = 0
          next
        }
        if (pending_line > 0) {
          if (match($0, /\*\*evidence:\*\*/)) {
            has_evidence = 1
            # once evidence is found, close this pending block
            pending_line = 0
            next
          }
          # blank line or next checkbox closes the block
          if ($0 ~ /^[[:space:]]*$/) {
            if (!has_evidence) {
              print "NO_EVIDENCE\t" pending_line "\t" pending_content
            }
            pending_line = 0
          }
        }
      }
    }
    END {
      if (in_section && pending_line > 0 && !has_evidence) {
        print "NO_EVIDENCE\t" pending_line "\t" pending_content
      }
    }
  ' "$f" 2>/dev/null || true)

  file_count=$((file_count + 1))

  total_issues=$(( ${#unchecked[@]} + ${#missing_evidence[@]} ))

  if [[ $total_issues -gt 0 ]]; then
    has_failure=1
    echo "❌ $task_id 有 ${total_issues} 個驗收問題："
    for item in "${unchecked[@]}"; do
      line_num="${item%%:*}"
      content="${item#*:}"
      content="${content#"${content%%[![:space:]]*}"}"
      echo "   [未勾選] L${line_num}: $content"
    done
    for item in "${missing_evidence[@]}"; do
      line_num="${item%%:*}"
      content="${item#*:}"
      content="${content#"${content%%[![:space:]]*}"}"
      echo "   [缺 evidence] L${line_num}: $content"
    done
    echo ""
  else
    echo "✅ $task_id — 所有 checkbox 已勾選且附 evidence"
  fi
done

echo "---"
echo "檢查完成: $file_count 個 briefs"

if [[ $has_failure -eq 1 ]]; then
  echo "結果: ❌ 有未勾選項目，無法標記完成"
  exit 1
else
  echo "結果: ✅ 全部通過"
  exit 0
fi
