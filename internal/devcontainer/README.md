# DevContainer Management

開發容器的完整生命週期：前置檢查 → 建立/重建 → 閒置自動停止。

## 包含的 packages

| Package | 路徑 | 職責 |
|---------|------|------|
| **devcontainer** | `internal/devcontainer/` | 容器建立/重建/狀態查詢、`.devcontainer/` 配置生成 |
| **autostop** | `internal/autostop/` | 監控 session 存活，專案所有 session 結束後自動停止容器 |
| **preflight** | `internal/preflight/` | 啟動前檢查必要工具（docker, git, tmux），缺少時給安裝提示 |

## 資料流

```
preflight (啟動檢查)
  │
  ▼
devcontainer Manager
  ├── setup: 組 base Dockerfile + 空語言層 → 寫入 .devcontainer/
  ├── create/rebuild: devcontainer CLI 操作
  └── status: docker ps 查詢容器狀態
  │
  ▼
autostop (背景監控)
  └── 所有 session 結束 → grace period → 停止容器
```

Setup 流程線性走完 writing → validating → building → verifying → done，不再呼叫 AI 生成語言層；使用者進入容器後再依需求 `apt/npm/pip install` 或編輯 Dockerfile 的 LANGUAGE LAYER 區段。

## 架構

採用 **本地 base image + 空語言層** 模式：
- Base image：預裝共用工具（SSH、Claude Code 等）
- 語言層：建立時為空，使用者依需求增量加裝（live install + 視需要持久化到 Dockerfile）
- Upgrader：升級 base 模板版本時，AI 仍可協助從舊污染 fragment 提取乾淨的語言層（`Generator.ExtractLanguageLayer`）

## 相關 API

```
GET    /api/projects                       → 專案列表 + 容器狀態
POST   /api/projects                       → 註冊專案
POST   /api/projects/{name}/rebuild        → 重建容器
POST   /api/projects/setup                 → DevContainer 設定流程
GET    /api/projects/setup/{id}/status     → 設定進度
GET    /api/projects/{name}/logs           → 容器日誌
```
