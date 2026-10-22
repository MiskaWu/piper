# UI Utilities

前端 UI 所需的輔助功能：目錄瀏覽、剪貼簿、Git 狀態、老鼠圖鑑。

## 包含的 packages

| Package | 路徑 | 職責 |
|---------|------|------|
| **browse** | `internal/browse/` | 目錄/檔案列表 API，供檔案瀏覽器使用 |
| **clipboard** | `internal/clipboard/` | 圖片/檔案暫存，支援 Ctrl+V 貼圖和拖放上傳（30 分鐘自動清理） |
| **git** | `internal/git/` | Git CLI 封裝：`git status --porcelain` 解析、逐檔狀態（M/A/D/?/R） |
| **gitwatcher** | `internal/gitwatcher/` | 監聽 `.git/refs/` 和 `.git/HEAD` 變化（fsnotify），debounce 後廣播 |
| **mousedex** | `internal/mousedex/` | 程式化產生老鼠頭像（DJB2 hash + mulberry32 PRNG，11³ 種組合） |

## 特性說明

### Git 整合

`git` package 提供靜態狀態查詢，`gitwatcher` 提供即時變更通知：

```
gitwatcher (fsnotify 監聽 .git/)
  │ debounce
  ▼
ws Hub → 前端更新 branch + dirty file count
```

Sidebar 的 branch 名稱、異動檔案數、SVG 分支線圖都依賴這兩個 package。

### 剪貼簿

終端中 Ctrl+V 貼圖或拖放檔案 → 上傳到暫存 → 路徑注入終端供 Claude CLI 讀取。
30 分鐘自動清理，有容量上限。

## 相關 API

```
GET    /api/directories/browse       → 瀏覽目錄
GET    /api/directories/files        → 檔案與目錄（Files Tree）
POST   /api/clipboard/image          → 上傳圖片
GET    /api/clipboard/image/{id}     → 取得圖片
POST   /api/clipboard/file           → 上傳檔案
GET    /api/clipboard/file/{id}      → 取得檔案
GET    /api/git/branches             → 分支列表
GET    /api/git/graph                → commit 線圖
GET    /api/git/file-status          → 逐檔 git status
GET    /api/mousedex                 → 老鼠圖鑑資料
GET    /api/mousedex/stats           → 圖鑑收集摘要
```
