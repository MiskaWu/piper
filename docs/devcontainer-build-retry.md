# DevContainer Build 失敗自動重試

> 當 `devcontainer up` 建置失敗時，將錯誤訊息餵回 Claude 修正配置，最多重試 3 次。

## 背景

目前 `SetupRunner.Run()` 的流程是：

```
Generating → Writing → Building → Done
                         │
                         失敗 → Failed（結束）
```

問題：Claude 產生的 Dockerfile 可能有錯（例如用了 `curl ... | sh` 安裝工具但容器缺少 `make`），`devcontainer up` 失敗後直接結束，使用者只能手動修復。

## 目標流程

```
Generating → Writing → Building ──成功──→ Done
                         │
                         失敗（保留 build error）
                         │
                         ▼
                      Fixing（把 build error 餵回 claude -p 修正配置）
                         │
                         ▼
                      Writing → Building ──成功──→ Done
                                    │
                                    失敗（重試次數 < 3）→ 回到 Fixing
                                    │
                                    失敗（已重試 3 次）→ Failed
```

## 需要變更的檔案

### 1. `internal/devcontainer/setup.go` — 主要修改

#### 新增 Phase

```go
const (
    // 現有
    PhaseGenerating SetupPhase = "generating"
    PhaseWriting    SetupPhase = "writing"
    PhaseBuilding   SetupPhase = "building"
    PhaseDone       SetupPhase = "done"
    PhaseFailed     SetupPhase = "failed"
    // 新增
    PhaseFixing     SetupPhase = "fixing"
)
```

#### 新增常數

```go
const maxBuildRetries = 3
```

#### 修改 `ConfigGenerator` 介面

新增一個修正方法，讓 Generator 可以根據 build error 修正已產生的配置：

```go
type ConfigGenerator interface {
    Generate(ctx context.Context, cfg GenerateConfig) (GeneratedFiles, error)
    Fix(ctx context.Context, cfg GenerateConfig, currentFiles GeneratedFiles, buildError string) (GeneratedFiles, error)
}
```

#### 修改 `Run()` 方法

Building 階段失敗後進入重試迴圈：

```go
func (s *SetupRunner) Run(ctx context.Context, cfg SetupConfig, progress ProgressCallback) (string, error) {
    // ... Generating + Writing（不變）...

    // Building with retry
    for attempt := 0; attempt <= maxBuildRetries; attempt++ {
        if err := ctx.Err(); err != nil {
            progress(SetupStatus{Phase: PhaseFailed, Message: "已取消", Error: err.Error()})
            return "", fmt.Errorf("setup cancelled: %w", err)
        }

        progress(SetupStatus{
            Phase:   PhaseBuilding,
            Message: buildMessage(attempt),
        })

        buildErr := s.runner.EnsureRunning(actualPath)
        if buildErr == nil {
            break // 成功
        }

        if attempt == maxBuildRetries {
            progress(SetupStatus{Phase: PhaseFailed, Message: "建置容器失敗（已重試 3 次）", Error: buildErr.Error()})
            return "", fmt.Errorf("build container after %d retries: %w", maxBuildRetries, buildErr)
        }

        // Fixing: 餵回錯誤讓 Claude 修正
        progress(SetupStatus{
            Phase:   PhaseFixing,
            Message: fmt.Sprintf("建置失敗，正在修正配置（第 %d 次重試）...", attempt+1),
        })

        // 需要先清除失敗的容器，否則下次 devcontainer up 可能撞到殘留狀態
        _ = s.runner.RemoveContainer(actualPath)

        fixedFiles, fixErr := s.generator.Fix(ctx, generatorCfg, files, buildErr.Error())
        if fixErr != nil {
            progress(SetupStatus{Phase: PhaseFailed, Message: "修正配置失敗", Error: fixErr.Error()})
            return "", fmt.Errorf("fix config (attempt %d): %w", attempt+1, fixErr)
        }

        // 重新寫入
        progress(SetupStatus{Phase: PhaseWriting, Message: "正在寫入修正後的配置..."})
        if err := writeFiles(actualPath, fixedFiles); err != nil {
            progress(SetupStatus{Phase: PhaseFailed, Message: "寫入修正檔案失敗", Error: err.Error()})
            return "", fmt.Errorf("write fixed files: %w", err)
        }

        files = fixedFiles
    }

    progress(SetupStatus{Phase: PhaseDone, Message: "設定完成"})
    return actualPath, nil
}
```

#### `ContainerRunner` 介面擴充

需要在重試前清理失敗的容器：

```go
type ContainerRunner interface {
    EnsureRunning(projectPath string) error
    RemoveContainer(projectPath string) error  // 新增
}
```

`Manager` 已經有 `RemoveContainer` 方法，介面對齊即可。

---

### 2. `internal/devcontainer/generator.go` — 新增 `Fix` 方法

