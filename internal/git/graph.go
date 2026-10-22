package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ChangedFile represents a file changed in a commit.
type ChangedFile struct {
	Status string `json:"status"` // "A", "M", "D", "R"
	Path   string `json:"path"`
}

// CommitStats holds aggregated line-change statistics for a commit.
type CommitStats struct {
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

// CommitNode represents a single commit with layout information.
type CommitNode struct {
	Hash       string        `json:"hash"`
	Subject    string        `json:"subject"`
	AuthorName string        `json:"authorName"`
	RelDate    string        `json:"relDate"`
	Refs       []string      `json:"refs"`
	IsHead     bool          `json:"isHead"`
	Column     int           `json:"column"`
	Color      int           `json:"color"`
	Parents    []string      `json:"parents"`
	Files      []ChangedFile `json:"files"`
	Stats      CommitStats   `json:"stats"`
}

// CommitDetail holds the full message body of a single commit.
type CommitDetail struct {
	Hash string `json:"hash"`
	Body string `json:"body"`
}

// Edge represents a connection between two commits in the graph.
type Edge struct {
	FromHash   string `json:"fromHash"`
	ToHash     string `json:"toHash"`
	FromColumn int    `json:"fromCol"`
	ToColumn   int    `json:"toCol"`
	Color      int    `json:"color"`
}

// GraphResult is the API response for the git graph endpoint.
type GraphResult struct {
	IsRepo  bool         `json:"isRepo"`
	Branch  string       `json:"branch"`
	Commits []CommitNode `json:"commits"`
	Edges   []Edge       `json:"edges"`
	MaxCol  int          `json:"maxCol"`
	Remotes []string     `json:"remotes"`
	Ahead   int          `json:"ahead"`
	Behind  int          `json:"behind"`
	HasMore bool         `json:"hasMore"`
}

// BranchInfo represents a single branch.
type BranchInfo struct {
	Name      string `json:"name"`
	IsCurrent bool   `json:"isCurrent"`
	IsRemote  bool   `json:"isRemote"`
}

// BranchListResult is the API response for the branches endpoint.
type BranchListResult struct {
	Current  string       `json:"current"`
	Branches []BranchInfo `json:"branches"`
}

// rawCommit holds parsed but not yet laid-out commit data.
type rawCommit struct {
	Hash       string
	Subject    string
	AuthorName string
	RelDate    string
	Refs       []string
	IsHead     bool
	Parents    []string
}

// GraphFetcher retrieves and processes git graph data.
type GraphFetcher struct {
	cmd Commander
}

// NewGraphFetcher creates a GraphFetcher with the given Commander.
func NewGraphFetcher(cmd Commander) *GraphFetcher {
	return &GraphFetcher{cmd: cmd}
}

// Available returns true if the git executable is found in PATH.
func (g *GraphFetcher) Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// GetBranches returns all local and remote branches for the given directory.
func (g *GraphFetcher) GetBranches(dir string) (BranchListResult, error) {
	// Check if it's a git repo
	_, err := g.cmd.Output("git", "-C", dir, "rev-parse", "--git-dir")
	if err != nil {
		return BranchListResult{Branches: []BranchInfo{}}, nil
	}

	result := BranchListResult{Branches: []BranchInfo{}}

	// Get current branch
	branchOut, err := g.cmd.Output("git", "-C", dir, "branch", "--show-current")
	if err == nil {
		result.Current = strings.TrimSpace(string(branchOut))
	}

	// Get all local branches
	localOut, err := g.cmd.Output("git", "-C", dir, "branch", "--format=%(refname:short)")
	if err == nil {
		for _, name := range strings.Split(strings.TrimRight(string(localOut), "\n"), "\n") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			result.Branches = append(result.Branches, BranchInfo{
				Name:      name,
				IsCurrent: name == result.Current,
			})
		}
	}

	// Get remote branches
	remoteOut, err := g.cmd.Output("git", "-C", dir, "branch", "-r", "--format=%(refname:short)")
	if err == nil {
		for _, name := range strings.Split(strings.TrimRight(string(remoteOut), "\n"), "\n") {
			name = strings.TrimSpace(name)
			if name == "" || strings.Contains(name, "HEAD") {
				continue
			}
			result.Branches = append(result.Branches, BranchInfo{
				Name:     name,
				IsRemote: true,
			})
		}
	}

	return result, nil
}

// GetGraph returns the commit graph for the given directory.
// If ref is non-empty, only show commits reachable from that ref.
func (g *GraphFetcher) GetGraph(dir string, limit int) (GraphResult, error) {
	return g.GetGraphRef(dir, limit, "")
}

