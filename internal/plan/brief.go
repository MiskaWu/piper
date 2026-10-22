package plan

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed templates/brief.md
var briefTmplStr string

var briefTmpl = template.Must(template.New("brief").Parse(briefTmplStr))

// SiblingTask holds minimal info for the sibling tasks table in a brief.
type SiblingTask struct {
	ID            string
	Name          string
	TargetProject string
	Status        string
}

// BriefData contains all data needed to render a task brief.
type BriefData struct {
	TaskID               string
	TaskName             string
	StoryID              string
	StoryName            string
	EpicName             string
	ActivityName         string
	StepName             string
	ReleaseLabel         string
	Tags                 string
	TargetProject        string
	TargetProjectDisplay string // human-readable name (basename), falls back to TargetProject
	Description          string
	ADRContent           string
	SpikeContent         string
	SiblingTasks         []SiblingTask
	Acceptance           []string // Story-level (L1) acceptance
	TaskAcceptance       []string // Task-level (L3) acceptance with verify commands
	Always               []string
	Ask                  []string
	Never                []string
}

// GenerateBrief renders a task brief from the given data.
func GenerateBrief(data BriefData) (string, error) {
	if data.ADRContent == "" {
		data.ADRContent = "無"
	}
	if data.SpikeContent == "" {
		data.SpikeContent = "無"
	}
	if data.Description == "" {
		data.Description = "（無描述）"
	}
	if data.TargetProjectDisplay == "" {
		data.TargetProjectDisplay = data.TargetProject
	}

	var buf bytes.Buffer
	if err := briefTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("generate brief %s: %w", data.TaskID, err)
	}
	return buf.String(), nil
}
