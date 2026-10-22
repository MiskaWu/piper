package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/miskawu/piper/internal/plan"
)

// StoryMapIO reads and writes story map YAML.
type StoryMapIO interface {
	ReadStoryMap(epicID string) (string, error)
	WriteStoryMap(epicID string, content string) error
}

// ProjectResolver maps a project name to its filesystem path.
type ProjectResolver interface {
	ResolveProject(name string) (path string, ok bool)
}

// PreviewResult contains the dispatch preview grouped by project.
type PreviewResult struct {
	EpicName string                   `json:"epicName"`
	Groups   map[string][]PreviewTask `json:"groups"`
	Total    int                      `json:"total"`
}

// PreviewTask represents a task in the preview.
type PreviewTask struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	StoryID       string `json:"storyId"`
	StoryName     string `json:"storyName"`
	TargetProject string `json:"targetProject"`
	ProjectPath   string `json:"projectPath,omitempty"`
	Dispatchable  bool   `json:"dispatchable"`
	Reason        string `json:"reason,omitempty"`
}

// DispatchRequest specifies which tasks to dispatch.
type DispatchRequest struct {
	EpicID    string   `json:"epicId"`
	ReleaseID string   `json:"releaseId,omitempty"`
	TaskIDs   []string `json:"taskIds,omitempty"` // empty = all dispatchable
}

// DispatchResult contains the outcome of a dispatch execution.
type DispatchResult struct {
	Dispatched []DispatchedTask    `json:"dispatched"`
	Skipped    []SkippedTask       `json:"skipped"`
	Projects   []DispatchedProject `json:"projects"`
}

// DispatchedTask records a successfully dispatched task.
type DispatchedTask struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	StoryID       string `json:"storyId"`
	TargetProject string `json:"targetProject"`
}

// SkippedTask records a task that was skipped.
type SkippedTask struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// DispatchedProject records per-project dispatch info.
type DispatchedProject struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// Service handles task dispatch operations.
type Service struct {
	io       StoryMapIO
	resolver ProjectResolver
	epicBase string
}

// New creates a new Dispatch Service.
func New(io StoryMapIO, resolver ProjectResolver, epicBase string) *Service {
	return &Service{io: io, resolver: resolver, epicBase: epicBase}
}

