---
description: "補 message 類型契約的實作綁定（transport service.method + proto message 型別名）到本專案 .plan/bindings/{id}.yaml"
argument-hint: "{contract-id}（單個補綁）或不帶參數（掃出所有需要綁的 message 契約）"
---

# Message 契約 Binding 補充器

**為什麼需要這步**：Message 契約（`type: message`）在 `/design` 階段只定語意（resource / action / 欄位 / errors），
不寫 `service.method` 與 message 名——那些是**本專案的實作綁定**，不該由 epic 端的 /design 猜。
契約保持 upstream 單一 writer 的 governance，binding 落在 project-local `.plan/bindings/{id}.yaml`。

`/submit` 會合併契約（upstream）+ binding（local）才能生成可編譯的 RPCX 測試骨架。
binding 缺失或過期時，`/submit` 會中斷並提示你跑 `/bind`。

## 參數

- 不帶參數：掃出所有 message 契約，列出缺 binding 或 contract_version 對不上的，逐一處理
- `{contract-id}`：只處理單一契約

## 前置條件

1. Piper 運行中，`$PIPER_HOST` 或 `127.0.0.1:8622` 可達
2. 本專案已跑過 `/manifest` 並寫入 `.plan/manifest.yaml`（manifest 決定這 project 的 transport）
3. 契約已由上游 `/design` 產出並有 `type: message`

## 啟動步驟

### 1. 讀 manifest 確認本專案走 proto

```bash
curl -sfL "http://$PIPER_HOST/api/projects/$(basename $PWD)/manifest"
```

取 `transport.kind`：
- `proto` 或 `mixed` → 繼續
- `rest` → **停下**，提示「本專案 transport 非 proto，不需要 binding」
- 404 → **停下**，提示先跑 `/manifest`

### 2. 反查相關契約

找出**綁到本專案 task 的 message 契約**——從 `.plan/briefs/` 下的任一 brief 取 epic 名：

```bash
EPIC=$(grep -h '^| Epic |' .plan/briefs/*.md | head -1 | sed 's/.*| Epic | \(.*\) |.*/\1/')
curl -sfL "http://$PIPER_HOST/api/epics/$EPIC/contracts" | jq -r '.[] | select(.type=="message") | .id'
```

得到 message 契約 ID 清單。

### 3. 比對現有 binding

```bash
PROJECT=$(basename $PWD)
curl -sfL "http://$PIPER_HOST/api/bindings/$PROJECT" | jq -r '.contractIds[]'
```

讀單一 binding：

```bash
curl -sfL "http://$PIPER_HOST/api/bindings/$PROJECT/{contract-id}"
```

分三類：
- 契約存在 + binding 存在 + contract_version 對齊 → skip
- 契約存在 + binding 缺 → **需補**
- 契約存在 + binding 存在但 contract_version 落後 → **需重綁**（契約 major bump 過）

## 逐契約補綁流程

對每個需補的契約：

### 1. 讀契約

```bash
curl -sfL "http://$PIPER_HOST/api/epics/$EPIC/contracts/{contract-id}"
```

從 YAML 取：
- `id` / `version`
- `message.resource` / `message.action`（業務語意）
- `message.request` / `message.response`（欄位語意，不是 proto message name）
- `message.errors`（error code 清單）

### 2. 搜本專案 .proto 檔找對應 service

```bash
# manifest 裡有 proto_root
PROTO_ROOT=$(curl -sfL "http://$PIPER_HOST/api/projects/$(basename $PWD)/manifest" | grep 'proto_root:' | awk '{print $2}')
grep -rn "service " "$PROTO_ROOT" --include='*.proto'
```

依 resource / action 語意比對，找最可能的 service.method。範例：
- 契約 `resource: session, action: list` → 候選 `SessionService.ListSessions` 或 `SessionService.List`
- 契約 `resource: item, action: get` → 候選 `ItemService.GetItem` 或 `ItemService.Get`

**找不到對應 service.method** → 列出該 service 的所有 RPC，問使用者選或要求新增一個。
**找到多個候選** → 列出來讓使用者挑。

### 3. 確認 proto message 型別名

對選定的 method，讀該 `.proto` 檔取 request / response 的 proto message 型別名：

```proto
service SessionService {
  rpc List(ListSessionsRequest) returns (ListSessionsResponse);
}
```

→ binding 填 `request: ListSessionsRequest`、`response: ListSessionsResponse`（proto message 型別）

### 4. 草擬 binding YAML

存到 `/tmp/piper-bind-{id}.yaml`，完整貼到對話：

```yaml
contract_id: {contract-id}
contract_version: "{契約當前 version}"
bound_at: {today-YYYY-MM-DD}
proto:
  service: {選定的 service}
  method: {選定的 method}
  file: {相對 repo root 的 .proto 路徑，如 proto/sessions.proto}
  request: {Request message 名}
  response: {Response message 名}
notes: |
  （選填，記錄命名判斷依據或 TODO）
```

### 5. A-gate 確認

向使用者說明：
- 契約語意（resource / action）→ 選定的 service.method
- Request / Response message 名
- proto file 路徑

問：**「確認寫入嗎？（yes / 修 X=Y / 撤回）」**

- `yes` → 進步驟 6
- `修 X=Y` → 修正後重貼
- `撤回` → 跳過這個契約

### 6. 寫入 binding

```bash
curl -sfL -X PUT "http://$PIPER_HOST/api/bindings/$PROJECT/{contract-id}" \
  -H 'Content-Type: text/yaml' \
  --data-binary @/tmp/piper-bind-{contract-id}.yaml
```

- 2xx → 寫入成功，檔案落在 `.plan/bindings/{contract-id}.yaml`
- 400 → 顯示錯誤（通常是 validation 失敗），回步驟 4 修正

### 7. 繼續下一個契約，或結束

全部處理完後提示：
- 已綁定的契約清單
- `.plan/bindings/` 建議進 git（binding 是本專案對契約的事實綁定）
- 下一步：跑 `/submit {task-id}` 會用新 binding 生成測試

## 重綁分支（contract_version 落後）

步驟 1-3 同上，**額外注意**：
- 契約 major bump 可能改了 request/response 欄位語意 → service.method 可能要換、message 名可能要換
- 重綁就是**覆蓋舊 binding 檔**，PUT 會直接覆蓋，不需要先刪

## 注意事項

- 使用繁體中文回報
- binding 檔**一定**要 A-gate，錯一次 /submit 會產錯測試
- `service` / `method` / `file` / `request` / `response` 任一欄位空都會被 API validator 擋（4xx）
- 本 skill 不動任何 .proto 檔——只讀取對應 service
- 若發現對應 .proto 根本還沒寫（proto-first 專案還沒建） → 停下，提示先補 .proto 檔再跑 /bind
