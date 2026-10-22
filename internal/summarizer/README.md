# AI & Usage

AI 整合與用量追蹤：自動摘要、API 配額監控、每日報告、Claude CLI 整合。

## 包含的 packages

| Package | 路徑 | 職責 |
|---------|------|------|
| **summarizer** | `internal/summarizer/` | `claude -p --model haiku` 自動產生 session 任務摘要 |
| **usage** | `internal/usage/` | Claude API 配額輪詢（5h/7d window）、工作時間追蹤 |
| **digest** | `internal/digest/` | 每日工作摘要彙總，支援 Discord webhook 發送 + 排程 |
| **claude** | `internal/claude/` | Claude CLI 整合：讀取 history.jsonl、追蹤 session 編輯檔案數 |

## 資料流

```
claude (讀 ~/.claude/history.jsonl)
  │
  ├──→ summarizer (比對 pane hash，有變化時呼叫 claude -p 產摘要)
  │
  ├──→ usage (輪詢 Anthropic OAuth API，10 分鐘快取)
  │
  └──→ digest (彙總當日 session 活動 + commit → Discord webhook)
```

## 外部依賴

| Package | 外部呼叫 |
|---------|---------|
| summarizer | `claude` CLI（`-p --model haiku`） |
| usage | Anthropic API（`api.anthropic.com/api/oauth/usage`） |
| digest | Discord webhook（POST） |
| claude | 讀檔（`~/.claude/history.jsonl`） |

## 相關 API

```
GET    /api/dashboard-stats    → 月度用量統計（token + 工作時間 + 花費）
GET    /api/summary-stats      → 摘要統計
GET    /api/digest             → 每日工作摘要（?date=YYYY-MM-DD）
POST   /api/digest/send        → 手動發送摘要到 Discord
GET    /api/settings/models    → AI 模型設定
PUT    /api/settings/models    → 更新模型設定
GET    /api/settings/digest    → 摘要排程設定
PUT    /api/settings/digest    → 更新摘要設定
```
