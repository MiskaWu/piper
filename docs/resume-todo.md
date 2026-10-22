# DevContainer Session 隔離 — 待辦事項

## 已完成

- [x] `internal/devcontainer/generator.go` — 新增 `EncodeProjectPath`，模板加 overlay mount + mkdir
- [x] `internal/devcontainer/version.go` — `ConfigTemplateVersion` 6 → 7，upgrade path 傳入 `ProjectPath`
- [x] `internal/claude/filetracker.go` — 新增 `FindClaudeSessionIDForDevcontainer`、`SessionJSONLPathForDevcontainer`
- [x] `watcher_handler.go` — `FileTrackerFunc` 根據 isDevContainer 分流
- [x] `routes_sessions.go` — restore endpoint 根據 isDevContainer 分流
- [x] `routes_projects.go` — DELETE /api/projects 時清除 `~/.claude-devcontainers/{encoded}/`
- [x] `go test ./...` 全部通過、`go build ./...` 通過

## 已完成：清理 + 升級（2026-04-10）

- [x] 手動清除舊污染：`~/.claude/sessions/*`、`~/.claude/session-env/*`
- [x] 升級 piper DevContainer 至 templateVersion 7 + 重建容器，隔離 mount 已生效
- [x] Host 殘留清理：`~/.claude/projects/-workspace` + worktree 殘留已從 host 端清除

## 背景

詳見計畫檔案：完整分析紀錄在對話中，核心問題是 `~/.claude` 被整個 bind mount 到容器，導致 `sessions/` 和 `projects/-workspace/` 被多容器共用污染，`FindClaudeSessionID` 配到錯誤的 session ID。
