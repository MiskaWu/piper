# Retro Session — 容器化環境

你正在容器化環境中運行，所有操作必須透過 Piper API。
**不可使用** Read/Write/Edit 工具存取 vault 或專案檔案（容器內沒有掛載）。

## 自動啟動

請**立即**開始 Retro 工作流。
Epic ID: `{{EPIC_SLUG}}`

## Retro 的作用

一次動作同時做兩件事，冪等可重跑：

1. **關閉 story**：task 全 done 但 story 還 in-progress / in-review 的 → 推到 done
2. **啟動 retro 報告**：release 內所有 story 都 done 時，呼叫報告產生器

如果某 release 已經有**有效報告**（> 200 字），預設 skip；需要重跑可帶 `forceReleases`。

## Piper API 參考

Base URL: `http://localhost:8622`

### 專案列表
```bash
curl -s http://localhost:8622/api/projects
```

### Story Map
```bash
curl -s http://localhost:8622/api/epics/{{EPIC_SLUG}}/story-map
```

### Retro 預覽
```bash
# 掃描：(1) task 全 done 但 story 還沒關的 story  (2) 各 release 的 retro 狀態
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{}' \
  http://localhost:8622/api/epics/{{EPIC_SLUG}}/retro/preview
```

回傳格式：
```json
{
  "epicName": "...",
  "closableStories": [
    {"storyId":"story-xxx", "storyName":"...", "releaseId":"rel-mvp", "taskCount": 3}
  ],
  "retroActions": [
    {"releaseId":"rel-mvp", "releaseLabel":"MVP", "action":"generate", "reason":"no valid retro report yet"},
    {"releaseId":"rel-v2", "releaseLabel":"v2",  "action":"skip",     "reason":"valid retro report already exists"}
  ]
}
```

### 執行 Retro
```bash
# 關 story + 對各 release 執行 retro 動作
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"epicId":"{{EPIC_SLUG}}"}' \
  http://localhost:8622/api/epics/{{EPIC_SLUG}}/retro/execute
```

強制重跑某幾個 release 的 retro 報告（即使已有有效報告）：
```bash
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"epicId":"{{EPIC_SLUG}}", "forceReleases":["rel-mvp"]}' \
  http://localhost:8622/api/epics/{{EPIC_SLUG}}/retro/execute
```

回傳格式：
```json
{
  "closedStories": [{"storyId":"story-xxx", "storyName":"...", "releaseId":"rel-mvp", "taskCount":3}],
  "retros": [
    {"releaseId":"rel-mvp", "releaseLabel":"MVP", "status":"generated",   "reportName":"release-mvp"},
    {"releaseId":"rel-v2", "releaseLabel":"v2",  "status":"skipped"}
  ]
}
```

`status` 可能是 `generated` / `regenerated` / `skipped` / `failed`。

## 工作流程

### 步驟

1. **呼叫 Retro 預覽** API 取得會發生什麼
2. **格式化預覽結果**展示給使用者：
   ```
   📋 Retro 預覽 — {{EPIC_SLUG}}

   ✅ 將關閉 2 個 story：
     story-aaa11 — 使用者登入（3 個 task 全 done）
     story-bbb22 — 資料管理（2 個 task 全 done）

   📝 Retro 動作：
     rel-mvp（MVP）         → 產生新報告
     rel-v2（v2）           → skip（已有有效報告）

   確認要執行嗎？
   ```
3. **使用者確認**後呼叫**執行 Retro** API（必要時附上 `forceReleases`）
4. **展示結果摘要**：關了幾個 story、產了哪些 retro 報告

### 顯示慣例

- Release 一律使用 `meta.releases` 中的 `label`
- 使用繁體中文

### 注意事項

- Task status 不由 Retro 改動（由 dispatch agent 的 `/verify` skill 負責）
- Retro 只改 story 狀態 + 寫報告檔
- 重跑 retro 是安全的：已有有效報告的 release 預設 skip，不會覆寫
- 有效報告定義：檔案存在且內容 ≥ 200 字（抓「空報告 / 破碎渲染」的 signal）
