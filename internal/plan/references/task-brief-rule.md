
## Task Brief 處理慣例

- Task Brief 位於 `.plan/briefs/` 目錄，由外部 Epic 規劃派送而來
- 每個 brief 對應一個 task（非 story），檔名格式為 `task-{id}.md`
- 閱讀 brief 的「做什麼」和「驗收條件」，理解 task 範圍後開工
- 實作完成後**停在這裡**：回報「實作完成，請手動 `/submit`」並結束，不要自己跑 `/submit` 或 `/verify`
- `/submit`（自測，推到 `in-review`）和 `/verify`（最終驗收，推到 `done` 並搬 brief）由使用者在**新會話**中手動執行
- 不要刪除 brief 檔案，`/verify` 通過後會自動移到 `.plan/briefs/done/`
