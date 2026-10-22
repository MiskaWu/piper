package retro

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/miskawu/piper/internal/plan"
)

// MinRetroReportSize is the minimum byte count for a retro report file to
// be considered "successfully generated". Reports below this threshold are
// treated as failed and will be auto-regenerated on the next retro run.
// A complete paragraph is roughly 2-3 sentences, so 200 bytes lets us
// distinguish broken renders from legitimate short reports.
const MinRetroReportSize = 200

// Retro planning actions per release.
const (
	ActionGenerate   = "generate"
	ActionRegenerate = "regenerate"
	ActionSkip       = "skip"
)

// Retro execution outcomes per release.
const (
	OutcomeGenerated   = "generated"
	OutcomeRegenerated = "regenerated"
	OutcomeSkipped     = "skipped"
	OutcomeFailed      = "failed"
)

// ExecutorStore extends StoryMapIO with the write-capable operations the
// retro closure step needs. VaultManager satisfies this interface out of the
// box; tests can supply a lightweight fake.
type ExecutorStore interface {
	StoryMapIO
	WriteStoryMap(epicID string, content string) error
	ListRetroReports(epicID string) ([]string, error)
	ReadRetroReport(epicID, name string) (string, error)
	WriteRetroReport(epicID, name, content string) error
	WriteRetroReportJSON(epicID, name string, data []byte) error
	ReadRetroReportJSON(epicID, name string) ([]byte, error)
	ReadRetroActions(epicID string) (string, error)
	WriteRetroActions(epicID, content string) error
}

// WithExecutor attaches an ExecutorStore so the service can run the combined
// "close ready stories + generate retro reports" flow. Without it, only the
// read-only operations (CollectMetrics, GenerateReport) are available.
func (s *Service) WithExecutor(executor ExecutorStore) *Service {
	s.executor = executor
	return s
}

// ExecuteRequest configures a retro-closure run.
type ExecuteRequest struct {
	EpicID string `json:"epicId"`
	// ForceReleases lists release IDs whose retro reports should be
	// regenerated even if a valid report already exists. Empty = honour
	// idempotency and skip releases with valid reports.
	ForceReleases []string `json:"forceReleases,omitempty"`
}

// ExecutePreview describes what a retro run will do without making changes.
type ExecutePreview struct {
	EpicName         string               `json:"epicName"`
	ClosableStories  []ClosableStoryEntry `json:"closableStories"`
	RetroActions     []RetroReleaseAction `json:"retroActions"`
}

// ClosableStoryEntry is a story whose tasks are all done but whose own
// status is still in-progress (or in-review) — candidate for closure.
type ClosableStoryEntry struct {
	StoryID   string `json:"storyId"`
	StoryName string `json:"storyName"`
	ReleaseID string `json:"releaseId,omitempty"`
	TaskCount int    `json:"taskCount"`
}

// RetroReleaseAction describes what this release will get on retro run.
type RetroReleaseAction struct {
	ReleaseID    string `json:"releaseId"`
	ReleaseLabel string `json:"releaseLabel"`
	Action       string `json:"action"` // "generate" | "regenerate" | "skip"
	Reason       string `json:"reason,omitempty"`
}

// ExecuteResult records what actually happened.
type ExecuteResult struct {
	ClosedStories []ClosableStoryEntry `json:"closedStories"`
	Retros        []RetroOutcome       `json:"retros"`
}

// RetroOutcome is one release's retro outcome after execution.
type RetroOutcome struct {
	ReleaseID    string `json:"releaseId"`
	ReleaseLabel string `json:"releaseLabel"`
	Status       string `json:"status"` // "generated" | "regenerated" | "skipped" | "failed"
	ReportName   string `json:"reportName,omitempty"`
	Error        string `json:"error,omitempty"`
}

