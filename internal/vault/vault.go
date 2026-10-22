package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/miskawu/piper/internal/plan"
	"github.com/miskawu/piper/internal/specextract"
)

// VaultManager manages the unified vault git repository.
type VaultManager struct {
	baseDir string
	git     plan.GitRunner
	epic    *plan.EpicManager
	mu      sync.RWMutex
	nextID  int
}

// Option configures VaultManager.
type Option func(*VaultManager)

// WithGitRunner injects a custom GitRunner (for testing).
func WithGitRunner(g plan.GitRunner) Option {
	return func(v *VaultManager) {
		v.git = g
	}
}

// New creates a new VaultManager.
func New(baseDir string, opts ...Option) *VaultManager {
	v := &VaultManager{baseDir: baseDir}
	for _, opt := range opts {
		opt(v)
	}
	if v.git == nil {
		v.git = plan.NewExecGitRunner()
	}
	return v
}

// Init initializes the vault directory structure and git repo.
func (v *VaultManager) Init() error {
	// Create directories
	for _, dir := range []string{v.baseDir, filepath.Join(v.baseDir, "epics"), filepath.Join(v.baseDir, "inbox")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("vault init: create %s: %w", dir, err)
		}
	}

	// Init git if needed
	if !v.git.IsRepo(v.baseDir) {
		if err := v.git.Init(v.baseDir); err != nil {
			return fmt.Errorf("vault init: git init: %w", err)
		}
	}

	// Create EpicManager (no git runner — it won't do git ops)
	v.epic = plan.NewEpicManager(filepath.Join(v.baseDir, "epics"))

	// Load meta
	metaPath := filepath.Join(v.baseDir, "meta.yaml")
	nextID, err := loadMeta(metaPath)
	if err != nil {
		return fmt.Errorf("vault init: %w", err)
	}
	v.nextID = nextID

	return nil
}

// BaseDir returns the vault base directory (satisfies SyncableRepo).
func (v *VaultManager) BaseDir() string { return v.baseDir }

// Git returns the GitRunner (satisfies SyncableRepo).
func (v *VaultManager) Git() plan.GitRunner { return v.git }

// EpicManager returns the underlying EpicManager.
func (v *VaultManager) EpicManager() *plan.EpicManager { return v.epic }

// autoCommit stages paths and commits. Errors are logged but not propagated.
func (v *VaultManager) autoCommit(msg string, paths ...string) {
	if v.git == nil {
		return
	}
	if !v.git.IsRepo(v.baseDir) {
		return
	}
	if err := v.git.Add(v.baseDir, paths...); err != nil {
		log.Printf("WARNING: vault git add: %v", err)
		return
	}
	if err := v.git.Commit(v.baseDir, msg); err != nil {
		log.Printf("WARNING: vault git commit: %v", err)
	}
}

// --- Epic wrapper methods ---

// CreateEpic creates a new epic and auto-commits.
func (v *VaultManager) CreateEpic(name string) (string, error) {
	id, err := v.epic.CreateEpic(name)
	if err != nil {
		return "", err
	}
	v.autoCommit(fmt.Sprintf("create epic: %s", id), filepath.Join("epics", id))
	return id, nil
}

// DeleteEpic removes an epic and auto-commits.
func (v *VaultManager) DeleteEpic(id string) error {
	if err := v.epic.DeleteEpic(id); err != nil {
		return err
	}
	v.autoCommit(fmt.Sprintf("delete epic: %s", id), "-A")
	return nil
}

// WriteStoryMap writes a story map and auto-commits.
func (v *VaultManager) WriteStoryMap(id string, content string) error {
	if err := v.epic.WriteStoryMap(id, content); err != nil {
		return err
	}
	v.autoCommit(fmt.Sprintf("update story-map: %s", id), filepath.Join("epics", id, "story-map.yaml"))
	return nil
}

// RenameEpic renames an epic's display name and auto-commits.
func (v *VaultManager) RenameEpic(id string, newName string) error {
	if err := v.epic.RenameEpic(id, newName); err != nil {
		return err
	}
	// RenameEpic calls WriteStoryMap internally, which already wrote the file.
	// We just need to commit.
	v.autoCommit(fmt.Sprintf("rename epic: %s", id), filepath.Join("epics", id, "story-map.yaml"))
	return nil
}