```go
// Fix calls claude -p with the original config + current files + build error,
// asking Claude to fix the configuration.
func (g *Generator) Fix(ctx context.Context, cfg GenerateConfig, currentFiles GeneratedFiles, buildError string) (GeneratedFiles, error) {
    prompt, err := g.buildFixPrompt(cfg, currentFiles, buildError)
    if err != nil {
        return nil, fmt.Errorf("build fix prompt: %w", err)
    }

    // 呼叫 claude -p（同 Generate 的 CLI 呼叫邏輯）
    // ... 解析輸出 ...
}
```

#### 新增 Fix prompt 模板 `templates/fix-prompt.md.tmpl`

```markdown
You previously generated DevContainer configuration files for project "{{.ProjectName}}",
but the build failed with the following error:

## Build Error

{{.BuildError}}

## Current Configuration Files

{{range $name, $content := .CurrentFiles}}
### {{$name}}
```
{{$content}}
```
{{end}}

## Instructions

1. Analyze the build error carefully
2. Fix the configuration files to resolve the error
3. Return the COMPLETE fixed files (not just the diff)

## Output Format

Return ONLY a JSON object with all files:

{"Dockerfile": "...", "devcontainer.json": "...", ...}
```

關鍵：prompt 要包含 **完整的現有檔案內容** + **build error 全文**，Claude 才能精準定位問題。

---

### 3. `internal/devcontainer/setup_test.go` — 新增測試

需要新增的測試案例：

| 測試名稱 | 情境 |
|---------|------|
| `TestSetupRunner_Run_建置失敗後自動修正成功` | 第一次 build 失敗，Fix 成功，第二次 build 成功 |
| `TestSetupRunner_Run_建置失敗修正三次後仍失敗` | build 連續失敗 4 次（1 次初始 + 3 次重試），最終 Failed |
| `TestSetupRunner_Run_建置失敗但修正產生器也失敗` | build 失敗，Fix 回傳 error，直接 Failed |
| `TestSetupRunner_Run_重試過程中Context取消` | build 失敗一次，重試迴圈中 context 被取消 |
| `TestSetupRunner_Run_重試時Phase回報順序正確` | 驗證 progress callback 的 phase 順序：Generating → Writing → Building → Fixing → Writing → Building → Done |

#### Mock 調整

`mockGenerator` 需要支援 `Fix` 方法：

```go
type mockGenerator struct {
    files    GeneratedFiles
    err      error
    fixFiles GeneratedFiles  // Fix 回傳的修正檔案
    fixErr   error
}

func (m *mockGenerator) Fix(ctx context.Context, cfg GenerateConfig, currentFiles GeneratedFiles, buildError string) (GeneratedFiles, error) {
    return m.fixFiles, m.fixErr
}
```

`mockRunner` 需要支援「第 N 次成功」的模式：

```go
type mockRunner struct {
    errors    []error  // 依序回傳的 error，nil 表示成功
    callCount int
    removedPaths []string
}

func (m *mockRunner) EnsureRunning(projectPath string) error {
    i := m.callCount
    m.callCount++
    if i < len(m.errors) {
        return m.errors[i]
    }
    return nil
}

func (m *mockRunner) RemoveContainer(projectPath string) error {
    m.removedPaths = append(m.removedPaths, projectPath)
    return nil
}
```

---

### 4. `internal/devcontainer/generator_test.go` — 新增 Fix 測試

| 測試名稱 | 情境 |
|---------|------|
| `TestGenerator_Fix_成功修正配置` | 給定 build error + 現有檔案，Claude 回傳修正版本 |
| `TestGenerator_Fix_prompt包含錯誤訊息和現有檔案` | 驗證 fix prompt 包含 build error 全文和所有現有檔案內容 |

---

### 5. 前端 — `SetupStatus` 的 `phase` 新增 `"fixing"` 值

前端 `SetupProgress.tsx` 需要能顯示新的 fixing 階段。影響範圍小，只需在步驟指示器新增一個狀態。

建議顯示文字：「建置失敗，正在修正配置（第 N 次重試）...」

---

## 設計決策

### 為什麼用 `Fix` 方法而不是重新 `Generate`？

- `Fix` 帶入現有檔案和錯誤訊息，Claude 有完整上下文可以精準修正
- 重新 `Generate` 等於從頭來，可能產生完全不同的配置，之前正確的部分也可能變掉

### 為什麼重試前要 `RemoveContainer`？

`devcontainer up` 失敗可能留下殘留的容器（state = created/exited），不清除的話下次 `devcontainer up` 可能行為不一致。

### 為什麼最多 3 次？

平衡：大部分 build 問題 1-2 次就能修好（缺套件、版本不對），超過 3 次通常是根本性問題（網路、Docker daemon），繼續重試只浪費 API 額度。

### Build error 截取

`EnsureRunning` 回傳的 error 已經包含 `formatBuildError` 的處理（JSON description + stderr 最後 30 行），直接傳給 `Fix` 即可，不需額外截取。
