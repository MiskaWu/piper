
## Agent 按 Wave 隔離執行

適用場景：實作 task brief 的 sub-tasks 時（brief 已經過 /breakdown 拆分為 waves）

以 wave 為粒度啟動獨立 agent，不要一個 agent 跑完整個 task 的所有 sub-tasks。

單一 agent 跑完 4-6 個 sub-tasks 後累積 70-100K context，debug 噪音會污染後續 sub-task 品質。per-wave 隔離讓每個 wave 從乾淨 context 開始（20-35K），錯誤修復不擴散到下一 wave。

### 做法

1. 每個 wave 啟動一個獨立 agent（搭配 worktree 隔離時同理）
2. Wave N+1 的 agent prompt 須包含：前一 wave 已建好的檔案路徑清單（不傳程式碼內容）+ 自己這個 wave 的 sub-task 描述
3. 同一 wave 內的多個 sub-tasks 由同一個 agent 依序完成
4. 最後一個 wave（整合驗證）的 agent 負責跑全量測試 + 逐條核對 task 驗收條件 + 對照 story 驗收條件
