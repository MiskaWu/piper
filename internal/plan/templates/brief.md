# Task Brief: {{.TaskID}} — {{.TaskName}}

## 上下文

| 欄位 | 值 |
|---|---|
| Epic | {{.EpicName}} |
| Story | {{.StoryID}} — {{.StoryName}} |
| Activity | {{.ActivityName}} |
| Step | {{.StepName}} |
| Release | {{.ReleaseLabel}} |
| Tags | {{.Tags}} |
| Target Project | {{.TargetProjectDisplay}} |

## 做什麼

{{.Description}}

## 相關決策

{{.ADRContent}}

## 技術探針結論

{{.SpikeContent}}

## 同 Story 相鄰 Tasks

| Task ID | 名稱 | Target Project | 狀態 |
|---|---|---|---|
{{- range .SiblingTasks}}
| {{.ID}} | {{.Name}} | {{.TargetProject}} | {{.Status}} |
{{- end}}
{{if not .SiblingTasks}}| （無） | — | — | — |
{{- end}}

## Story 驗收條件

{{- range .Acceptance}}
- {{.}}
{{- end}}
{{if not .Acceptance}}
（無）
{{- end}}

## 行動邊界
{{- if or .Always .Ask .Never}}

### Always（必須遵守）
{{- range .Always}}
- {{.}}
{{- end}}
{{- if not .Always}}
（無）
{{- end}}

### Ask（需要確認）
{{- range .Ask}}
- {{.}}
{{- end}}
{{- if not .Ask}}
（無）
{{- end}}

### Never（禁止）
{{- range .Never}}
- {{.}}
{{- end}}
{{- if not .Never}}
（無）
{{- end}}
{{- else}}

（本 Story 未定義行動邊界）
{{- end}}

## 驗收條件

{{- if .TaskAcceptance}}
{{- range .TaskAcceptance}}
- [ ] {{.}}
{{- end}}
{{- else}}
- [ ] （由 /breakdown 補充）
{{- end}}
