# Story 拆分原則

Story 拆分沒有唯一正解，不同人會有不同切法。判斷標準不是「切得對不對」，而是：

1. **後續工作能否達成完整使用者體驗**
2. **是否方便避免漏東西**

只要符合這兩條，拆分方式可以有不同風格。

## 拆分框架（從粗到細）

1. **使用者體驗閉環優先**：以玩家/使用者的完整操作路徑為單位切 story，避免只切 UI 或只切 BE 的水平切法（vertical slice）
2. **INVEST 自查**：Independent / Negotiable / Valuable / Estimable / Small / Testable
3. **Scope Check 四問**（見 plan-claude.md 的 Scope Check 段落）
4. **與既有 story 對照扣除重疊**：新 story 的功能若已被 done/in-progress 的 story 吸收，從 description 移除並註明依賴關係

## 漏洞檢查清單（Review 時必走）

對每組相關 story，逐一檢查以下 6 點，結果分級處理：

### 🔴 真漏洞 — 必須補進 story 的 AC

**AC 是契約**。Integration 類 story（彙整、跨 story 傳資料、事件發布）的 acceptance 必須明確定義「對外介面」，否則下游各自實作會走樣。

檢查重點：

1. **彙整 / 聚合類 story 的對外資料結構**
   - 誰讀、讀什麼欄位，必須在 AC 列出
   - 範例：「彙整結果以統一資料結構對外暴露（清單供 A 讀 / 數值供 B 讀 / 選項供 C 讀）」

2. **事件 / 旗標類 story 的讀寫介面**
   - 寫入方（如成就完成）與讀取方（如技能解鎖）的介面要統一
   - 範例：「提供統一旗標讀取介面（GetFlag(id) → bool），被解鎖方一律透過此介面查詢」

### 🟡 次要 — 補一行 AC 或 description 註記

3. **UI 狀態的生命週期**
   - NEW 標籤、已讀狀態、通知等：誰標記、何時清除、是 BE 還是 FE 狀態
   - 範例：「新項目首次出現時標記 NEW，進入詳情後清除」

4. **規格未定的清單 / 情境**
   - description 明確註記「待 specs 定案，至少包含 X + Y」，避免實作時兩邊落差

### 🟢 Task 層處理即可 — 不必污染 story

5. **與既有資料模型的相容性**（如注入新目標要相容舊 objective schema）
6. **UI 入口的統一性**（多個使用場景是否共用一個入口）

## Independence 與 stub 策略

Story 宣稱 Independent ≠ 無任何依賴。對於下游尚在 in-progress 的 story：

- 寫清楚對外介面契約（即 🔴 第 1、2 點）
- 下游尚未完成時，用 stub / mock 推進本 story 的實作與測試
- 不要因為「要等下游」而把本 story 的 scope 綁進下游

## 避免過度細化

Story Map 不是規格文件。目的是抓主流程與 scope 邊界：

- 主使用者路徑先抓，細節用 AC 補足
- 實作細節（變數命名、API 路由、表結構）不寫進 story
- 🟢 層級的事項留給 task 拆分階段

## 與 plan-claude.md 規範的關係

本規則補充 plan-claude.md 的「Scope Check 四問」與「INVEST 原則」，專注於**每個 story 內部的 AC 完整性**，而非 story 之間的關係。

使用順序：
1. 先用 Scope Check 判斷「這個 story 該不該存在」
2. 再用本規則檢查「這個 story 的 AC 是否完整」