// GetGraphRef returns the commit graph filtered by an optional ref.
// hideRemotes is a list of remote names to exclude from the graph.
func (g *GraphFetcher) GetGraphRef(dir string, limit int, ref string, hideRemotes ...string) (GraphResult, error) {
	// Check if it's a git repo
	_, err := g.cmd.Output("git", "-C", dir, "rev-parse", "--git-dir")
	if err != nil {
		return GraphResult{
			IsRepo:  false,
			Commits: []CommitNode{},
			Edges:   []Edge{},
			Remotes: []string{},
		}, nil
	}

	result := GraphResult{
		IsRepo:  true,
		Commits: []CommitNode{},
		Edges:   []Edge{},
		Remotes: []string{},
	}

	// Get current branch
	branchOut, err := g.cmd.Output("git", "-C", dir, "branch", "--show-current")
	if err == nil {
		result.Branch = strings.TrimSpace(string(branchOut))
	}

	// Get remotes
	remotesOut, err := g.cmd.Output("git", "-C", dir, "remote")
	if err == nil {
		result.Remotes = parseRemotes(string(remotesOut))
	}

	// Get ahead/behind relative to upstream
	abOut, err := g.cmd.Output("git", "-C", dir, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err == nil {
		parts := strings.Fields(strings.TrimSpace(string(abOut)))
		if len(parts) == 2 {
			if n, e := strconv.Atoi(parts[0]); e == nil {
				result.Ahead = n
			}
			if n, e := strconv.Atoi(parts[1]); e == nil {
				result.Behind = n
			}
		}
	} else if len(result.Remotes) > 0 && result.Branch != "" {
		// No upstream tracking branch but remote exists — count all local commits
		// not yet on the remote's matching branch as ahead.
		remote := result.Remotes[0]
		remoteBranch := remote + "/" + result.Branch
		countOut, countErr := g.cmd.Output("git", "-C", dir, "rev-list", "--count", remoteBranch+"..HEAD")
		if countErr == nil {
			if n, e := strconv.Atoi(strings.TrimSpace(string(countOut))); e == nil {
				result.Ahead = n
			}
		} else {
			// Remote branch doesn't exist at all — all local commits are ahead
			countOut, countErr = g.cmd.Output("git", "-C", dir, "rev-list", "--count", "HEAD")
			if countErr == nil {
				if n, e := strconv.Atoi(strings.TrimSpace(string(countOut))); e == nil {
					result.Ahead = n
				}
			}
		}
	}

	// Get commit log
	if limit <= 0 {
		limit = 50
	}

	// When a specific ref is requested, show only that ref's history;
	// otherwise show all local branches + tags + primary remote.
	var refArgs []string
	if ref != "" {
		refArgs = []string{ref}
	} else {
		refArgs = buildRefArgs(result.Remotes, hideRemotes...)
	}

	format := "%h%x00%s%x00%an%x00%cr%x00%D%x00%p"
	logArgs := append([]string{"-C", dir, "log"}, refArgs...)
	logArgs = append(logArgs,
		fmt.Sprintf("--format=%s", format),
		"--topo-order",
		fmt.Sprintf("-n%d", limit+1))
	logOut, err := g.cmd.Output("git", logArgs...)
	if err != nil {
		// Empty repo (no commits yet)
		return result, nil
	}

	raw := parseCommits(string(logOut))
	if len(raw) == 0 {
		return result, nil
	}

	if len(raw) > limit {
		result.HasMore = true
		raw = raw[:limit]
	}

	nodes, maxCol := assignColumns(raw)
	edges := buildEdges(nodes)

	// Get changed files per commit
	nsArgs := append([]string{"-C", dir, "log"}, refArgs...)
	nsArgs = append(nsArgs,
		"--format=%h",
		"--name-status",
		"--topo-order",
		fmt.Sprintf("-n%d", limit))
	nameStatusOut, err := g.cmd.Output("git", nsArgs...)
	if err == nil {
		fileMap := parseNameStatus(string(nameStatusOut))
		for i := range nodes {
			if files, ok := fileMap[nodes[i].Hash]; ok {
				nodes[i].Files = files
			} else {
				nodes[i].Files = []ChangedFile{}
			}
		}
	}

	// Get line-change stats per commit
	numArgs := append([]string{"-C", dir, "log"}, refArgs...)
	numArgs = append(numArgs,
		"--format=%h",
		"--numstat",
		"--topo-order",
		fmt.Sprintf("-n%d", limit))
	numstatOut, err := g.cmd.Output("git", numArgs...)
	if err == nil {
		statsMap := parseNumstat(string(numstatOut))
		for i := range nodes {
			if s, ok := statsMap[nodes[i].Hash]; ok {
				nodes[i].Stats = s
			}
		}
	}

	result.Commits = nodes
	result.Edges = edges
	result.MaxCol = maxCol

	return result, nil
}

// buildRefArgs returns git log ref-filter arguments.
// Shows local branches, tags, and all remotes not in hideRemotes.
// When no remotes are hidden, uses --remotes (all).
// When some are hidden, uses --glob=refs/remotes/<name> for each visible remote.
func buildRefArgs(remotes []string, hideRemotes ...string) []string {
	args := []string{"--branches", "--tags"}

	if len(remotes) == 0 {
		return args
	}

	// Build hide set
	hideSet := make(map[string]bool, len(hideRemotes))
	for _, r := range hideRemotes {
		hideSet[r] = true
	}

	// Collect visible remotes
	var visible []string
	for _, r := range remotes {
		if !hideSet[r] {
			visible = append(visible, r)
		}
	}

	if len(visible) == len(remotes) {
		// No filtering — show all remotes
		args = append(args, "--remotes")
	} else {
		// Selective: add --glob for each visible remote
		for _, r := range visible {
			args = append(args, fmt.Sprintf("--glob=refs/remotes/%s", r))
		}
	}

	return args
}

// parseNameStatus parses `git log --format=%h --name-status` output into
// a map of hash → []ChangedFile.
func parseNameStatus(output string) map[string][]ChangedFile {
	result := make(map[string][]ChangedFile)
	if strings.TrimSpace(output) == "" {
		return result
	}
	lines := strings.Split(output, "\n")
	var currentHash string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		// A line with a tab is a file entry (e.g. "M\tpath/to/file")
		if strings.Contains(line, "\t") {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 && currentHash != "" {
				status := parts[0]
				path := parts[1]
				// Rename: "R100\told\tnew" — take the new path
				if strings.HasPrefix(status, "R") {
					status = "R"
					if sub := strings.SplitN(parts[1], "\t", 2); len(sub) == 2 {
						path = sub[1]
					}
				}
				result[currentHash] = append(result[currentHash], ChangedFile{
					Status: status,
					Path:   path,
				})
			}
		} else {
			// This is a commit hash line
			currentHash = strings.TrimSpace(line)
		}
	}
	return result
}

