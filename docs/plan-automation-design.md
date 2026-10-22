# PLAN 流程自動化與品質提升設計

> 2026-04-16 討論結論
>
> **歷史記錄** (2026-04-22 重構後)：本文件保留當時設計脈絡。Skill 命名已調整：
> 文中的 `/verify`（跑測試 + fix loop）已改名為 `/submit`；文中的 `/done`
> （事後驗收）已改名為 `/verify`。metrics 欄位名（passed_first_try、
> done_sampling_failed 等）為相容性保留不動。實際實作請參考 `CLAUDE.md`。

## 背景

兩個目標：
1. 減少人工確認步驟，讓流程更自動化
2. 提升需求拆分品質，讓 Claude 執行得更好

## #4 Always / Ask / Never 框架

### 決定

- **層級**：Story 層，管業務邊界（技術規範由目標專案 `.claude/rules/` 管）
- **誰寫**：Claude 在 plan session 建議 + 互動式補充
- **Brief 呈現**：獨立 `## 行動邊界` section（與驗收條件分開）

### Story Map 結構變更

```yaml
stories:
  - id: s1
    name: 上傳功能
    status: todo
    acceptance:
      - 支援 PNG/JPG
    always:
      - 新 handler 必須有 table-driven 測試
    ask:
      - 要新增外部 dependency 時
    never:
      - 不改現有 API 回傳格式
    tasks: [...]
```

### 設計原理

- Always：Claude 自動執行，確保品質底線
- Ask：影響範圍大但不確定對錯，保留人類決定權
- Never：防護欄，避免破壞性變更
- 驗收條件是「做完打勾」，A/A/N 是「持續遵守」，性質不同所以分開

## #5 Breakdown 自動驗證規則

### 結構規則（程式化檢查）

| # | 規則 | 狀態 |
|---|------|------|
| 1 | 同 wave 內的 sub-tasks 不得修改同一檔案 | 已有 |
| 2 | 每個 sub-task 影響 ≤ 3 個檔案 | 已有 |
| 3 | Wave 1 為 contract/interface 定義（不需要時可標記跳過） | 新增 |
| 4 | 最後實作 wave 跑全量測試，不另開獨立整合 wave | 修改版 |
| 5 | 每個 sub-task 都要有測試描述 | 新增 |
| 6 | 沒有循環依賴（wave N 只依賴 wave < N） | 新增 |

### 語意規則（Claude 自檢）

| # | 規則 | 因果 |
|---|------|------|
| 7 | 每個 sub-task 可獨立理解 | agent 不讀其他 sub-task，描述必須自足 |
| 8 | 所有 task acceptance criteria 都被至少一個 sub-task 覆蓋 | 防止拆完後有驗收條件沒人負責 |
| 9 | 新增 interface 時，某個 wave 必須負責 DI wiring | 防止只有骨架沒有連接 |

### 自動放行流程

```
breakdown 完成
  → 結構規則 1-6（程式化）
    → 全過 + 語意規則 7-9 自檢通過 → 自動放行
    → 失敗 → 自動重試（最多 2 次）→ 仍失敗 → 升級給人
```

### 粒度決策

- 預設 per wave（一個 agent 執行整個 wave 的所有 sub-tasks）
- 不做 per sub-task，靠 #8 retro 數據（sub-task index vs 通過率）判斷是否需要升級

### 所有規則可由 retro 數據更新

## #6 Spec 變更 → Story Map 影響對照

### 影響判斷方式

Claude 語意比對：把 spec changelog + 所有 stories 一起給 Claude 判斷受影響的 story ids + 原因。
不用關鍵字匹配（spec 用詞和 story 命名不一定一致）。

### 依 status 分流

| Story Status | 處理方式 |
|-------------|---------|
| todo | 直接修改原 story |
| in-progress | 不動原 story，開新 follow-up story |
| done | 不動原 story，開新 follow-up story |
| cut | 忽略 |

### Follow-up Story 結構

```yaml
- id: s12
  name: 上傳格式擴充（follow-up s3）
  description: |
    原因：spec v2 新增 HEIC 格式支援
    變更來源：specs/changelog.md 2026-04-16
  status: todo
  release: r2          # 可以跟原 story 不同 release
  links:
    - type: follows
      target: s3
  acceptance:
    - ...
```

### Story Map 結構變更

Story 新增 `links` 欄位：

```yaml
links:
  - type: follows      # 關係類型
    target: s3         # 目標 story id
```

## #8 品質回饋迴圈 / Retro 指標

### 觸發時機

Harvest 完成後自動產出 retro 報告。

### 指標清單（16 項）

#### 執行品質（7）

1. **一次通過率**：`/done` 第一次就過的比例
2. **Gap 數量**：per task
3. **Gap 類型分布**：missing_test, boundary_case, integration, acceptance_uncovered, scope_violation, ...
4. **Sub-task index vs 通過率**：偵測 context 污染（越後面越差 = 該升級粒度）
5. **Agent 超範圍修改**：碰了 sub-task 未列出的檔案
6. **Agent 提問次數**：Ask 觸發次數（越多 = brief 越不清楚）
7. **Agent 自我修正次數**：做了又改回來

#### 流程效率（4）

8. **Dispatch → Done 時間**
9. **Breakdown 重試次數**
10. **Rework rate**：`/done` 失敗後重做的次數
11. **規則 3 跳過比例**：Wave 1 contract 跳過是否被濫用

#### Spec 品質（3）

12. **Story 有 A/A/N 的比例**
13. **Acceptance criteria 平均數**
14. **Follow-up story 佔比**

#### 元指標（2）

15. **未分類 gap 數量 + 內容**：gap 歸不進已知類型 → 標記 uncategorized + 自由文字描述
16. **指標異常沉默**：指標全綠但結果不好 → retro 時主觀 + 客觀對比

### 資料收集

`/done` 驗收時，每個 sub-task 記一筆：

```yaml
task: task-a1
wave: 2
subtask_index: 3
subtask_total: 3
passed_first_try: false
gap_count: 2
gap_types: [missing_test, boundary_case]
```

### 存放結構

```
epic-xxx/
  retro/
    release-r1.md       # retro 報告（永久保留）
    release-r2.md
    actions.yaml         # 改善行動追蹤
```

### actions.yaml

```yaml
actions:
  - id: a1
    from_retro: release-r1
    description: missing_test 加進 Always 規則
    status: todo         # todo → done
    applied_at: null
```

### Retro 報告包含

1. 總覽（stories/tasks 完成數）
2. 執行品質指標
3. 流程效率指標
4. Spec 品質指標
5. 上次改善行動追蹤（狀態 + 效果驗證）
6. 指標盲區檢查（未分類 gap + 異常沉默）
7. 建議行動

### 跨 release 趨勢

每個指標跟前一個 release 比較，顯示趨勢方向。

### 改善迴圈

```
retro 報告 → 建議行動 → 人確認 → 寫入 actions.yaml
  → 下次 retro → 檢查行動狀態 → 驗證效果 → 新的建議
```

## 未來項目（暫不實作）

- #1 Dispatch auto-mode：先手動
- #2 Harvest auto-mode：先手動
- #3 Done 自修迴圈：另有會話討論
- #7 Pipeline 全流程編排：待其他項目穩定後再做
