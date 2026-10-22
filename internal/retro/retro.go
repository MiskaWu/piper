package retro

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/miskawu/piper/internal/plan"
)

// StoryMapIO reads and writes story map YAML.
type StoryMapIO interface {
	ReadStoryMap(epicID string) (string, error)
}

// ProjectResolver maps a project name to its filesystem path.
type ProjectResolver interface {
	ResolveProject(name string) (path string, ok bool)
}

// Verdict constants for PlanningCheck.
const (
	VerdictSufficient       = "sufficient"
	VerdictNeedsMoreContext = "needs_more_context"
)

// DecisionType values accepted in decisions.jsonl. Kept in sync with
// plan-claude.md's decision log section.
const (
	DecisionTypeTags           = "tags"
	DecisionTypeTaskSplit      = "task_split"
	DecisionTypeAAN            = "aan"
	DecisionTypeSpecsProofread = "specs_proofread"
)

// SubTaskMetric corresponds to one line in metrics.jsonl.
type SubTaskMetric struct {
	Task           string   `json:"task"`
	Timestamp      string   `json:"timestamp"`
	PassedFirstTry bool     `json:"passed_first_try"`
	GapCount       int      `json:"gap_count"`
	GapTypes       []string `json:"gap_types"`
	GapDetails     []string `json:"gap_details,omitempty"`
	ReworkCount    int      `json:"rework_count"`

	VerifyLoopCount      int  `json:"verify_loop_count,omitempty"`
	VerifyPassedFirstTry bool `json:"verify_passed_first_try,omitempty"`
	VerifyExhausted      bool `json:"verify_exhausted,omitempty"`
	DiffGateBlocked      bool `json:"diff_gate_blocked,omitempty"`
	DoneSamplingFailed   bool `json:"done_sampling_failed,omitempty"`
}