// Preview shows what would be dispatched without making changes.
func (s *Service) Preview(epicID, releaseID string) (*PreviewResult, error) {
	raw, err := s.io.ReadStoryMap(epicID)
	if err != nil {
		return nil, fmt.Errorf("dispatch preview %s: %w", epicID, err)
	}

	sm, err := plan.ParseStoryMap([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("dispatch preview %s: %w", epicID, err)
	}

	groups := sm.FilterDispatchable(releaseID)

	result := &PreviewResult{
		EpicName: sm.Meta.Name,
		Groups:   make(map[string][]PreviewTask),
	}

	for proj, contexts := range groups {
		projPath, ok := s.resolver.ResolveProject(proj)
		for _, ctx := range contexts {
			pt := PreviewTask{
				ID:            ctx.Task.ID,
				Name:          ctx.Task.Name,
				StoryID:       ctx.StoryID,
				StoryName:     ctx.StoryName,
				TargetProject: proj,
			}
			if ok {
				pt.ProjectPath = projPath
				pt.Dispatchable = true
				result.Total++
			} else {
				pt.Reason = "專案未在 piper 註冊"
			}
			result.Groups[proj] = append(result.Groups[proj], pt)
		}
	}

	return result, nil
}

// Execute performs the dispatch: generates briefs, deploys skills, updates status.
func (s *Service) Execute(req DispatchRequest) (*DispatchResult, error) {
	raw, err := s.io.ReadStoryMap(req.EpicID)
	if err != nil {
		return nil, fmt.Errorf("dispatch execute %s: %w", req.EpicID, err)
	}

	sm, err := plan.ParseStoryMap([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("dispatch execute %s: %w", req.EpicID, err)
	}

	groups := sm.FilterDispatchable(req.ReleaseID)

	// Build allowed set if specific IDs requested
	allowed := make(map[string]bool)
	if len(req.TaskIDs) > 0 {
		for _, id := range req.TaskIDs {
			allowed[id] = true
		}
	}

	result := &DispatchResult{}
	projectCounts := make(map[string]int)
	projectPaths := make(map[string]string)
	storiesToActivate := make(map[string]bool) // track stories that need in-progress

	for proj, contexts := range groups {
		projPath, ok := s.resolver.ResolveProject(proj)
		if !ok {
			for _, ctx := range contexts {
				result.Skipped = append(result.Skipped, SkippedTask{
					ID:     ctx.Task.ID,
					Name:   ctx.Task.Name,
					Reason: "專案未在 piper 註冊",
				})
			}
			continue
		}

		// Setup project directory structure on first dispatch
		if err := s.setupProject(projPath); err != nil {
			return nil, fmt.Errorf("dispatch setup project %s: %w", proj, err)
		}

		for _, ctx := range contexts {
			if len(allowed) > 0 && !allowed[ctx.Task.ID] {
				continue
			}

			if err := s.dispatchTask(sm, ctx, projPath); err != nil {
				return nil, fmt.Errorf("dispatch task %s: %w", ctx.Task.ID, err)
			}

			sm.SetTaskStatus(ctx.Task.ID, "in-progress")
			storiesToActivate[ctx.StoryID] = true
			result.Dispatched = append(result.Dispatched, DispatchedTask{
				ID:            ctx.Task.ID,
				Name:          ctx.Task.Name,
				StoryID:       ctx.StoryID,
				TargetProject: proj,
			})
			projectCounts[proj]++
			projectPaths[proj] = projPath
		}
	}

	// Set parent stories to in-progress
	for storyID := range storiesToActivate {
		sm.SetStoryStatus(storyID, "in-progress")
	}

	// Update story map with new statuses
	if len(result.Dispatched) > 0 {
		sm.Meta.Updated = time.Now().UTC().Format(time.RFC3339)
		data, err := plan.MarshalStoryMap(sm)
		if err != nil {
			return nil, fmt.Errorf("dispatch marshal story map: %w", err)
		}
		if err := s.io.WriteStoryMap(req.EpicID, string(data)); err != nil {
			return nil, fmt.Errorf("dispatch write story map: %w", err)
		}
	}

	for proj, count := range projectCounts {
		result.Projects = append(result.Projects, DispatchedProject{
			Name:  proj,
			Path:  projectPaths[proj],
			Count: count,
		})
	}

	return result, nil
}

// setupProject creates the .plan/briefs/done directory structure and deploys skills.
func (s *Service) setupProject(projPath string) error {
	// Create directory structure
	briefsDir := filepath.Join(projPath, ".plan", "briefs")
	doneDir := filepath.Join(briefsDir, "done")
	if err := os.MkdirAll(doneDir, 0o755); err != nil {
		return fmt.Errorf("create briefs dir: %w", err)
	}

	// Deploy task-brief rule (auto-loaded by Claude Code)
	rulesDir := filepath.Join(projPath, ".claude", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return fmt.Errorf("create rules dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "task-brief.md"), []byte(plan.TaskBriefRule), 0o644); err != nil {
		return fmt.Errorf("write task-brief rule: %w", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "wave-isolation.md"), []byte(plan.WaveIsolationRule), 0o644); err != nil {
		return fmt.Errorf("write wave-isolation rule: %w", err)
	}

	claudeScriptsDir := filepath.Join(projPath, ".claude", "scripts")
	if err := os.MkdirAll(claudeScriptsDir, 0o755); err != nil {
		return fmt.Errorf("create claude scripts dir: %w", err)
	}
	scripts := []struct {
		name    string
		content string
	}{
		{"check-briefs.sh", plan.CheckBriefsScript},
		{"verify-setup-worktree.sh", plan.VerifySetupWorktreeScript},
		{"verify-diff-gate.sh", plan.VerifyDiffGateScript},
		{"verify-cleanup-worktree.sh", plan.VerifyCleanupWorktreeScript},
	}
	for _, s := range scripts {
		if err := os.WriteFile(filepath.Join(claudeScriptsDir, s.name), []byte(s.content), 0o755); err != nil {
			return fmt.Errorf("write %s: %w", s.name, err)
		}
	}

	skills := []struct {
		name    string
		content string
	}{
		{"submit", plan.SubmitSkill},
		{"breakdown", plan.BreakdownSkill},
		{"verify", plan.VerifySkill},
		{"manifest", plan.ManifestSkill},
		{"bind", plan.BindSkill},
	}
	for _, sk := range skills {
		if err := plan.DeploySkill(projPath, sk.name, sk.content); err != nil {
			return err
		}
	}

	// Write infra version stamp (new location: .claude/)
	versionPath := filepath.Join(projPath, ".claude", ".infra-version")
	if err := os.WriteFile(versionPath, []byte(strconv.Itoa(plan.InfraVersion)), 0o644); err != nil {
		return fmt.Errorf("write infra version: %w", err)
	}

	return nil
}

// ReadInfraVersion reads the infrastructure version from a project.
// Returns 0 if the version file does not exist.
func ReadInfraVersion(projectPath string) int {
	data, err := os.ReadFile(filepath.Join(projectPath, ".claude", ".infra-version"))
	if err != nil {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return v
}

// InfraUpdateAvailable returns true if the project's infrastructure is outdated.
func InfraUpdateAvailable(projectPath string) bool {
	return ReadInfraVersion(projectPath) < plan.InfraVersion
}

// SyncInfra updates only infrastructure files (rules, skills, scripts) without dispatching tasks.
func (s *Service) SyncInfra(projectPath string) error {
	return s.setupProject(projectPath)
}

// InfraProjectStatus describes the infra version state of a single project.
type InfraProjectStatus struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	Version         int    `json:"version"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

// InfraStatus describes the infra version state across projects for an epic.
type InfraStatus struct {
	CurrentVersion int                  `json:"currentVersion"`
	Projects       []InfraProjectStatus `json:"projects"`
}

// GetInfraStatus checks infrastructure versions for all target projects in a story map.
func (s *Service) GetInfraStatus(epicID string) (*InfraStatus, error) {
	raw, err := s.io.ReadStoryMap(epicID)
	if err != nil {
		return nil, fmt.Errorf("infra status %s: %w", epicID, err)
	}

	sm, err := plan.ParseStoryMap([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("infra status %s: %w", epicID, err)
	}

	// Collect unique target projects
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

	result := &InfraStatus{
		CurrentVersion: plan.InfraVersion,
	}

	for proj := range projects {
		projPath, ok := s.resolver.ResolveProject(proj)
		if !ok {
			continue
		}

		v := ReadInfraVersion(projPath)
		result.Projects = append(result.Projects, InfraProjectStatus{
			Name:            proj,
			Path:            projPath,
			Version:         v,
			UpdateAvailable: v < plan.InfraVersion,
		})
	}

	return result, nil
}

// dispatchTask generates and writes a brief for a single task.
func (s *Service) dispatchTask(sm *plan.StoryMap, ctx plan.TaskContext, projPath string) error {
	// Find parent story for ADR/Spike references
	storyCtx, _ := sm.FindStory(ctx.StoryID)
	var story *plan.Story
	if storyCtx != nil {
		story = storyCtx.Story
	}

	// Read ADR content if referenced
	adrContent := ""
	if story != nil && story.ADR != "" {
		adrPath := filepath.Join(s.epicBase, "adr", story.ADR+".md")
		if data, err := os.ReadFile(adrPath); err == nil {
			decision := plan.ExtractSection(string(data), "決定")
			consequence := plan.ExtractSection(string(data), "後果")
			var parts []string
			if decision != "" {
				parts = append(parts, "### 決定\n\n"+decision)
			}
			if consequence != "" {
				parts = append(parts, "### 後果\n\n"+consequence)
			}
			adrContent = strings.Join(parts, "\n\n")
		}
	}

	// Read Spike content if referenced
	spikeContent := ""
	if story != nil && story.Spike != "" {
		spikePath := filepath.Join(s.epicBase, "spikes", story.Spike+".md")
		if data, err := os.ReadFile(spikePath); err == nil {
			conclusion := plan.ExtractSection(string(data), "結論")
			if conclusion != "" {
				spikeContent = conclusion
			}
		}
	}

	// Build sibling tasks
	var siblings []plan.SiblingTask
	for _, sib := range ctx.SiblingTasks {
		siblings = append(siblings, plan.SiblingTask{
			ID:            sib.ID,
			Name:          sib.Name,
			TargetProject: sib.TargetProject,
			Status:        sib.Status,
		})
	}

	tags := ""
	if story != nil {
		tags = strings.Join(story.Tags, ", ")
	}
	if tags == "" {
		tags = "—"
	}

	var storyAlways, storyAsk, storyNever []string
	if story != nil {
		storyAlways = story.Always
		storyAsk = story.Ask
		storyNever = story.Never
	}

	brief, err := plan.GenerateBrief(plan.BriefData{
		TaskID:               ctx.Task.ID,
		TaskName:             ctx.Task.Name,
		StoryID:              ctx.StoryID,
		StoryName:            ctx.StoryName,
		EpicName:             sm.Meta.Name,
		ActivityName:         ctx.ActivityName,
		StepName:             ctx.StepName,
		ReleaseLabel:         ctx.ReleaseLabel,
		Tags:                 tags,
		TargetProject:        ctx.Task.TargetProject,
		TargetProjectDisplay: filepath.Base(projPath),
		Description:          ctx.Task.Description,
		ADRContent:           adrContent,
		SpikeContent:         spikeContent,
		SiblingTasks:         siblings,
		Acceptance:           ctx.Acceptance,
		TaskAcceptance:       ctx.TaskAcceptance,
		Always:               storyAlways,
		Ask:                  storyAsk,
		Never:                storyNever,
	})
	if err != nil {
		return err
	}

	briefPath := filepath.Join(projPath, ".plan", "briefs", ctx.Task.ID+".md")
	if err := os.WriteFile(briefPath, []byte(brief), 0o644); err != nil {
		return fmt.Errorf("write brief %s: %w", ctx.Task.ID, err)
	}

	return nil
}