// GetCommitBody returns the body (excluding subject) of a single commit.
func (g *GraphFetcher) GetCommitBody(dir, hash string) (CommitDetail, error) {
	out, err := g.cmd.Output("git", "-C", dir, "log", "-1", "--format=%b", hash)
	if err != nil {
		return CommitDetail{}, fmt.Errorf("get commit body %s: %w", hash, err)
	}
	body := strings.TrimSpace(string(out))
	return CommitDetail{Hash: hash, Body: body}, nil
}

// parseNumstat parses `git log --format=%h --numstat` output into
// a map of hash → CommitStats (aggregated insertions/deletions).
func parseNumstat(output string) map[string]CommitStats {
	result := make(map[string]CommitStats)
	if strings.TrimSpace(output) == "" {
		return result
	}
	lines := strings.Split(output, "\n")
	var currentHash string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		// A numstat line looks like: "10\t5\tpath/to/file" or "-\t-\tbinary"
		if strings.Contains(line, "\t") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) == 3 && currentHash != "" {
				add, errA := strconv.Atoi(parts[0])
				del, errD := strconv.Atoi(parts[1])
				if errA == nil && errD == nil {
					s := result[currentHash]
					s.Insertions += add
					s.Deletions += del
					result[currentHash] = s
				}
				// binary files ("-\t-\t...") are silently skipped
			}
		} else {
			currentHash = strings.TrimSpace(line)
		}
	}
	return result
}

// parseRemotes splits remote output into a list.
func parseRemotes(output string) []string {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	var result []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}

