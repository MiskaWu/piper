package plan

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// StoryMap represents the full story-map.yaml structure.
type StoryMap struct {
	Meta       StoryMapMeta `yaml:"meta"`
	Activities []Activity   `yaml:"activities"`
}

// StoryMapMeta holds epic-level metadata.
type StoryMapMeta struct {
	Name     string    `yaml:"name"`
	Updated  string    `yaml:"updated"`
	Owner    string    `yaml:"owner,omitempty"`
	Releases []Release `yaml:"releases"`
}

// Release represents a release milestone.
type Release struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Color string `yaml:"color,omitempty"`
}

// Activity represents a top-level grouping of steps.
type Activity struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

// Step represents a grouping of stories within an activity.
type Step struct {
	ID      string  `yaml:"id"`
	Name    string  `yaml:"name"`
	Stories []Story `yaml:"stories"`
}

// Story represents a single story in the story map.
type Story struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty,flow"`
	Release     string   `yaml:"release,omitempty"`
	Spike       string   `yaml:"spike,omitempty"`
	ADR         string   `yaml:"adr,omitempty"`
	Status      string   `yaml:"status"`
	Tasks       []Task   `yaml:"tasks"`
	Acceptance  []string `yaml:"acceptance,omitempty,flow"`
	Always      []string `yaml:"always,omitempty,flow"`
	Ask         []string `yaml:"ask,omitempty,flow"`
	Never       []string `yaml:"never,omitempty,flow"`
	Links       []Link   `yaml:"links,omitempty"`
}

// Task represents a single task within a story.
type Task struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description,omitempty"`
	TargetProject string   `yaml:"targetProject"`
	Status        string   `yaml:"status"`
	Acceptance    []string `yaml:"acceptance,omitempty"`
}

// Link represents a relationship between stories.
type Link struct {
	Type   string `yaml:"type"`
	Target string `yaml:"target"`
}

// StoryContext bundles a story with its surrounding context.
type StoryContext struct {
	Story        *Story
	ActivityName string
	StepName     string
	ReleaseLabel string
	Siblings     []Story // other stories in the same step
}

// TaskContext bundles a task with its parent story context.
type TaskContext struct {
	Task           *Task
	StoryID        string
	StoryName      string
	ActivityName   string
	StepName       string
	ReleaseLabel   string
	SiblingTasks   []Task   // other tasks in the same story
	Acceptance     []string // parent story's acceptance criteria (L1)
	TaskAcceptance []string // task's own L3 acceptance (with verify commands)
	Always         []string // parent story's always rules
	Ask            []string // parent story's ask rules
	Never          []string // parent story's never rules
}

// Task / story status values. Four-state workflow (敏捷 in-review 風格):
//
//	todo → in-progress → in-review → done
//
//	in-progress: 實作中
//	in-review:   /submit 綠燈，等獨立驗證者 /verify 收尾
//	done:        /verify 通過
const (
	StatusTodo       = "todo"
	StatusInProgress = "in-progress"
	StatusInReview   = "in-review"
	StatusDone       = "done"
)

// statusOrder maps statuses to their position in the forward workflow chain.
// Used by ValidateStatusTransition to check ordering.
var statusOrder = map[string]int{
	StatusTodo:       0,
	StatusInProgress: 1,
	StatusInReview:   2,
	StatusDone:       3,
}

// validStatuses is the set of accepted task/story status values. Derived from
// statusOrder so the two sources stay in sync automatically.
var validStatuses = func() map[string]bool {
	m := make(map[string]bool, len(statusOrder))
	for s := range statusOrder {
		m[s] = true
	}
	return m
}()

