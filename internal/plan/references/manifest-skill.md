---
description: "掃描本專案 codebase 結構訊號，產生 .plan/manifest.yaml — 供 /design 決定契約類型與產物路徑"
argument-hint: "不帶參數（首次掃描）或 rescan（覆蓋既有 manifest）"
---

# Project Manifest 掃描器

產出 `.plan/manifest.yaml` — 本專案在 piper 規劃體系裡的**基礎設施宣告**：
transport 類型（rest / proto / mixed）、契約產物命名模板、語言與建置指令。

`/design` 會讀這份 manifest 決定要寫哪種類型的契約、`generates[].path` 填什麼；
`/submit` 會讀它判斷用哪個建置 / 測試指令跑 L3 驗證。

> **契約 vs manifest**：契約是**邏輯共識**（跨專案的語意協議），manifest 是**本專案的基礎設施事實**（用什麼 stack、檔案放哪）。契約不該鏡像 codebase，但產契約時需要 manifest 當 context。

## 參數

- 不帶參數：首次掃描。若 `.plan/manifest.yaml` 已存在 → 停止並提示使用者用 `/manifest rescan`
- `rescan`：覆蓋既有 manifest。會先讀舊版當 base（保留使用者手動調整過的欄位）

## 啟動步驟

1. 檢查 `.plan/manifest.yaml` 是否存在
2. 依參數分流（首次 / rescan）
3. 進入掃描流程

## 掃描流程

### 1. 偵測語言

依序檢查（第一個命中即採用）：

| 訊號檔 | language |
|---|---|
| `go.mod` | go |
| `package.json`（看 `dependencies`） | node |
| `pom.xml` / `build.gradle` | java |
| `Cargo.toml` | rust |
| `pyproject.toml` / `setup.py` / `requirements.txt` | python |

找不到任何訊號 → 標記 `language: unknown`，在 `notes` 說明，由使用者補。

### 2. 偵測 transport

**絕對不做**實作層級的解析（不解 AST、不讀 handler 內容），只看結構性訊號：

| 訊號 | transport 候選 |
|---|---|
| 存在任何 `*.proto` 檔 | proto |
| 存在 `routes/`、`handlers/`、`router.go`、`routes.go`、或 `*_routes.*` 檔 | rest |
| `package.json` 含 `express` / `fastify` / `koa` | rest |
| Go 檔案內含 `mux.NewRouter` / `gin.New` / `chi.NewRouter`（用 grep 粗篩即可） | rest |

判定規則：
- 只命中一種 → 直接採用
- 兩種都命中 → `transport.kind: mixed`，在 `notes` 列出證據，請使用者決定 primary
- 都沒命中 → `transport.kind: unknown`，由使用者補

### 3. 偵測契約產物模板

掃 existing test / types 檔案，推契約 generator 產物該放哪：

**test_pattern**（REST 契約的 `go-httptest` 或同語言對應物）
- Go：找 `*_contract_test.go` 或 `routes*_test.go`，取最常見路徑模板
- Node：找 `*.contract.test.ts` / `*.contract.spec.ts`
- 找不到現成範例 → 填建議預設值，並在 `notes` 標「未驗證，請確認」

**types_pattern**（前端 TS 型別）
- 找 `ui/src/types/contracts/` 或 `src/types/contracts/` → 用之
- 找 `ui/src/types/` / `src/types/` → 建議 `{parent}/contracts/{id}.ts`
- 找不到 `ui/` → 省略（此 project 可能非全端）

**proto_root**（transport=proto 時必填）
- 第一個含 `.proto` 的目錄 → 取相對 repo root 的路徑
- 多個候選目錄 → 取最淺層的

### 4. 偵測建置與測試指令

依 language 給預設：

| language | test_command | build_command |
|---|---|---|
| go | `go test ./...` | `go build ./...` |
| node（有 `test` script） | `npm test` | `npm run build`（若有 build script） |
| java | `mvn test` / `gradle test` | `mvn package` / `gradle build` |
| python | `pytest` | （通常省略） |

若 `Makefile` 存在 → grep `test:` / `build:` target，優先採用 `make test` / `make build`。

### 5. 草擬 manifest

先寫到 `/tmp/piper-manifest-draft.yaml`（不直接寫 `.plan/manifest.yaml`）：

```yaml
version: 1
project:
  name: {dir basename}
  language: go
  framework: ""                          # 如偵測到特定框架 (gin, express) 填入

transport:
  kind: rest                             # rest | proto | mixed | unknown
  # kind=proto 或 mixed 時才有以下欄位：
  proto_root: proto/
  rpc_framework: ""                      # rpcx | grpc | twirp | ""（使用者補）

contracts:
  test_pattern: "routes_{resource}_contract_test.go"
  types_pattern: "ui/src/types/contracts/{id}.ts"
  # kind=proto 時新增：
  # rpc_test_pattern: "internal/rpc/{service}_contract_test.go"

conventions:
  test_command: "go test ./..."
  build_command: "go build ./..."

detected_at: {YYYY-MM-DD}
notes: |
  {逐項列出偵測依據，格式：「欄位: 看到 X，推斷 Y」}
  {不確定的欄位標「未驗證，請確認」}
```

## A-gate（寫入前確認）

把草稿**完整貼到對話**，逐欄位說明判定依據：

```
掃描結果：

✓ language: go                   （看到 go.mod，module name = piper）
✓ transport.kind: rest           （看到 routes.go、routes_*.go，無 .proto）
? transport.kind: mixed          （同時看到 routes.go 與 proto/sessions.proto，請選一個 primary）
✓ test_pattern: routes_{resource}_contract_test.go
                                 （看到 routes_sessions_contract_test.go 當範例）
? types_pattern: ui/src/types/contracts/{id}.ts
                                 （未找到現成 contracts 目錄，這是建議值）
✓ test_command: go test ./...
✓ build_command: go build ./...

請確認（yes 寫入 / 修 X=Y / 撤回）
```

- `yes` → 寫入 `.plan/manifest.yaml`
- `修 transport.kind=proto` → 修 YAML 後重貼 A-gate
- `撤回` → 停止，不寫檔

**必須互動**，不自動寫入——manifest 錯一次會汙染所有後續契約的 `generates[].path`。

## 寫入 `.plan/manifest.yaml`

1. 確保 `.plan/` 目錄存在（`mkdir -p`）
2. 寫入 YAML
3. 提示使用者：
   - manifest 已寫入 `.plan/manifest.yaml`
   - 建議 commit 進 git（codebase fact，該進版控）
   - 後續 `/design` 會自動讀取

## Rescan 分支（已有 manifest）

1. 讀 `.plan/manifest.yaml` 當 base
2. 重跑掃描流程
3. **對比**新舊：
   - 使用者手動加過的欄位（如 `rpc_framework: rpcx`）**保留**
   - 結構性欄位（`transport.kind`、`test_pattern`）若偵測結果跟舊版不同 → 在 A-gate 逐項標紅，說明變更原因
4. A-gate 同首次流程

## 注意事項

- 使用繁體中文回報
- **只看結構訊號**，不解析實作細節
- 所有「不確定」欄位都標在 `notes` 下，A-gate 用 `?` 標記讓使用者確認
- Manifest 不包含 `owners` / `changelog` 這類 governance 欄位——它是事實宣告，不是協議
- 寫入後不自動 commit，由使用者決定