// ContractMetric describes one changelog entry in a contract YAML.
type ContractMetric struct {
	Contract   string `json:"contract"`
	Version    string `json:"version"`
	ChangeType string `json:"change_type"` // additive / breaking / none
	Reason     string `json:"reason,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Date       string `json:"date"`
}

// VerifyStats summarizes verify metrics for the retro report.
type VerifyStats struct {
	TotalVerifiedTasks   int     `json:"totalVerifiedTasks"`
	VerifyPassedFirstTry int     `json:"verifyPassedFirstTry"`
	VerifyPassFirstRate  float64 `json:"verifyPassFirstRate"`
	VerifyExhausted      int     `json:"verifyExhausted"`
	DiffGateBlocked      int     `json:"diffGateBlocked"`
	DoneSamplingFailed   int     `json:"doneSamplingFailed"`
	AvgLoopCount         float64 `json:"avgLoopCount"`
}

// ContractStats summarizes contract change metrics for the retro report.
type ContractStats struct {
	Total    int              `json:"total"`
	Breaking int              `json:"breaking"`
	Additive int              `json:"additive"`
	None     int              `json:"none"`
	Entries  []ContractMetric `json:"entries,omitempty"`
}

// Action represents an improvement action from a retro.
type Action struct {
	ID          string `yaml:"id"`
	FromRetro   string `yaml:"from_retro"`
	Description string `yaml:"description"`
	Status      string `yaml:"status"` // todo, done
	AppliedAt   string `yaml:"applied_at,omitempty"`
}

// ActionsFile is the top-level structure of actions.yaml.
type ActionsFile struct {
	Actions []Action `yaml:"actions"`
}

// PlanningCheck analyzes whether gaps indicate insufficient planning context.
type PlanningCheck struct {
	AcceptanceUncoveredRate float64 `json:"acceptanceUncoveredRate"`
	ScopeViolationRate      float64 `json:"scopeViolationRate"`
	IntegrationGapRate      float64 `json:"integrationGapRate"`
	StoriesWithoutAAN       int     `json:"storiesWithoutAan"`
	Verdict                 string  `json:"verdict"`
}

// ReportData holds all computed metrics for the retro report template.
type ReportData struct {
	EpicName          string            `json:"epicName"`
	ReleaseLabel      string            `json:"releaseLabel"`
	Stories           plan.StatusCounts `json:"stories"`
	Tasks             plan.StatusCounts `json:"tasks"`
	FirstPassRate     float64           `json:"firstPassRate"`
	FirstPassCount    int               `json:"firstPassCount"`
	TotalTasks        int               `json:"totalTasks"`
	GapTotal          int               `json:"gapTotal"`
	GapDistribution   map[string]int    `json:"gapDistribution,omitempty"`
	AvgReworkCount    float64           `json:"avgReworkCount"`
	UncategorizedGaps []string          `json:"uncategorizedGaps,omitempty"`
	Planning          PlanningCheck     `json:"planning"`
	AANPassRate       float64           `json:"aanPassRate"`
	NonAANPassRate    float64           `json:"nonAanPassRate"`
	AANTaskCount      int               `json:"aanTaskCount"`
	NonAANTaskCount   int               `json:"nonAanTaskCount"`
	Verify            VerifyStats       `json:"verify"`
	Contracts         ContractStats     `json:"contracts"`
	DecisionReviews   []DecisionReview  `json:"decisionReviews,omitempty"`
	// DecisionsLoaded distinguishes "loader not wired" from "loader wired but log empty"
	// so the template can show different hints for each state.
	DecisionsLoaded bool `json:"decisionsLoaded"`
}

// ContractLoader reads contract YAML files for an epic.
// Implementations typically delegate to vault.VaultManager.
type ContractLoader interface {
	ListContracts(epicID string) ([]string, error)
	ReadContract(epicID, id string) (string, error)
}

// DecisionLoader reads the raw decisions.jsonl for an epic.
type DecisionLoader interface {
	ReadRetroDecisions(epicID string) (string, error)
}

// DecisionEntry represents one parsed line from decisions.jsonl.
type DecisionEntry struct {
	Timestamp    string   `json:"ts"`
	Type         string   `json:"type"`
	Target       string   `json:"target"`
	Context      []string `json:"context,omitempty"`
	Rationale    string   `json:"rationale,omitempty"`
	Alternatives string   `json:"alternatives,omitempty"`
	Output       string   `json:"output,omitempty"`
}

// DecisionReview is one row of the retro 決策回顧 section — a task that
// has gaps this release, cross-referenced with decisions whose target is
// the task itself or its parent story.
type DecisionReview struct {
	Task      string          `json:"task"`
	StoryID   string          `json:"storyId"`
	GapCount  int             `json:"gapCount"`
	GapTypes  []string        `json:"gapTypes,omitempty"`
	Decisions []DecisionEntry `json:"decisions,omitempty"`
}

// Service handles retro metrics collection and report generation.
type Service struct {
	io        StoryMapIO
	resolver  ProjectResolver
	executor  ExecutorStore  // optional — required for Preview/Execute; nil means read-only
	contracts ContractLoader // optional — nil means contract stats are skipped
	decisions DecisionLoader // optional — nil means decision review is skipped
}

// New creates a new Retro Service.
func New(io StoryMapIO, resolver ProjectResolver) *Service {
	return &Service{io: io, resolver: resolver}
}

// WithContractLoader attaches a ContractLoader so retro reports include
// contract-change statistics. If not set, contract sections stay empty.
func (s *Service) WithContractLoader(loader ContractLoader) *Service {
	s.contracts = loader
	return s
}

// WithDecisionLoader attaches a DecisionLoader so retro reports include
// a 決策回顧 section. If not set, the section is omitted.
func (s *Service) WithDecisionLoader(loader DecisionLoader) *Service {
	s.decisions = loader
	return s
}

// parseJSONL reads JSON Lines from r, skipping blank lines and malformed
// entries (with a stderr warning tagged by label). Used for both
// metrics.jsonl and decisions.jsonl.
func parseJSONL[T any](r io.Reader, label string) []T {
	var out []T
	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s line %d: %v\n", label, lineNum, err)
			continue
		}
		out = append(out, v)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s scanner error: %v\n", label, err)
	}
	return out
}

// CollectMetrics reads metrics.jsonl from all target projects for an epic.
// Malformed lines are skipped with warnings logged to stderr.
func (s *Service) CollectMetrics(epicID string) ([]SubTaskMetric, error) {
	raw, err := s.io.ReadStoryMap(epicID)
	if err != nil {
		return nil, fmt.Errorf("collect metrics read story map %s: %w", epicID, err)
	}

	sm, err := plan.ParseStoryMap([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("collect metrics parse story map %s: %w", epicID, err)
	}

	return s.collectMetricsFromMap(sm)
}

// collectMetricsFromMap reads metrics.jsonl from all target projects referenced by the story map.
func (s *Service) collectMetricsFromMap(sm *plan.StoryMap) ([]SubTaskMetric, error) {
	projects := sm.TargetProjects()

	var metrics []SubTaskMetric
	for proj := range projects {
		projPath, ok := s.resolver.ResolveProject(proj)
		if !ok {
			continue
		}

		m, err := readMetricsFile(filepath.Join(projPath, ".plan", "metrics.jsonl"))
		if err != nil {
			continue // no metrics file = no data, not an error
		}
		metrics = append(metrics, m...)
	}

	return metrics, nil
}

// readMetricsFile reads and parses a single metrics.jsonl file.
func readMetricsFile(path string) ([]SubTaskMetric, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseJSONL[SubTaskMetric](f, path), nil
}

// collectContractMetrics walks an epic's contracts/ directory and collects one
// entry per changelog row. Returns empty slice if no ContractLoader is set.
func (s *Service) collectContractMetrics(epicID string) ([]ContractMetric, error) {
	if s.contracts == nil {
		return nil, nil
	}
	ids, err := s.contracts.ListContracts(epicID)
	if err != nil {
		return nil, fmt.Errorf("list contracts %s: %w", epicID, err)
	}
	var out []ContractMetric
	for _, id := range ids {
		raw, err := s.contracts.ReadContract(epicID, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: contract %s/%s: %v\n", epicID, id, err)
			continue
		}
		var shell struct {
			Changelog []struct {
				Version string `yaml:"version"`
				Date    string `yaml:"date"`
				Type    string `yaml:"type"`
				Summary string `yaml:"summary"`
				Reason  string `yaml:"reason"`
			} `yaml:"changelog"`
		}
		if err := yaml.Unmarshal([]byte(raw), &shell); err != nil {
			fmt.Fprintf(os.Stderr, "warning: parse contract %s/%s: %v\n", epicID, id, err)
			continue
		}
		for _, entry := range shell.Changelog {
			out = append(out, ContractMetric{
				Contract:   id,
				Version:    entry.Version,
				ChangeType: entry.Type,
				Reason:     entry.Reason,
				Summary:    entry.Summary,
				Date:       entry.Date,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Contract < out[j].Contract
	})
	return out, nil
}

// GenerateReport produces a markdown retro report for a release. Convenience
// wrapper that chains BuildReportData and RenderReport; most call sites can
// use this. If the caller needs the structured data (e.g. to persist a JSON
// snapshot alongside the markdown), call BuildReportData + RenderReport
// separately.
func (s *Service) GenerateReport(epicID, releaseLabel string) (string, error) {
	data, err := s.BuildReportData(epicID, releaseLabel)
	if err != nil {
		return "", err
	}
	return RenderReport(data)
}

// BuildReportData aggregates all the retro metrics for a release into a
// ReportData struct. Pure computation — no I/O side effects except reads.
// Used both by GenerateReport (rendering) and by runRetroAction (persisting
// structured snapshot before rendering).
func (s *Service) BuildReportData(epicID, releaseLabel string) (*ReportData, error) {
	raw, err := s.io.ReadStoryMap(epicID)
	if err != nil {
		return nil, fmt.Errorf("retro report read story map %s: %w", epicID, err)
	}

	sm, err := plan.ParseStoryMap([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("retro report parse story map %s: %w", epicID, err)
	}

	metrics, err := s.collectMetricsFromMap(sm)
	if err != nil {
		return nil, fmt.Errorf("retro report collect metrics: %w", err)
	}

	stats := sm.Stats()

	// Build task-to-story mapping for A/A/N analysis
	taskStoryMap := make(map[string]string) // task ID → story ID
	storyHasAAN := make(map[string]bool)    // story ID → has A/A/N
	for _, act := range sm.Activities {
		for _, step := range act.Steps {
			for _, story := range step.Stories {
				hasAAN := len(story.Always) > 0 || len(story.Ask) > 0 || len(story.Never) > 0
				storyHasAAN[story.ID] = hasAAN
				for _, task := range story.Tasks {
					taskStoryMap[task.ID] = story.ID
				}
			}
		}
	}

	// Compute metrics
	data := ReportData{
		EpicName:        sm.Meta.Name,
		ReleaseLabel:    releaseLabel,
		Stories:         stats.Stories,
		Tasks:           stats.Tasks,
		GapDistribution: make(map[string]int),
	}

	totalRework := 0
	firstPassCount := 0
	aanFirstPass := 0
	aanTotal := 0
	nonAANFirstPass := 0
	nonAANTotal := 0

	verifyTotal := 0
	verifyLoopSum := 0
	verifyPassFirst := 0
	verifyExhausted := 0
	diffGateBlocked := 0
	doneSamplingFailed := 0

	for _, m := range metrics {
		data.TotalTasks++
		if m.PassedFirstTry {
			firstPassCount++
		}
		data.GapTotal += m.GapCount
		totalRework += m.ReworkCount
		for _, gt := range m.GapTypes {
			data.GapDistribution[gt]++
		}
		for _, detail := range m.GapDetails {
			if detail != "" {
				data.UncategorizedGaps = append(data.UncategorizedGaps, detail)
			}
		}

		// Verify aggregates (a task counts as "verified" if any verify field is set).
		verified := m.VerifyLoopCount > 0 || m.VerifyPassedFirstTry || m.VerifyExhausted || m.DiffGateBlocked
		if verified {
			verifyTotal++
			verifyLoopSum += m.VerifyLoopCount
			if m.VerifyPassedFirstTry {
				verifyPassFirst++
			}
			if m.VerifyExhausted {
				verifyExhausted++
			}
			if m.DiffGateBlocked {
				diffGateBlocked++
			}
		}
		if m.DoneSamplingFailed {
			doneSamplingFailed++
		}

		// A/A/N effectiveness
		storyID := taskStoryMap[m.Task]
		if storyHasAAN[storyID] {
			aanTotal++
			if m.PassedFirstTry {
				aanFirstPass++
			}
		} else {
			nonAANTotal++
			if m.PassedFirstTry {
				nonAANFirstPass++
			}
		}
	}

	data.Verify = VerifyStats{
		TotalVerifiedTasks:   verifyTotal,
		VerifyPassedFirstTry: verifyPassFirst,
		VerifyExhausted:      verifyExhausted,
		DiffGateBlocked:      diffGateBlocked,
		DoneSamplingFailed:   doneSamplingFailed,
	}
	if verifyTotal > 0 {
		data.Verify.VerifyPassFirstRate = float64(verifyPassFirst) / float64(verifyTotal) * 100
		data.Verify.AvgLoopCount = float64(verifyLoopSum) / float64(verifyTotal)
	}

	contractEntries, err := s.collectContractMetrics(epicID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: collect contract metrics for %s: %v\n", epicID, err)
	}
	data.Contracts.Entries = contractEntries
	data.Contracts.Total = len(contractEntries)
	for _, c := range contractEntries {
		switch c.ChangeType {
		case "breaking":
			data.Contracts.Breaking++
		case "additive":
			data.Contracts.Additive++
		case "none":
			data.Contracts.None++
		}
	}

	data.FirstPassCount = firstPassCount
	if data.TotalTasks > 0 {
		data.FirstPassRate = float64(firstPassCount) / float64(data.TotalTasks) * 100
		data.AvgReworkCount = float64(totalRework) / float64(data.TotalTasks)
	}

	if aanTotal > 0 {
		data.AANPassRate = float64(aanFirstPass) / float64(aanTotal) * 100
	}
	data.AANTaskCount = aanTotal
	if nonAANTotal > 0 {
		data.NonAANPassRate = float64(nonAANFirstPass) / float64(nonAANTotal) * 100
	}
	data.NonAANTaskCount = nonAANTotal

	// Count stories without A/A/N
	storiesWithoutAAN := 0
	for _, hasAAN := range storyHasAAN {
		if !hasAAN {
			storiesWithoutAAN++
		}
	}

	// Planning quality diagnosis
	planning := PlanningCheck{
		StoriesWithoutAAN: storiesWithoutAAN,
		Verdict:           VerdictSufficient,
	}
	if data.GapTotal > 0 {
		planning.AcceptanceUncoveredRate = float64(data.GapDistribution["acceptance_uncovered"]) / float64(data.GapTotal) * 100
		planning.ScopeViolationRate = float64(data.GapDistribution["scope_violation"]) / float64(data.GapTotal) * 100
		planning.IntegrationGapRate = float64(data.GapDistribution["integration"]) / float64(data.GapTotal) * 100

		if planning.AcceptanceUncoveredRate+planning.ScopeViolationRate+planning.IntegrationGapRate > 40 {
			planning.Verdict = VerdictNeedsMoreContext
		}
	}
	data.Planning = planning

	// Decision review: join decisions.jsonl against tasks with gaps this release
	if s.decisions != nil {
		data.DecisionsLoaded = true
		rawDecisions, err := s.decisions.ReadRetroDecisions(epicID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: read decisions for %s: %v\n", epicID, err)
		} else {
			data.DecisionReviews = buildDecisionReviews(metrics, taskStoryMap, parseJSONL[DecisionEntry](strings.NewReader(rawDecisions), "decisions.jsonl"))
		}
	}

	return &data, nil
}

// RenderReport executes the markdown template against prepared ReportData.
// Pure function — no I/O.
func RenderReport(data *ReportData) (string, error) {
	var buf bytes.Buffer
	if err := reportTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("retro report render: %w", err)
	}
	return buf.String(), nil
}

// buildDecisionReviews cross-references metrics and decisions to produce
// one DecisionReview per task that has at least one gap, listing decisions
// whose target is the task itself or its parent story.
func buildDecisionReviews(metrics []SubTaskMetric, taskStoryMap map[string]string, decisions []DecisionEntry) []DecisionReview {
	if len(metrics) == 0 || len(decisions) == 0 {
		return nil
	}
	byTarget := make(map[string][]DecisionEntry)
	for _, d := range decisions {
		if d.Target == "" {
			continue
		}
		byTarget[d.Target] = append(byTarget[d.Target], d)
	}

	var reviews []DecisionReview
	for _, m := range metrics {
		if m.GapCount == 0 {
			continue
		}
		storyID := taskStoryMap[m.Task]
		matched := append([]DecisionEntry(nil), byTarget[m.Task]...)
		if storyID != "" {
			matched = append(matched, byTarget[storyID]...)
		}
		if len(matched) == 0 {
			continue
		}
		reviews = append(reviews, DecisionReview{
			Task:      m.Task,
			StoryID:   storyID,
			GapCount:  m.GapCount,
			GapTypes:  m.GapTypes,
			Decisions: matched,
		})
	}
	sort.Slice(reviews, func(i, j int) bool {
		return reviews[i].Task < reviews[j].Task
	})
	return reviews
}

// ReadActions reads the actions.yaml file from the given path.
func ReadActions(path string) (*ActionsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ActionsFile{}, nil
		}
		return nil, fmt.Errorf("read actions: %w", err)
	}

	var af ActionsFile
	if err := yaml.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parse actions: %w", err)
	}
	return &af, nil
}

// WriteActions writes the actions.yaml file to the given path.
func WriteActions(path string, af *ActionsFile) error {
	data, err := yaml.Marshal(af)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create actions dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write actions: %w", err)
	}
	return nil
}

// Report template
var reportTmplStr = `# Retro: {{.ReleaseLabel}} — {{.EpicName}}

## 總覽

| 指標 | 值 |
|------|---|
| Stories | {{.Stories.Done}} done / {{.Stories.InReview}} in-review / {{.Stories.Total}} total |
| Tasks | {{.Tasks.Done}} done / {{.Tasks.InReview}} in-review / {{.Tasks.Total}} total |
| 一次通過率 | {{printf "%.0f" .FirstPassRate}}% ({{.FirstPassCount}}/{{.TotalTasks}}) |

## 執行品質

| 指標 | 值 |
|------|---|
| Gap 總數 | {{.GapTotal}} |
{{- range $type, $count := .GapDistribution}}
| Gap: {{$type}} | {{$count}} |
{{- end}}
| 平均 rework 次數 | {{printf "%.1f" .AvgReworkCount}} |

## 規劃品質診斷

| 指標 | 值 |
|------|---|
| acceptance_uncovered 佔比 | {{printf "%.0f" .Planning.AcceptanceUncoveredRate}}% |
| scope_violation 佔比 | {{printf "%.0f" .Planning.ScopeViolationRate}}% |
| integration 佔比 | {{printf "%.0f" .Planning.IntegrationGapRate}}% |
{{- if gt .AANTaskCount 0}}
| 有 A/A/N 的 task 一次通過率 | {{printf "%.0f" .AANPassRate}}% ({{.AANTaskCount}} tasks) |
{{- end}}
{{- if gt .NonAANTaskCount 0}}
| 無 A/A/N 的 task 一次通過率 | {{printf "%.0f" .NonAANPassRate}}% ({{.NonAANTaskCount}} tasks) |
{{- end}}
| 無 A/A/N 的 story 數 | {{.Planning.StoriesWithoutAAN}} |
| 診斷 | {{if eq .Planning.Verdict "sufficient"}}規劃 context 充足{{else}}規劃 context 可能不足{{end}} |

## Verify 統計

{{- if gt .Verify.TotalVerifiedTasks 0}}

| 指標 | 值 |
|------|---|
| 已跑過 /submit 的 task | {{.Verify.TotalVerifiedTasks}} |
| /submit 一次通過 | {{.Verify.VerifyPassedFirstTry}} ({{printf "%.0f" .Verify.VerifyPassFirstRate}}%) |
| /submit 耗盡重試上限 | {{.Verify.VerifyExhausted}} |
| Diff-gate 擋下（作弊跡象） | {{.Verify.DiffGateBlocked}} |
| /verify 抽驗失敗 | {{.Verify.DoneSamplingFailed}} |
| 平均 fix loop 次數 | {{printf "%.1f" .Verify.AvgLoopCount}} |
{{- else}}

本 release 無自測指標資料（/submit 尚未使用或 metrics.jsonl 未寫入）。
{{- end}}

## 契約修改統計

{{- if gt .Contracts.Total 0}}

| 變更類型 | 數量 |
|---|---|
| breaking | {{.Contracts.Breaking}} |
| additive | {{.Contracts.Additive}} |
| none | {{.Contracts.None}} |

### 變更明細

| Contract | Version | Type | Summary | Reason |
|---|---|---|---|---|
{{- range .Contracts.Entries}}
| {{.Contract}} | {{.Version}} | {{.ChangeType}} | {{.Summary}} | {{.Reason}} |
{{- end}}
{{- else}}

本 release 無契約 changelog 資料（.vault/epics/*/contracts/ 為空或未掃描）。
{{- end}}

## 決策回顧
{{- if not .DecisionsLoaded}}

本 release 未載入 decision log（Piper 未注入 DecisionLoader）。
{{- else if not .DecisionReviews}}

本 release 有 gap 的 task 中，無對應的 decision log 紀錄。
可能原因：plan session 未 append decision log、gap 非由自動推導產生、或 decision log 尚未累積足夠資料。
{{- else}}

以下列出本 release 中「有 gap」且「對應到 decision log」的 task，供回看當時推導依據：

{{range .DecisionReviews}}
### {{.Task}}{{if .StoryID}} (story {{.StoryID}}){{end}} — {{.GapCount}} gap{{if .GapTypes}}（{{range $i, $t := .GapTypes}}{{if $i}}, {{end}}{{$t}}{{end}}）{{end}}
{{range .Decisions}}
- **{{.Type}}** @ {{.Timestamp}}
  - rationale: {{.Rationale}}
  {{- if .Alternatives}}
  - alternatives: {{.Alternatives}}
  {{- end}}
  {{- if .Output}}
  - output: {{.Output}}
  {{- end}}
  {{- if .Context}}
  - context: {{range $i, $c := .Context}}{{if $i}}, {{end}}{{$c}}{{end}}
  {{- end}}
{{end}}
{{end}}
{{- end}}

## 跨 task 整合 bug（手動標記）

> 若本 release 有通過 /verify 但整合後才發現的 bug，手動列於此處，並補入 .vault/epics/{epic}/retro/cross-task-bugs.jsonl 以利後續分析。

## 指標盲區檢查

{{- if .UncategorizedGaps}}
未分類 gap：{{len .UncategorizedGaps}} 筆
{{range .UncategorizedGaps}}- {{.}}
{{end}}
{{- else}}
未分類 gap：0 筆
{{- end}}

## 建議行動

> 根據以上指標，建議以下改善行動（由人確認後寫入 actions.yaml）：
`

var reportTmpl = template.Must(template.New("retro").Parse(reportTmplStr))