// ReadStoryMap delegates directly to EpicManager (no git needed).
func (v *VaultManager) ReadStoryMap(id string) (string, error) {
	return v.epic.ReadStoryMap(id)
}

// ListEpics delegates directly to EpicManager.
func (v *VaultManager) ListEpics() ([]plan.EpicInfo, error) {
	return v.epic.ListEpics()
}

// --- Vault file CRUD methods ---

// epicDir returns the validated epic directory path.
func (v *VaultManager) epicDir(epicID string) (string, error) {
	if v.epic == nil {
		return "", fmt.Errorf("vault not initialized")
	}
	dir := filepath.Join(v.epic.BaseDir(), epicID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", fmt.Errorf("epic %s: not found", epicID)
	}
	return dir, nil
}

// safePath validates that filename has no path traversal and stays within the target directory.
func safePath(base, filename string) (string, error) {
	cleaned := filepath.Clean(filename)
	if cleaned != filename || strings.Contains(cleaned, "..") || filepath.IsAbs(cleaned) || strings.ContainsAny(cleaned, `/\`) {
		return "", fmt.Errorf("invalid filename %q", filename)
	}
	return filepath.Join(base, cleaned), nil
}

// listMDFiles lists .md filenames in a directory (without extension).
func listMDFiles(dir string) ([]string, error) {
	return listFilesWithExt(dir, ".md")
}

// listFilesWithExt lists files with the given extension (without the extension suffix).
func listFilesWithExt(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ext) {
			names = append(names, strings.TrimSuffix(e.Name(), ext))
		}
	}
	if names == nil {
		names = []string{}
	}
	return names, nil
}

// --- Problem Statement ---

// ReadProblemStatement reads the problem-statement.md for an epic.
func (v *VaultManager) ReadProblemStatement(epicID string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "problem-statement.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("epic %s: problem-statement.md not found", epicID)
		}
		return "", fmt.Errorf("read problem-statement %s: %w", epicID, err)
	}
	return string(data), nil
}

// WriteProblemStatement writes problem-statement.md with validation and auto-commit.
func (v *VaultManager) WriteProblemStatement(epicID string, content string) error {
	if err := ValidateProblemStatement(content); err != nil {
		return err
	}
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "problem-statement.md")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write problem-statement %s: %w", epicID, err)
	}
	v.autoCommit(fmt.Sprintf("update problem-statement: %s", epicID),
		filepath.Join("epics", epicID, "problem-statement.md"))
	return nil
}

// --- ADR ---

// ListADRs returns ADR names (without .md extension) for an epic.
func (v *VaultManager) ListADRs(epicID string) ([]string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return nil, err
	}
	return listMDFiles(filepath.Join(dir, "adr"))
}

// ReadADR reads a specific ADR file.
func (v *VaultManager) ReadADR(epicID, filename string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	p, err := safePath(filepath.Join(dir, "adr"), filename+".md")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("epic %s: adr %s not found", epicID, filename)
		}
		return "", fmt.Errorf("read adr %s/%s: %w", epicID, filename, err)
	}
	return string(data), nil
}

// WriteADR writes an ADR file with validation and auto-commit.
func (v *VaultManager) WriteADR(epicID, filename, content string) error {
	if err := ValidateADR(content); err != nil {
		return err
	}
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	adrDir := filepath.Join(dir, "adr")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		return fmt.Errorf("write adr %s/%s: %w", epicID, filename, err)
	}
	p, err := safePath(adrDir, filename+".md")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write adr %s/%s: %w", epicID, filename, err)
	}
	v.autoCommit(fmt.Sprintf("update adr: %s/%s", epicID, filename),
		filepath.Join("epics", epicID, "adr", filename+".md"))
	return nil
}

// DeleteADR removes an ADR file and auto-commits.
func (v *VaultManager) DeleteADR(epicID, filename string) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	p, err := safePath(filepath.Join(dir, "adr"), filename+".md")
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("epic %s: adr %s not found", epicID, filename)
		}
		return fmt.Errorf("delete adr %s/%s: %w", epicID, filename, err)
	}
	v.autoCommit(fmt.Sprintf("delete adr: %s/%s", epicID, filename), "-A")
	return nil
}

// --- Spikes ---

// ListSpikes returns spike names (without .md extension) for an epic.
func (v *VaultManager) ListSpikes(epicID string) ([]string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return nil, err
	}
	return listMDFiles(filepath.Join(dir, "spikes"))
}

// ReadSpike reads a specific spike file.
func (v *VaultManager) ReadSpike(epicID, filename string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	p, err := safePath(filepath.Join(dir, "spikes"), filename+".md")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("epic %s: spike %s not found", epicID, filename)
		}
		return "", fmt.Errorf("read spike %s/%s: %w", epicID, filename, err)
	}
	return string(data), nil
}

// WriteSpike writes a spike file and auto-commits (no special validation).
func (v *VaultManager) WriteSpike(epicID, filename, content string) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	spikesDir := filepath.Join(dir, "spikes")
	if err := os.MkdirAll(spikesDir, 0o755); err != nil {
		return fmt.Errorf("write spike %s/%s: %w", epicID, filename, err)
	}
	p, err := safePath(spikesDir, filename+".md")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write spike %s/%s: %w", epicID, filename, err)
	}
	v.autoCommit(fmt.Sprintf("update spike: %s/%s", epicID, filename),
		filepath.Join("epics", epicID, "spikes", filename+".md"))
	return nil
}

// DeleteSpike removes a spike file and auto-commits.
func (v *VaultManager) DeleteSpike(epicID, filename string) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	p, err := safePath(filepath.Join(dir, "spikes"), filename+".md")
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("epic %s: spike %s not found", epicID, filename)
		}
		return fmt.Errorf("delete spike %s/%s: %w", epicID, filename, err)
	}
	v.autoCommit(fmt.Sprintf("delete spike: %s/%s", epicID, filename), "-A")
	return nil
}

// --- Specs ---

// ReadSpecsSummary reads specs/summary.md for an epic.
func (v *VaultManager) ReadSpecsSummary(epicID string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "specs", "summary.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("epic %s: specs/summary.md not found", epicID)
		}
		return "", fmt.Errorf("read specs summary %s: %w", epicID, err)
	}
	return string(data), nil
}

// WriteSpecsSummary writes specs/summary.md and auto-commits.
func (v *VaultManager) WriteSpecsSummary(epicID string, content string) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	specsDir := filepath.Join(dir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return fmt.Errorf("write specs summary %s: %w", epicID, err)
	}
	target := filepath.Join(specsDir, "summary.md")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write specs summary %s: %w", epicID, err)
	}
	v.autoCommit(fmt.Sprintf("update specs summary: %s", epicID),
		filepath.Join("epics", epicID, "specs", "summary.md"))
	return nil
}

// ReadSpecsChangelog reads specs/changelog.md for an epic.
func (v *VaultManager) ReadSpecsChangelog(epicID string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "specs", "changelog.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("epic %s: specs/changelog.md not found", epicID)
		}
		return "", fmt.Errorf("read specs changelog %s: %w", epicID, err)
	}
	return string(data), nil
}

// WriteSpecsChangelog writes specs/changelog.md and auto-commits.
func (v *VaultManager) WriteSpecsChangelog(epicID string, content string) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	specsDir := filepath.Join(dir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return fmt.Errorf("write specs changelog %s: %w", epicID, err)
	}
	target := filepath.Join(specsDir, "changelog.md")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write specs changelog %s: %w", epicID, err)
	}
	v.autoCommit(fmt.Sprintf("update specs changelog: %s", epicID),
		filepath.Join("epics", epicID, "specs", "changelog.md"))
	return nil
}

// ListExtracts returns extract names (without .extract.md suffix) for an epic.
func (v *VaultManager) ListExtracts(epicID string) ([]string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return nil, err
	}
	return listFilesWithExt(filepath.Join(dir, "specs", "extracts"), ".extract.md")
}

// ReadExtract reads a specific extract file.
func (v *VaultManager) ReadExtract(epicID, filename string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	p, err := safePath(filepath.Join(dir, "specs", "extracts"), filename+".extract.md")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("epic %s: extract %s not found", epicID, filename)
		}
		return "", fmt.Errorf("read extract %s/%s: %w", epicID, filename, err)
	}
	return string(data), nil
}

// RunExtract extracts structured markdown from a source file (xlsx/docx/pptx/xmind).
// sourcePath should be an absolute path to the uploaded file.
func (v *VaultManager) RunExtract(epicID, sourcePath string) (extractName string, extractContent string, err error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", "", err
	}
	specsDir := filepath.Join(dir, "specs")

	result, err := specextract.Extract(sourcePath, specsDir)
	if err != nil {
		return "", "", fmt.Errorf("extract %s: %w", filepath.Base(sourcePath), err)
	}

	// Auto-commit all spec changes (sources + extracts)
	v.autoCommit(fmt.Sprintf("extract spec: %s/%s", epicID, result.ExtractName),
		filepath.Join("epics", epicID, "specs"))

	return result.ExtractName, result.Content, nil
}

// --- Retro ---

// ListRetroReports lists retro report names for an epic.
func (v *VaultManager) ListRetroReports(epicID string) ([]string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return nil, err
	}
	return listMDFiles(filepath.Join(dir, "retro"))
}

// ReadRetroReport reads a retro report.
func (v *VaultManager) ReadRetroReport(epicID, name string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	path, err := safePath(filepath.Join(dir, "retro"), name+".md")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read retro report %s: %w", name, err)
	}
	return string(data), nil
}

// WriteRetroReport writes a retro report and auto-commits.
func (v *VaultManager) WriteRetroReport(epicID, name, content string) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	retroDir := filepath.Join(dir, "retro")
	if err := os.MkdirAll(retroDir, 0o755); err != nil {
		return fmt.Errorf("create retro dir: %w", err)
	}
	path, err := safePath(retroDir, name+".md")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write retro report %s: %w", name, err)
	}
	v.autoCommit(fmt.Sprintf("retro report: %s/%s", epicID, name), path)
	return nil
}

// WriteRetroReportJSON writes a JSON snapshot of the retro report's structured
// data alongside the markdown file. Used so the UI can build cross-release
// trend views without re-parsing markdown.
func (v *VaultManager) WriteRetroReportJSON(epicID, name string, data []byte) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	retroDir := filepath.Join(dir, "retro")
	if err := os.MkdirAll(retroDir, 0o755); err != nil {
		return fmt.Errorf("create retro dir: %w", err)
	}
	path, err := safePath(retroDir, name+".json")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write retro snapshot %s: %w", name, err)
	}
	v.autoCommit(fmt.Sprintf("retro snapshot: %s/%s", epicID, name), path)
	return nil
}

// ReadRetroReportJSON reads the JSON snapshot. Returns os.ErrNotExist when
// the snapshot is missing (e.g. legacy reports produced before snapshots
// were introduced), so callers can distinguish "missing" from other errors.
func (v *VaultManager) ReadRetroReportJSON(epicID, name string) ([]byte, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return nil, err
	}
	path, err := safePath(filepath.Join(dir, "retro"), name+".json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("read retro snapshot %s: %w", name, err)
	}
	return data, nil
}

// ReadRetroActions reads the retro actions.yaml for an epic.
func (v *VaultManager) ReadRetroActions(epicID string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "retro", "actions.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read retro actions: %w", err)
	}
	return string(data), nil
}

// WriteRetroActions writes the retro actions.yaml and auto-commits.
func (v *VaultManager) WriteRetroActions(epicID, content string) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	retroDir := filepath.Join(dir, "retro")
	if err := os.MkdirAll(retroDir, 0o755); err != nil {
		return fmt.Errorf("create retro dir: %w", err)
	}
	path := filepath.Join(retroDir, "actions.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write retro actions: %w", err)
	}
	v.autoCommit(fmt.Sprintf("update retro actions: %s", epicID), path)
	return nil
}

// AppendRetroDecision appends one JSON entry to retro/decisions.jsonl.
// entry must be a valid JSON object on a single line (newlines are rejected).
func (v *VaultManager) AppendRetroDecision(epicID string, entry []byte) error {
	trimmed := strings.TrimSpace(string(entry))
	if trimmed == "" {
		return fmt.Errorf("decision entry is empty")
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return fmt.Errorf("decision entry must be a single line")
	}
	if !json.Valid([]byte(trimmed)) {
		return fmt.Errorf("decision entry invalid json")
	}
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	retroDir := filepath.Join(dir, "retro")
	if err := os.MkdirAll(retroDir, 0o755); err != nil {
		return fmt.Errorf("create retro dir: %w", err)
	}
	path := filepath.Join(retroDir, "decisions.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open retro decisions: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(trimmed + "\n"); err != nil {
		return fmt.Errorf("append retro decision: %w", err)
	}
	v.autoCommit(fmt.Sprintf("retro decision: %s", epicID), path)
	return nil
}

// ReadRetroDecisions reads retro/decisions.jsonl as raw text.
// Returns empty string if the file does not exist.
func (v *VaultManager) ReadRetroDecisions(epicID string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "retro", "decisions.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read retro decisions: %w", err)
	}
	return string(data), nil
}

// --- Contracts ---

// ListContracts returns contract IDs (without .yaml extension) for an epic.
func (v *VaultManager) ListContracts(epicID string) ([]string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return nil, err
	}
	return listFilesWithExt(filepath.Join(dir, "contracts"), ".yaml")
}

// ListContractsByOwner returns contract IDs whose owners include the given owner.
// Malformed YAML is treated as non-fatal (logged and skipped) so one hand-edited
// contract does not block reverse lookups used by /breakdown and /verify.
func (v *VaultManager) ListContractsByOwner(epicID, owner string) ([]string, error) {
	if owner == "" {
		return nil, fmt.Errorf("list contracts by owner: owner is empty")
	}
	ids, err := v.ListContracts(epicID)
	if err != nil {
		return nil, err
	}
	var matched []string
	for _, id := range ids {
		content, err := v.ReadContract(epicID, id)
		if err != nil {
			return nil, fmt.Errorf("list contracts by owner %s/%s: %w", epicID, id, err)
		}
		c, err := ParseContract([]byte(content))
		if err != nil {
			log.Printf("WARNING: vault parse contract %s/%s: %v", epicID, id, err)
			continue
		}
		for _, o := range c.Owners {
			if o == owner {
				matched = append(matched, id)
				break
			}
		}
	}
	return matched, nil
}

// ReadContract reads a specific contract file.
func (v *VaultManager) ReadContract(epicID, id string) (string, error) {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return "", err
	}
	p, err := safePath(filepath.Join(dir, "contracts"), id+".yaml")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("epic %s: contract %s not found", epicID, id)
		}
		return "", fmt.Errorf("read contract %s/%s: %w", epicID, id, err)
	}
	return string(data), nil
}

// WriteContract writes a contract file with validation and auto-commit.
func (v *VaultManager) WriteContract(epicID, id, content string) error {
	if err := ValidateContract(content); err != nil {
		return err
	}
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	contractsDir := filepath.Join(dir, "contracts")
	if err := os.MkdirAll(contractsDir, 0o755); err != nil {
		return fmt.Errorf("write contract %s/%s: %w", epicID, id, err)
	}
	p, err := safePath(contractsDir, id+".yaml")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write contract %s/%s: %w", epicID, id, err)
	}
	v.autoCommit(fmt.Sprintf("update contract: %s/%s", epicID, id),
		filepath.Join("epics", epicID, "contracts", id+".yaml"))
	return nil
}

// Warning codes on GeneratedArtifact.Warnings[].Code.
const (
	// WarnStaleManifest signals that the target directory does not exist —
	// the manifest's test/types pattern likely points at a renamed location.
	WarnStaleManifest = "stale_manifest"
)

// ArtifactWarning is a structured warning attached to a GeneratedArtifact.
// Consumers can branch on Code (stable) rather than scanning Message (i18n).
type ArtifactWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GeneratedArtifact describes a single file produced by GenerateContract.
type GeneratedArtifact struct {
	Project  string            `json:"project"`
	Format   string            `json:"format"`
	Path     string            `json:"path"`
	Content  string            `json:"content"`
	Written  bool              `json:"written"`
	Warnings []ArtifactWarning `json:"warnings,omitempty"`
}

// GenerateOptions controls how GenerateContract writes artifacts.
type GenerateOptions struct {
	DryRun    bool
	Overwrite bool
}

// ProjectResolver resolves a project name (as referenced in contract.generates[].project)
// to an absolute filesystem root. Returns ok=false if the project is unknown.
type ProjectResolver func(name string) (root string, ok bool)

// resolveRoot calls the resolver defensively: returns ok=false when the
// resolver is absent or returns an empty/unknown project root.
func resolveRoot(resolve ProjectResolver, project string) (string, bool) {
	if resolve == nil {
		return "", false
	}
	root, ok := resolve(project)
	if !ok || root == "" {
		return "", false
	}
	return root, true
}

// GenerateContract reads a contract and produces the configured target files.
// When opts.DryRun is true, nothing is written — content is returned for preview.
// When opts.Overwrite is false, existing files are skipped (Written=false).
func (v *VaultManager) GenerateContract(epicID, id string, resolve ProjectResolver, opts GenerateOptions) ([]GeneratedArtifact, error) {
	raw, err := v.ReadContract(epicID, id)
	if err != nil {
		return nil, err
	}
	c, err := ParseContract([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("generate contract %s/%s: %w", epicID, id, err)
	}
	if len(c.Generates) == 0 {
		return []GeneratedArtifact{}, nil
	}

	artifacts := make([]GeneratedArtifact, 0, len(c.Generates))
	for _, target := range c.Generates {
		var content string
		var genErr error
		switch target.Format {
		case "go-httptest":
			content, genErr = GenerateGoTest(c, target.Path)
		case "ts-types":
			content, genErr = GenerateTSTypes(c)
		default:
			return nil, fmt.Errorf("generate contract %s/%s: unknown format %q", epicID, id, target.Format)
		}
		if genErr != nil {
			return nil, genErr
		}

		art := GeneratedArtifact{
			Project: target.Project,
			Format:  target.Format,
			Path:    target.Path,
			Content: content,
		}

		// Stale-manifest detection: a missing parent dir usually means
		// manifest.contracts.{test,types}_pattern points at a renamed location.
		// Checked in dry-run too — stat is side-effect free.
		if root, ok := resolveRoot(resolve, target.Project); ok {
			parent := filepath.Dir(filepath.Join(root, target.Path))
			if _, statErr := os.Stat(parent); os.IsNotExist(statErr) {
				art.Warnings = append(art.Warnings, ArtifactWarning{
					Code: WarnStaleManifest,
					Message: fmt.Sprintf(
						"target directory does not exist: %s — manifest may be stale, consider re-running /manifest",
						filepath.Dir(target.Path)),
				})
			}
		}

		if opts.DryRun {
			artifacts = append(artifacts, art)
			continue
		}

		if resolve == nil {
			return nil, fmt.Errorf("generate contract %s/%s: project resolver required when not in dry-run", epicID, id)
		}
		root, ok := resolve(target.Project)
		if !ok || root == "" {
			return nil, fmt.Errorf("generate contract %s/%s: project %q root not configured", epicID, id, target.Project)
		}
		absPath := filepath.Join(root, target.Path)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, fmt.Errorf("generate contract %s/%s: %w", epicID, id, err)
		}

		// Atomic create-or-skip when overwrite is false: O_EXCL avoids the
		// Stat→Write TOCTOU race.
		flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if !opts.Overwrite {
			flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
		}
		f, err := os.OpenFile(absPath, flag, 0o644)
		if err != nil {
			if !opts.Overwrite && os.IsExist(err) {
				artifacts = append(artifacts, art)
				continue
			}
			return nil, fmt.Errorf("generate contract %s/%s: %w", epicID, id, err)
		}
		if _, err := f.WriteString(content); err != nil {
			f.Close()
			return nil, fmt.Errorf("generate contract %s/%s: %w", epicID, id, err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("generate contract %s/%s: %w", epicID, id, err)
		}
		art.Written = true
		artifacts = append(artifacts, art)
	}
	return artifacts, nil
}

// DeleteContract removes a contract file and auto-commits.
func (v *VaultManager) DeleteContract(epicID, id string) error {
	dir, err := v.epicDir(epicID)
	if err != nil {
		return err
	}
	p, err := safePath(filepath.Join(dir, "contracts"), id+".yaml")
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("epic %s: contract %s not found", epicID, id)
		}
		return fmt.Errorf("delete contract %s/%s: %w", epicID, id, err)
	}
	v.autoCommit(fmt.Sprintf("delete contract: %s/%s", epicID, id), "-A")
	return nil
}
