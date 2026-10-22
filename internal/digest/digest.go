package digest

import (
	"path/filepath"
	"sort"
	"time"

	"github.com/miskawu/piper/internal/usage"
)

// SessionInfo holds metadata for a session relevant to digest generation.
type SessionInfo struct {
	Description string
	ProjectPath string
}

// Sources provides all raw data needed to generate a daily digest.
type Sources struct {
	WorkTimeEntries     []usage.WorkTimeEntry
	SessionDescriptions map[string]SessionInfo
	// Commits per projectPath — collected from git log.
	ProjectCommits map[string][]string
	// Conversations per projectPath — collected from Claude history.
	ProjectConversations map[string][]string
}

// Digest is the daily work summary.
type Digest struct {
	Date         string          `json:"date"`
	TotalWorkSec int             `json:"totalWorkSec"`
	Projects     []ProjectDigest `json:"projects"`
}

// ProjectDigest groups session activity by project.
type ProjectDigest struct {
	ProjectPath   string          `json:"projectPath"`
	ProjectName   string          `json:"projectName"`
	TotalWorkSec  int             `json:"totalWorkSec"`
	Summary       string          `json:"summary,omitempty"`
	Commits       []string        `json:"commits,omitempty"`
	Conversations []string        `json:"conversations,omitempty"`
	Sessions      []SessionDigest `json:"sessions"`
}

// SessionDigest summarizes a single session's activity for the day.
type SessionDigest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	WorkSec     int    `json:"workSec"`
}

// Generate creates a daily digest for the given date (format: "2006-01-02").
// loc determines the timezone used to map UTC timestamps to calendar dates.
// If loc is nil, time.UTC is used.
func Generate(date string, src *Sources, loc *time.Location) *Digest {
	if loc == nil {
		loc = time.UTC
	}
	d := &Digest{
		Date:     date,
		Projects: []ProjectDigest{},
	}

	// Per-session accumulators
	type sessionAcc struct {
		workSec     int
		projectPath string // from JSONL entry (first non-empty wins)
	}
	sessions := map[string]*sessionAcc{}
	ensureSession := func(name string) *sessionAcc {
		if s, ok := sessions[name]; ok {
			return s
		}
		s := &sessionAcc{}
		sessions[name] = s
		return s
	}

	// Aggregate work time
	for _, e := range src.WorkTimeEntries {
		if e.Timestamp.In(loc).Format("2006-01-02") != date {
			continue
		}
		s := ensureSession(e.SessionName)
		s.workSec += e.DurationSec
		d.TotalWorkSec += e.DurationSec
		if s.projectPath == "" && e.ProjectPath != "" {
			s.projectPath = e.ProjectPath
		}
	}

	// Group sessions by project
	projectMap := map[string]*ProjectDigest{} // projectPath -> ProjectDigest
	var projectOrder []string

	for name, acc := range sessions {
		// Prefer projectPath from JSONL entry; fallback to SessionDescriptions
		projPath := acc.projectPath
		if projPath == "" {
			projPath = src.SessionDescriptions[name].ProjectPath
		}

		proj, ok := projectMap[projPath]
		if !ok {
			proj = &ProjectDigest{
				ProjectPath: projPath,
				ProjectName: filepath.Base(projPath),
			}
			if projPath == "" {
				proj.ProjectName = ""
			}
			projectMap[projPath] = proj
			projectOrder = append(projectOrder, projPath)
		}

		proj.Sessions = append(proj.Sessions, SessionDigest{
			Name:        name,
			Description: src.SessionDescriptions[name].Description,
			WorkSec:     acc.workSec,
		})
		proj.TotalWorkSec += acc.workSec
	}

	// Attach commits and conversations to each project
	for path, proj := range projectMap {
		if commits, ok := src.ProjectCommits[path]; ok {
			proj.Commits = commits
		}
		if convos, ok := src.ProjectConversations[path]; ok {
			proj.Conversations = convos
		}
	}

	// Collect and sort projects by total work time (descending)
	for _, path := range projectOrder {
		d.Projects = append(d.Projects, *projectMap[path])
	}
	sort.Slice(d.Projects, func(i, j int) bool {
		return d.Projects[i].TotalWorkSec > d.Projects[j].TotalWorkSec
	})

	return d
}