// Preview inspects the story map and returns the closures and retro actions
// a subsequent Execute call would perform. Read-only.
func (s *Service) Preview(epicID string) (*ExecutePreview, error) {
	if s.executor == nil {
		return nil, fmt.Errorf("retro preview: executor store not configured")
	}
	raw, err := s.executor.ReadStoryMap(epicID)
	if err != nil {
		return nil, fmt.Errorf("retro preview read story map %s: %w", epicID, err)
	}
	sm, err := plan.ParseStoryMap([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("retro preview parse story map %s: %w", epicID, err)
	}

	closable := findClosableStories(sm)
	actions, err := s.planRetroActions(epicID, sm, closable, nil)
	if err != nil {
		return nil, err
	}

	return &ExecutePreview{
		EpicName:        sm.Meta.Name,
		ClosableStories: closable,
		RetroActions:    actions,
	}, nil
}

// Execute closes ready stories and generates retro reports for qualifying
// releases. Idempotent: running it twice with the same inputs only produces
// fresh output when story state has advanced or ForceReleases is non-empty.
func (s *Service) Execute(req ExecuteRequest) (*ExecuteResult, error) {
	if s.executor == nil {
		return nil, fmt.Errorf("retro execute: executor store not configured")
	}
	raw, err := s.executor.ReadStoryMap(req.EpicID)
	if err != nil {
		return nil, fmt.Errorf("retro execute read story map %s: %w", req.EpicID, err)
	}
	sm, err := plan.ParseStoryMap([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("retro execute parse story map %s: %w", req.EpicID, err)
	}

	// --- Phase 1: close stories whose tasks are all done ---
	closable := findClosableStories(sm)
	closed := applyStoryClosures(sm, closable)
	if len(closed) > 0 {
		sm.Meta.Updated = time.Now().UTC().Format(time.RFC3339)
		data, err := plan.MarshalStoryMap(sm)
		if err != nil {
			return nil, fmt.Errorf("retro execute marshal story map: %w", err)
		}
		if err := s.executor.WriteStoryMap(req.EpicID, string(data)); err != nil {
			return nil, fmt.Errorf("retro execute write story map: %w", err)
		}
	}

	// --- Phase 2: decide retro actions per release ---
	forceSet := make(map[string]bool, len(req.ForceReleases))
	for _, id := range req.ForceReleases {
		forceSet[id] = true
	}
	actions, err := s.planRetroActions(req.EpicID, sm, closable, forceSet)
	if err != nil {
		return nil, err
	}

	// --- Phase 3: execute each retro action ---
	result := &ExecuteResult{
		ClosedStories: closed,
		Retros:        make([]RetroOutcome, 0, len(actions)),
	}
	for _, action := range actions {
		result.Retros = append(result.Retros, s.runRetroAction(req.EpicID, action))
	}

	return result, nil
}

// runRetroAction executes one planned retro action and returns its outcome.
// Errors are captured in the outcome (not returned) so Execute can continue
// with remaining releases even when one report fails.
func (s *Service) runRetroAction(epicID string, action RetroReleaseAction) RetroOutcome {
	outcome := RetroOutcome{
		ReleaseID:    action.ReleaseID,
		ReleaseLabel: action.ReleaseLabel,
	}
	if action.Action == ActionSkip {
		outcome.Status = OutcomeSkipped
		return outcome
	}

	fail := func(err error) RetroOutcome {
		outcome.Status = OutcomeFailed
		outcome.Error = err.Error()
		return outcome
	}

	data, err := s.BuildReportData(epicID, action.ReleaseLabel)
	if err != nil {
		return fail(err)
	}
	content, err := RenderReport(data)
	if err != nil {
		return fail(err)
	}
	reportName := RetroReportName(action.ReleaseLabel)

	if snapshot, jerr := json.Marshal(data); jerr == nil {
		if werr := s.executor.WriteRetroReportJSON(epicID, reportName, snapshot); werr != nil {
			fmt.Fprintf(os.Stderr, "warning: write retro snapshot %s: %v\n", reportName, werr)
		}
	} else {
		fmt.Fprintf(os.Stderr, "warning: marshal retro snapshot %s: %v\n", reportName, jerr)
	}

	if err := s.executor.WriteRetroReport(epicID, reportName, content); err != nil {
		return fail(err)
	}

	if action.Action == ActionRegenerate {
		outcome.Status = OutcomeRegenerated
	} else {
		outcome.Status = OutcomeGenerated
	}
	outcome.ReportName = reportName
	return outcome
}

// findClosableStories returns stories whose tasks are all done but whose
// own status is still in-progress or in-review. Stories with no tasks are
// not included — closure is only meaningful once work has been done.
func findClosableStories(sm *plan.StoryMap) []ClosableStoryEntry {
	var out []ClosableStoryEntry
	for i := range sm.Activities {
		for j := range sm.Activities[i].Steps {
			for k := range sm.Activities[i].Steps[j].Stories {
				story := &sm.Activities[i].Steps[j].Stories[k]
				if story.Status != plan.StatusInProgress && story.Status != plan.StatusInReview {
					continue
				}
				if len(story.Tasks) == 0 {
					continue
				}
				allDone := true
				for ti := range story.Tasks {
					if story.Tasks[ti].Status != plan.StatusDone {
						allDone = false
						break
					}
				}
				if !allDone {
					continue
				}
				out = append(out, ClosableStoryEntry{
					StoryID:   story.ID,
					StoryName: story.Name,
					ReleaseID: story.Release,
					TaskCount: len(story.Tasks),
				})
			}
		}
	}
	return out
}

// applyStoryClosures mutates sm in-place, setting the status of closable
// stories to done. Returns the entries that were actually changed.
func applyStoryClosures(sm *plan.StoryMap, closable []ClosableStoryEntry) []ClosableStoryEntry {
	if len(closable) == 0 {
		return nil
	}
	ids := make(map[string]bool, len(closable))
	for _, c := range closable {
		ids[c.StoryID] = true
	}
	var closed []ClosableStoryEntry
	for i := range sm.Activities {
		for j := range sm.Activities[i].Steps {
			for k := range sm.Activities[i].Steps[j].Stories {
				story := &sm.Activities[i].Steps[j].Stories[k]
				if !ids[story.ID] {
					continue
				}
				story.Status = plan.StatusDone
				closed = append(closed, ClosableStoryEntry{
					StoryID:   story.ID,
					StoryName: story.Name,
					ReleaseID: story.Release,
					TaskCount: len(story.Tasks),
				})
			}
		}
	}
	return closed
}

// planRetroActions decides what retro action each release gets.
//
// A release is eligible for retro when all its stories are (or will be
// after closure) done. For eligible releases:
//
//   - If no valid report exists → generate
//   - If valid report exists and release is in forceSet → regenerate
//   - If valid report exists and release is NOT in forceSet → skip
//
// Releases that still have open stories are not included in the result.
func (s *Service) planRetroActions(
	epicID string,
	sm *plan.StoryMap,
	willClose []ClosableStoryEntry,
	forceSet map[string]bool,
) ([]RetroReleaseAction, error) {
	willCloseSet := make(map[string]bool, len(willClose))
	for _, c := range willClose {
		willCloseSet[c.StoryID] = true
	}

	releaseMap := make(map[string]string, len(sm.Meta.Releases))
	for _, rel := range sm.Meta.Releases {
		releaseMap[rel.ID] = rel.Label
	}

	type releaseTally struct {
		total int
		done  int
	}
	tally := make(map[string]*releaseTally)

	for i := range sm.Activities {
		for j := range sm.Activities[i].Steps {
			for k := range sm.Activities[i].Steps[j].Stories {
				story := &sm.Activities[i].Steps[j].Stories[k]
				if story.Release == "" {
					continue
				}
				t, ok := tally[story.Release]
				if !ok {
					t = &releaseTally{}
					tally[story.Release] = t
				}
				t.total++
				// Treat stories being closed this run as done, so a retro
				// triggers in the same execution that closes them.
				if story.Status == plan.StatusDone || willCloseSet[story.ID] {
					t.done++
				}
			}
		}
	}

	actions := make([]RetroReleaseAction, 0)
	for relID, t := range tally {
		if t.total == 0 || t.total != t.done {
			continue
		}
		label := releaseMap[relID]
		if label == "" {
			label = relID
		}
		hasValid, err := s.hasValidRetroReport(epicID, label)
		if err != nil {
			return nil, err
		}
		action := RetroReleaseAction{ReleaseID: relID, ReleaseLabel: label}
		switch {
		case !hasValid:
			action.Action = ActionGenerate
			action.Reason = "no valid retro report yet"
		case forceSet[relID]:
			action.Action = ActionRegenerate
			action.Reason = "forced rerun"
		default:
			action.Action = ActionSkip
			action.Reason = "valid retro report already exists"
		}
		actions = append(actions, action)
	}
	return actions, nil
}

// hasValidRetroReport returns true if the release already has a retro report
// that passes the size threshold (MinRetroReportSize). Read errors are treated
// as "no valid report" so the caller regenerates — this covers both missing
// files and unreadable ones without forcing callers to distinguish.
func (s *Service) hasValidRetroReport(epicID, releaseLabel string) (bool, error) {
	content, err := s.executor.ReadRetroReport(epicID, RetroReportName(releaseLabel))
	if err != nil {
		return false, nil
	}
	return len(content) >= MinRetroReportSize, nil
}

// RetroReportName derives the canonical report filename from a release label.
// Callers outside this package (e.g. routes that accept a releaseLabel and
// want to store/look up the corresponding report) should use this so the
// naming stays in one place.
func RetroReportName(releaseLabel string) string {
	return "release-" + strings.ReplaceAll(strings.ToLower(releaseLabel), " ", "-")
}

// TrendEntry is one release's key metrics, projected from the full ReportData
// snapshot. Used by the UI to render cross-release trend charts and delta
// cards without loading each full report.
type TrendEntry struct {
	ReleaseLabel       string         `json:"releaseLabel"`
	ReportName         string         `json:"reportName"`
	HasSnapshot        bool           `json:"hasSnapshot"`
	FirstPassRate      float64        `json:"firstPassRate"`
	FirstPassCount     int            `json:"firstPassCount"`
	TotalTasks         int            `json:"totalTasks"`
	GapTotal           int            `json:"gapTotal"`
	GapDistribution    map[string]int `json:"gapDistribution,omitempty"`
	AvgReworkCount     float64        `json:"avgReworkCount"`
	AANPassRate        float64        `json:"aanPassRate"`
	NonAANPassRate     float64        `json:"nonAanPassRate"`
	AANTaskCount       int            `json:"aanTaskCount"`
	NonAANTaskCount    int            `json:"nonAanTaskCount"`
	VerifyExhausted    int            `json:"verifyExhausted"`
	DiffGateBlocked    int            `json:"diffGateBlocked"`
	DoneSamplingFailed int            `json:"doneSamplingFailed"`
	AvgLoopCount       float64        `json:"avgLoopCount"`
}

// Trends contains the full trend view for an epic: one entry per retro report
// ordered by report name (stable, roughly reflects release chronology since
// names are derived from release labels).
type Trends struct {
	EpicID   string       `json:"epicId"`
	Releases []TrendEntry `json:"releases"`
}

// GetTrends returns one TrendEntry per retro report, whether or not that
// report has a JSON snapshot. Legacy reports (markdown only) show up with
// HasSnapshot=false so the UI can render a placeholder.
func (s *Service) GetTrends(epicID string) (*Trends, error) {
	if s.executor == nil {
		return nil, fmt.Errorf("retro trends: executor store not configured")
	}
	names, err := s.executor.ListRetroReports(epicID)
	if err != nil {
		return nil, fmt.Errorf("retro trends list: %w", err)
	}
	sort.Strings(names)

	entries := make([]TrendEntry, 0, len(names))
	for _, name := range names {
		entry := TrendEntry{ReportName: name, ReleaseLabel: labelFromReportName(name)}
		raw, err := s.executor.ReadRetroReportJSON(epicID, name)
		switch {
		case err == nil:
			var data ReportData
			if jerr := json.Unmarshal(raw, &data); jerr != nil {
				fmt.Fprintf(os.Stderr, "warning: parse retro snapshot %s: %v\n", name, jerr)
			} else {
				populateTrendEntry(&entry, &data)
				entry.HasSnapshot = true
			}
		case errors.Is(err, os.ErrNotExist):
			// legacy markdown-only report — leave HasSnapshot=false
		default:
			fmt.Fprintf(os.Stderr, "warning: read retro snapshot %s: %v\n", name, err)
		}
		entries = append(entries, entry)
	}
	return &Trends{EpicID: epicID, Releases: entries}, nil
}

func populateTrendEntry(e *TrendEntry, d *ReportData) {
	e.ReleaseLabel = d.ReleaseLabel
	e.FirstPassRate = d.FirstPassRate
	e.FirstPassCount = d.FirstPassCount
	e.TotalTasks = d.TotalTasks
	e.GapTotal = d.GapTotal
	e.GapDistribution = d.GapDistribution
	e.AvgReworkCount = d.AvgReworkCount
	e.AANPassRate = d.AANPassRate
	e.NonAANPassRate = d.NonAANPassRate
	e.AANTaskCount = d.AANTaskCount
	e.NonAANTaskCount = d.NonAANTaskCount
	e.VerifyExhausted = d.Verify.VerifyExhausted
	e.DiffGateBlocked = d.Verify.DiffGateBlocked
	e.DoneSamplingFailed = d.Verify.DoneSamplingFailed
	e.AvgLoopCount = d.Verify.AvgLoopCount
}

// labelFromReportName is the inverse of RetroReportName for the common case
// (display fallback when the JSON snapshot is missing). It won't recover
// original casing or spaces, but the UI can still show something readable.
func labelFromReportName(name string) string {
	return strings.TrimPrefix(name, "release-")
}

// ActionStatusTodo and ActionStatusDone are the two permitted values of
// Action.Status. Any other value passed to UpdateActionStatus is rejected.
const (
	ActionStatusTodo = "todo"
	ActionStatusDone = "done"
)

// ErrActionNotFound is returned when UpdateActionStatus or DeleteAction
// cannot locate the given action ID. Route handlers use errors.Is to map
// this into HTTP 404 without string matching on the error message.
var ErrActionNotFound = errors.New("action not found")

// ErrInvalidActionStatus is returned when UpdateActionStatus receives a
// value outside {ActionStatusTodo, ActionStatusDone}. Route handlers map
// this to HTTP 400.
var ErrInvalidActionStatus = errors.New("invalid action status")

// ListActions returns the current retro improvement actions for an epic.
// Empty slice (not nil) when there are no actions, so JSON encoders don't
// emit `null`.
func (s *Service) ListActions(epicID string) ([]Action, error) {
	if s.executor == nil {
		return nil, fmt.Errorf("retro actions: executor store not configured")
	}
	af, err := s.loadActions(epicID)
	if err != nil {
		return nil, err
	}
	if af.Actions == nil {
		return []Action{}, nil
	}
	return af.Actions, nil
}

// AddAction appends a new action and returns it with a generated ID. fromRetro
// identifies which retro report prompted this action (may be empty for
// ad-hoc additions from the UI).
func (s *Service) AddAction(epicID, fromRetro, description string) (*Action, error) {
	if s.executor == nil {
		return nil, fmt.Errorf("retro actions: executor store not configured")
	}
	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("action description cannot be empty")
	}
	af, err := s.loadActions(epicID)
	if err != nil {
		return nil, err
	}
	action := Action{
		ID:          newActionID(),
		FromRetro:   fromRetro,
		Description: description,
		Status:      ActionStatusTodo,
	}
	af.Actions = append(af.Actions, action)
	if err := s.saveActions(epicID, af); err != nil {
		return nil, err
	}
	return &action, nil
}

// UpdateActionStatus toggles an action between todo and done. Setting done
// stamps AppliedAt with current UTC time; reverting to todo clears it.
func (s *Service) UpdateActionStatus(epicID, actionID, status string) error {
	if s.executor == nil {
		return fmt.Errorf("retro actions: executor store not configured")
	}
	if status != ActionStatusTodo && status != ActionStatusDone {
		return fmt.Errorf("%w %q (want %q or %q)", ErrInvalidActionStatus, status, ActionStatusTodo, ActionStatusDone)
	}
	af, err := s.loadActions(epicID)
	if err != nil {
		return err
	}
	for i := range af.Actions {
		if af.Actions[i].ID == actionID {
			af.Actions[i].Status = status
			if status == ActionStatusDone {
				af.Actions[i].AppliedAt = time.Now().UTC().Format(time.RFC3339)
			} else {
				af.Actions[i].AppliedAt = ""
			}
			return s.saveActions(epicID, af)
		}
	}
	return fmt.Errorf("%w: %q", ErrActionNotFound, actionID)
}

// DeleteAction removes an action by ID.
func (s *Service) DeleteAction(epicID, actionID string) error {
	if s.executor == nil {
		return fmt.Errorf("retro actions: executor store not configured")
	}
	af, err := s.loadActions(epicID)
	if err != nil {
		return err
	}
	filtered := make([]Action, 0, len(af.Actions))
	found := false
	for _, a := range af.Actions {
		if a.ID == actionID {
			found = true
			continue
		}
		filtered = append(filtered, a)
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrActionNotFound, actionID)
	}
	af.Actions = filtered
	return s.saveActions(epicID, af)
}

func (s *Service) loadActions(epicID string) (*ActionsFile, error) {
	raw, err := s.executor.ReadRetroActions(epicID)
	if err != nil {
		return nil, fmt.Errorf("read actions: %w", err)
	}
	var af ActionsFile
	if strings.TrimSpace(raw) == "" {
		return &af, nil
	}
	if err := yaml.Unmarshal([]byte(raw), &af); err != nil {
		return nil, fmt.Errorf("parse actions yaml: %w", err)
	}
	return &af, nil
}

func (s *Service) saveActions(epicID string, af *ActionsFile) error {
	data, err := yaml.Marshal(af)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	return s.executor.WriteRetroActions(epicID, string(data))
}

// newActionID produces an opaque, URL-safe identifier for a new action.
// Uses unix nano + random suffix for rough sortability without coupling to
// wall-clock time displays.
func newActionID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("action-%d-%x", time.Now().UnixNano(), b)
}