// parseCommits parses the git log output into rawCommit structs.
func parseCommits(output string) []rawCommit {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	var commits []rawCommit
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: hash\x00subject\x00authorName\x00relDate\x00refs\x00parents
		parts := strings.SplitN(line, "\x00", 6)
		if len(parts) < 6 {
			continue
		}
		c := rawCommit{
			Hash:       parts[0],
			Subject:    parts[1],
			AuthorName: parts[2],
			RelDate:    parts[3],
		}

		// Parse refs (e.g. "HEAD -> main, origin/main")
		if refs := strings.TrimSpace(parts[4]); refs != "" {
			for _, ref := range strings.Split(refs, ", ") {
				ref = strings.TrimSpace(ref)
				if ref == "" {
					continue
				}
				if strings.Contains(ref, "HEAD -> ") {
					c.IsHead = true
					ref = strings.TrimPrefix(ref, "HEAD -> ")
				} else if ref == "HEAD" {
					c.IsHead = true
					continue
				}
				c.Refs = append(c.Refs, ref)
			}
		}

		// Parse parents (space-separated short hashes)
		if parents := strings.TrimSpace(parts[5]); parents != "" {
			c.Parents = strings.Fields(parents)
		}

		commits = append(commits, c)
	}
	return commits
}

// assignColumns assigns a column (lane) and color to each commit.
// Returns the laid-out CommitNodes and the maximum column used.
func assignColumns(raw []rawCommit) ([]CommitNode, int) {
	if len(raw) == 0 {
		return nil, 0
	}

	// hashToCol maps a commit hash to its assigned column
	hashToCol := make(map[string]int)
	// hashToColor maps a commit hash to its color index
	hashToColor := make(map[string]int)
	// activeLanes tracks which columns are in use; value is the expected next hash
	activeLanes := make(map[int]string)
	maxCol := 0
	nextColor := 0

	nodes := make([]CommitNode, 0, len(raw))

	for _, rc := range raw {
		col, hasCol := hashToCol[rc.Hash]
		color, hasColor := hashToColor[rc.Hash]

		if !hasCol {
			// Find the first free column
			col = findFreeColumn(activeLanes)
		}
		if !hasColor {
			color = nextColor % 8
			nextColor++
		}

		if col > maxCol {
			maxCol = col
		}

		// This commit now occupies this column
		// Remove old lane reservation if it pointed to this hash
		for c, h := range activeLanes {
			if h == rc.Hash {
				delete(activeLanes, c)
			}
		}

		refs := rc.Refs
		if refs == nil {
			refs = []string{}
		}
		parents := rc.Parents
		if parents == nil {
			parents = []string{}
		}
		node := CommitNode{
			Hash:       rc.Hash,
			Subject:    rc.Subject,
			AuthorName: rc.AuthorName,
			RelDate:    rc.RelDate,
			Refs:       refs,
			IsHead:     rc.IsHead,
			Column:     col,
			Color:      color,
			Parents:    parents,
		}
		nodes = append(nodes, node)

		// Assign columns to parents
		if len(rc.Parents) > 0 {
			// First parent (direct ancestor) inherits this column
			firstParent := rc.Parents[0]
			if _, already := hashToCol[firstParent]; !already {
				hashToCol[firstParent] = col
				hashToColor[firstParent] = color
				activeLanes[col] = firstParent
			}

			// Additional parents (merge sources) get new columns
			for i := 1; i < len(rc.Parents); i++ {
				p := rc.Parents[i]
				if _, already := hashToCol[p]; !already {
					newCol := findFreeColumn(activeLanes)
					hashToCol[p] = newCol
					mergeColor := nextColor % 8
					nextColor++
					hashToColor[p] = mergeColor
					activeLanes[newCol] = p
					if newCol > maxCol {
						maxCol = newCol
					}
				}
			}
		}
	}

	return nodes, maxCol
}

// findFreeColumn returns the lowest column index not in use.
func findFreeColumn(activeLanes map[int]string) int {
	col := 0
	for {
		if _, used := activeLanes[col]; !used {
			return col
		}
		col++
	}
}

// buildEdges generates edges between commits based on parent relationships.
func buildEdges(nodes []CommitNode) []Edge {
	if len(nodes) == 0 {
		return nil
	}

	// Build hash → node index map
	hashToNode := make(map[string]int, len(nodes))
	for i, n := range nodes {
		hashToNode[n.Hash] = i
	}

	var edges []Edge
	for _, node := range nodes {
		for _, parentHash := range node.Parents {
			parentIdx, ok := hashToNode[parentHash]
			if !ok {
				// Parent not in visible range
				continue
			}
			parent := nodes[parentIdx]
			edges = append(edges, Edge{
				FromHash:   node.Hash,
				ToHash:     parentHash,
				FromColumn: node.Column,
				ToColumn:   parent.Column,
				Color:      parent.Color,
			})
		}
	}

	return edges
}