// allowedStatusesList returns the sorted, comma-separated status list used in
// validation error messages.
var allowedStatusesList = func() string {
	keys := make([]string, 0, len(validStatuses))
	for s := range validStatuses {
		keys = append(keys, s)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}()

// ValidateStatusTransition checks whether a status change from → to is allowed.
// Returns nil when allowed, or an error describing the violation.
//
// Rules:
//   - No-op (from == to) is always OK.
//   - done → in-progress: allowed (reopen); done → other: rejected.
//   - Forward moves must be exactly one step at a time (no skipping).
//   - Backward moves to any earlier state are allowed (correction / fix-loop retry).
//
// Empty from ("") is treated as a new task and allowed to transition to any valid status.
func ValidateStatusTransition(from, to string) error {
	if from == to {
		return nil
	}
	if !validStatuses[to] {
		return fmt.Errorf("invalid target status %q", to)
	}
	if from == "" {
		return nil
	}
	if from == StatusDone {
		if to != StatusInProgress {
			return fmt.Errorf("cannot transition from done to %q (only done→in-progress allowed for reopen)", to)
		}
		return nil
	}
	fromPos, fromOK := statusOrder[from]
	toPos, toOK := statusOrder[to]
	if !fromOK {
		return fmt.Errorf("invalid source status %q", from)
	}
	if !toOK {
		return fmt.Errorf("invalid target status %q", to)
	}
	if toPos > fromPos && toPos != fromPos+1 {
		return fmt.Errorf("cannot skip states: %q → %q (must progress one step at a time)", from, to)
	}
	return nil
}

// ValidateStatusTransitions compares old and new story maps and returns a list
// of transition errors. New stories/tasks not present in oldSM are treated as
// creation (no transition check). Missing items in newSM are ignored.
func ValidateStatusTransitions(oldSM, newSM *StoryMap) []string {
	if oldSM == nil || newSM == nil {
		return nil
	}

	oldStoryStatus := map[string]string{}
	oldTaskStatus := map[string]string{}
	for i := range oldSM.Activities {
		for j := range oldSM.Activities[i].Steps {
			for k := range oldSM.Activities[i].Steps[j].Stories {
				st := &oldSM.Activities[i].Steps[j].Stories[k]
				oldStoryStatus[st.ID] = st.Status
				for ti := range st.Tasks {
					oldTaskStatus[st.Tasks[ti].ID] = st.Tasks[ti].Status
				}
			}
		}
	}

	var errs []string
	for i := range newSM.Activities {
		for j := range newSM.Activities[i].Steps {
			for k := range newSM.Activities[i].Steps[j].Stories {
				st := &newSM.Activities[i].Steps[j].Stories[k]
				if prev, ok := oldStoryStatus[st.ID]; ok && prev != st.Status {
					if err := ValidateStatusTransition(prev, st.Status); err != nil {
						errs = append(errs, fmt.Sprintf("story %q: %v", st.ID, err))
					}
				}
				for ti := range st.Tasks {
					task := &st.Tasks[ti]
					if prev, ok := oldTaskStatus[task.ID]; ok && prev != task.Status {
						if err := ValidateStatusTransition(prev, task.Status); err != nil {
							errs = append(errs, fmt.Sprintf("task %q: %v", task.ID, err))
						}
					}
				}
			}
		}
	}
	return errs
}

// ValidateStoryMap parses raw YAML and checks structural + semantic integrity.
// Returns the parsed StoryMap and a list of validation errors.
// Fatal errors (empty content, invalid YAML) are returned as a single-element list
// with a nil StoryMap. Semantic errors return the parsed StoryMap so it can be
// inspected, along with all problems found.
func ValidateStoryMap(data []byte) (*StoryMap, []string) {
	if len(data) == 0 {
		return nil, []string{"content is empty"}
	}

	var sm StoryMap
	if err := yaml.Unmarshal(data, &sm); err != nil {
		return nil, []string{fmt.Sprintf("invalid YAML: %v", err)}
	}

	var errs []string

	// --- structural checks ---
	if sm.Meta.Name == "" {
		errs = append(errs, "meta.name is required")
	}

	// build release ID set for referential integrity check
	releaseIDs := make(map[string]bool, len(sm.Meta.Releases))
	for _, rel := range sm.Meta.Releases {
		if rel.ID == "" {
			errs = append(errs, "meta.releases contains entry with empty id")
		}
		releaseIDs[rel.ID] = true
	}

	storyIDs := make(map[string]int) // id → count
	taskIDs := make(map[string]int)  // task id → count

	for i := range sm.Activities {
		act := &sm.Activities[i]
		if act.ID == "" {
			errs = append(errs, fmt.Sprintf("activity[%d] missing id", i))
			continue
		}
		if len(act.Steps) == 0 {
			errs = append(errs, fmt.Sprintf("activity %q has no steps", act.ID))
		}
		for j := range act.Steps {
			step := &act.Steps[j]
			if step.ID == "" {
				errs = append(errs, fmt.Sprintf("activity %q step[%d] missing id", act.ID, j))
				continue
			}
			if len(step.Stories) == 0 {
				errs = append(errs, fmt.Sprintf("activity %q step %q has no stories", act.ID, step.ID))
			}
			for k := range step.Stories {
				story := &step.Stories[k]

				// normalize nil tags
				if story.Tags == nil {
					story.Tags = []string{}
				}

				if story.ID == "" {
					errs = append(errs, fmt.Sprintf("activity %q step %q story[%d] missing id", act.ID, step.ID, k))
					continue
				}

				storyIDs[story.ID]++

				// status check
				if story.Status == "" {
					errs = append(errs, fmt.Sprintf("story %q missing status", story.ID))
				} else if !validStatuses[story.Status] {
					errs = append(errs, fmt.Sprintf("story %q has invalid status %q (allowed: %s)", story.ID, story.Status, allowedStatusesList))
				}

				// release referential integrity
				if story.Release != "" && len(releaseIDs) > 0 && !releaseIDs[story.Release] {
					errs = append(errs, fmt.Sprintf("story %q references release %q which is not defined in meta.releases", story.ID, story.Release))
				}

				// task validation (tasks are optional for backward compatibility with old story maps)
				for ti := range story.Tasks {
					task := &story.Tasks[ti]
					if task.ID == "" {
						errs = append(errs, fmt.Sprintf("story %q task[%d] missing id", story.ID, ti))
						continue
					}
					taskIDs[task.ID]++
					if task.Name == "" {
						errs = append(errs, fmt.Sprintf("task %q missing name", task.ID))
					}
					if task.TargetProject == "" {
						errs = append(errs, fmt.Sprintf("task %q missing targetProject", task.ID))
					}
					if task.Status == "" {
						errs = append(errs, fmt.Sprintf("task %q missing status", task.ID))
					} else if !validStatuses[task.Status] {
						errs = append(errs, fmt.Sprintf("task %q has invalid status %q (allowed: %s)", task.ID, task.Status, allowedStatusesList))
					}
					for ai, item := range task.Acceptance {
						if item == "" {
							errs = append(errs, fmt.Sprintf("task %q acceptance[%d] is empty", task.ID, ai))
						}
					}
				}
			}
		}
	}

	// duplicate story ID check
	for id, count := range storyIDs {
		if count > 1 {
			errs = append(errs, fmt.Sprintf("story ID %q appears %d times (must be unique)", id, count))
		}
	}

	// duplicate task ID check
	for id, count := range taskIDs {
		if count > 1 {
			errs = append(errs, fmt.Sprintf("task ID %q appears %d times (must be unique)", id, count))
		}
	}

	if len(errs) > 0 {
		return &sm, errs
	}
	return &sm, nil
}

// ParseStoryMap parses raw YAML bytes into a StoryMap.
func ParseStoryMap(data []byte) (*StoryMap, error) {
	var sm StoryMap
	if err := yaml.Unmarshal(data, &sm); err != nil {
		return nil, fmt.Errorf("parse story map: %w", err)
	}
	return &sm, nil
}

// MarshalStoryMap serializes a StoryMap to YAML bytes.
func MarshalStoryMap(sm *StoryMap) ([]byte, error) {
	data, err := yaml.Marshal(sm)
	if err != nil {
		return nil, fmt.Errorf("marshal story map: %w", err)
	}
	return data, nil
}

// FindStory locates a story by ID and returns its context.
func (sm *StoryMap) FindStory(id string) (*StoryContext, bool) {
	releaseMap := make(map[string]string, len(sm.Meta.Releases))
	for _, rel := range sm.Meta.Releases {
		releaseMap[rel.ID] = rel.Label
	}

	for i := range sm.Activities {
		act := &sm.Activities[i]
		for j := range act.Steps {
			step := &act.Steps[j]
			for k := range step.Stories {
				if step.Stories[k].ID == id {
					var siblings []Story
					for l := range step.Stories {
						if l != k {
							siblings = append(siblings, step.Stories[l])
						}
					}
					return &StoryContext{
						Story:        &step.Stories[k],
						ActivityName: act.Name,
						StepName:     step.Name,
						ReleaseLabel: releaseMap[step.Stories[k].Release],
						Siblings:     siblings,
					}, true
				}
			}
		}
	}
	return nil, false
}

// SetStoryStatus updates a story's status by ID. Returns true if found.
func (sm *StoryMap) SetStoryStatus(id, status string) bool {
	for i := range sm.Activities {
		for j := range sm.Activities[i].Steps {
			for k := range sm.Activities[i].Steps[j].Stories {
				if sm.Activities[i].Steps[j].Stories[k].ID == id {
					sm.Activities[i].Steps[j].Stories[k].Status = status
					return true
				}
			}
		}
	}
	return false
}

// FindTask locates a task by ID and returns its context.
func (sm *StoryMap) FindTask(id string) (*TaskContext, bool) {
	releaseMap := make(map[string]string, len(sm.Meta.Releases))
	for _, rel := range sm.Meta.Releases {
		releaseMap[rel.ID] = rel.Label
	}

	for i := range sm.Activities {
		act := &sm.Activities[i]
		for j := range act.Steps {
			step := &act.Steps[j]
			for k := range step.Stories {
				story := &step.Stories[k]
				for ti := range story.Tasks {
					if story.Tasks[ti].ID == id {
						var siblings []Task
						for si := range story.Tasks {
							if si != ti {
								siblings = append(siblings, story.Tasks[si])
							}
						}
						return &TaskContext{
							Task:           &story.Tasks[ti],
							StoryID:        story.ID,
							StoryName:      story.Name,
							ActivityName:   act.Name,
							StepName:       step.Name,
							ReleaseLabel:   releaseMap[story.Release],
							SiblingTasks:   siblings,
							Acceptance:     story.Acceptance,
							TaskAcceptance: story.Tasks[ti].Acceptance,
							Always:         story.Always,
							Ask:            story.Ask,
							Never:          story.Never,
						}, true
					}
				}
			}
		}
	}
	return nil, false
}

// SetTaskStatus updates a task's status by ID. Returns true if found.
func (sm *StoryMap) SetTaskStatus(taskID, status string) bool {
	for i := range sm.Activities {
		for j := range sm.Activities[i].Steps {
			for k := range sm.Activities[i].Steps[j].Stories {
				for ti := range sm.Activities[i].Steps[j].Stories[k].Tasks {
					if sm.Activities[i].Steps[j].Stories[k].Tasks[ti].ID == taskID {
						sm.Activities[i].Steps[j].Stories[k].Tasks[ti].Status = status
						return true
					}
				}
			}
		}
	}
	return false
}

// StatusCounts holds counts by status for stories or tasks.
type StatusCounts struct {
	Total      int `json:"total"`
	Todo       int `json:"todo"`
	InProgress int `json:"inProgress"`
	InReview   int `json:"inReview"`
	Done       int `json:"done"`
}

// ReleaseCounts holds story and task counts for a release.
type ReleaseCounts struct {
	Stories int `json:"stories"`
	Tasks   int `json:"tasks"`
}

// StoryMapStats holds aggregated statistics for a story map.
type StoryMapStats struct {
	Stories             StatusCounts              `json:"stories"`
	Tasks               StatusCounts              `json:"tasks"`
	ByRelease           map[string]ReleaseCounts  `json:"byRelease"`
	ByProject           map[string]int            `json:"byProject"`
	StoriesWithoutTasks int                       `json:"storiesWithoutTasks"`
}

func addStatus(c *StatusCounts, status string) {
	c.Total++
	switch status {
	case StatusTodo:
		c.Todo++
	case StatusInProgress:
		c.InProgress++
	case StatusInReview:
		c.InReview++
	case StatusDone:
		c.Done++
	}
}

// Stats computes aggregated statistics from the story map.
func (sm *StoryMap) Stats() StoryMapStats {
	releaseMap := make(map[string]string, len(sm.Meta.Releases))
	for _, rel := range sm.Meta.Releases {
		releaseMap[rel.ID] = rel.Label
	}

	stats := StoryMapStats{
		ByRelease: make(map[string]ReleaseCounts),
		ByProject: make(map[string]int),
	}

	for i := range sm.Activities {
		for j := range sm.Activities[i].Steps {
			for k := range sm.Activities[i].Steps[j].Stories {
				story := &sm.Activities[i].Steps[j].Stories[k]
				addStatus(&stats.Stories, story.Status)

				label := releaseMap[story.Release]
				rc := stats.ByRelease[label]
				rc.Stories++

				if len(story.Tasks) == 0 {
					stats.StoriesWithoutTasks++
				}
				for ti := range story.Tasks {
					task := &story.Tasks[ti]
					addStatus(&stats.Tasks, task.Status)
					rc.Tasks++
					if task.TargetProject != "" {
						stats.ByProject[task.TargetProject]++
					}
				}
				stats.ByRelease[label] = rc
			}
		}
	}

	// Remove empty-key entry if no stories lack a release
	if rc, ok := stats.ByRelease[""]; ok && rc.Stories == 0 && rc.Tasks == 0 {
		delete(stats.ByRelease, "")
	}

	return stats
}

// TargetProjects returns the set of unique target project names across all tasks.
func (sm *StoryMap) TargetProjects() map[string]bool {
	projects := make(map[string]bool)
	for _, act := range sm.Activities {
		for _, step := range act.Steps {
			for _, story := range step.Stories {
				for _, task := range story.Tasks {
					if task.TargetProject != "" {
						projects[task.TargetProject] = true
					}
				}
			}
		}
	}
	return projects
}

// Warnings returns non-blocking warnings about the story map.
// These don't prevent saving but highlight potential issues.
func (sm *StoryMap) Warnings() []string {
	releaseMap := make(map[string]string, len(sm.Meta.Releases))
	for _, rel := range sm.Meta.Releases {
		releaseMap[rel.ID] = rel.Label
	}

	// Build story ID set for link validation
	storyIDSet := make(map[string]bool)
	for i := range sm.Activities {
		for j := range sm.Activities[i].Steps {
			for k := range sm.Activities[i].Steps[j].Stories {
				storyIDSet[sm.Activities[i].Steps[j].Stories[k].ID] = true
			}
		}
	}

	var warnings []string
	for i := range sm.Activities {
		for j := range sm.Activities[i].Steps {
			for k := range sm.Activities[i].Steps[j].Stories {
				story := &sm.Activities[i].Steps[j].Stories[k]

				// Link target validation applies to ALL stories (not gated by status)
				for _, link := range story.Links {
					if link.Target != "" && !storyIDSet[link.Target] {
						warnings = append(warnings, fmt.Sprintf("story %q has link to %q which does not exist in story map", story.ID, link.Target))
					}
				}

				if story.Status == StatusDone || story.Status == StatusInReview {
					continue
				}
				if story.Release != "" && len(story.Tasks) == 0 {
					label := releaseMap[story.Release]
					if label == "" {
						label = story.Release
					}
					warnings = append(warnings, fmt.Sprintf("story %q (%s) 已指定 release %q 但尚未拆分 tasks，無法 dispatch", story.ID, story.Name, label))
				}
				if len(story.Tasks) > 0 && len(story.Acceptance) == 0 {
					warnings = append(warnings, fmt.Sprintf("story %q (%s) 已有 tasks 但尚未撰寫驗收條件（acceptance），dispatch 前須補齊", story.ID, story.Name))
				}
			}
		}
	}
	return warnings
}

// FilterDispatchable returns tasks that are dispatchable (task status=todo, task has targetProject),
// grouped by targetProject. If releaseID is non-empty, only stories in that release are included.
// Dispatch eligibility is determined by task status only; story status is not a gate.
func (sm *StoryMap) FilterDispatchable(releaseID string) map[string][]TaskContext {
	releaseMap := make(map[string]string, len(sm.Meta.Releases))
	for _, rel := range sm.Meta.Releases {
		releaseMap[rel.ID] = rel.Label
	}

	result := make(map[string][]TaskContext)

	for i := range sm.Activities {
		act := &sm.Activities[i]
		for j := range act.Steps {
			step := &act.Steps[j]
			for k := range step.Stories {
				s := &step.Stories[k]
				if releaseID != "" && s.Release != releaseID {
					continue
				}

				for ti := range s.Tasks {
					task := &s.Tasks[ti]
					if task.Status != "todo" {
						continue
					}
					if task.TargetProject == "" {
						continue
					}

					var siblings []Task
					for si := range s.Tasks {
						if si != ti {
							siblings = append(siblings, s.Tasks[si])
						}
					}

					ctx := TaskContext{
						Task:           task,
						StoryID:        s.ID,
						StoryName:      s.Name,
						ActivityName:   act.Name,
						StepName:       step.Name,
						ReleaseLabel:   releaseMap[s.Release],
						SiblingTasks:   siblings,
						Acceptance:     s.Acceptance,
						TaskAcceptance: task.Acceptance,
						Always:         s.Always,
						Ask:            s.Ask,
						Never:          s.Never,
					}
					result[task.TargetProject] = append(result[task.TargetProject], ctx)
				}
			}
		}
	}

	return result
}
