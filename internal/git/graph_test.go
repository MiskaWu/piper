package git

import (
	"errors"
	"strings"
	"testing"
)

func TestAvailable_回傳git是否存在(t *testing.T) {
	mock := newGraphMock()
	fetcher := NewGraphFetcher(mock)
	// In test environment git should be available (or not), just verify no panic
	got := fetcher.Available()
	// Verify it returns a bool (true in CI/dev where git is installed)
	if !got {
		t.Log("git not found in PATH — Available() correctly returned false")
	}
}

func TestParseCommits_基本解析(t *testing.T) {
	output := "abc1234\x00feat: add login\x00Alice\x002 hours ago\x00HEAD -> main\x00def5678\n"
	commits := parseCommits(output)
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	c := commits[0]
	if c.Hash != "abc1234" {
		t.Errorf("hash = %q, want %q", c.Hash, "abc1234")
	}
	if c.Subject != "feat: add login" {
		t.Errorf("subject = %q, want %q", c.Subject, "feat: add login")
	}
	if c.AuthorName != "Alice" {
		t.Errorf("authorName = %q, want %q", c.AuthorName, "Alice")
	}
	if c.RelDate != "2 hours ago" {
		t.Errorf("relDate = %q, want %q", c.RelDate, "2 hours ago")
	}
	if !c.IsHead {
		t.Error("expected IsHead = true")
	}
	if len(c.Refs) != 1 || c.Refs[0] != "main" {
		t.Errorf("refs = %v, want [main]", c.Refs)
	}
	if len(c.Parents) != 1 || c.Parents[0] != "def5678" {
		t.Errorf("parents = %v, want [def5678]", c.Parents)
	}
}

func TestParseCommits_多條記錄(t *testing.T) {
	output := "aaa\x00msg1\x00Bob\x001 day ago\x00\x00bbb\nbbb\x00msg2\x00Carol\x002 days ago\x00origin/main\x00\n"
	commits := parseCommits(output)
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].Hash != "aaa" {
		t.Errorf("first hash = %q", commits[0].Hash)
	}
	if commits[1].Hash != "bbb" {
		t.Errorf("second hash = %q", commits[1].Hash)
	}
	// Second commit has no parents (root)
	if len(commits[1].Parents) != 0 {
		t.Errorf("root commit should have no parents, got %v", commits[1].Parents)
	}
}

func TestParseCommits_合併Commit有多個Parent(t *testing.T) {
	output := "m1\x00merge branch\x00Dev\x00now\x00HEAD -> main\x00p1 p2\n"
	commits := parseCommits(output)
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if len(commits[0].Parents) != 2 {
		t.Fatalf("expected 2 parents, got %d", len(commits[0].Parents))
	}
	if commits[0].Parents[0] != "p1" || commits[0].Parents[1] != "p2" {
		t.Errorf("parents = %v, want [p1, p2]", commits[0].Parents)
	}
}

func TestParseCommits_空輸出(t *testing.T) {
	commits := parseCommits("")
	if commits != nil {
		t.Errorf("expected nil, got %v", commits)
	}
}

func TestParseCommits_Subject含有管線符號(t *testing.T) {
	output := "abc1234\x00feat: foo | bar\x00Alice\x002 hours ago\x00HEAD -> main\x00def5678\n"
	commits := parseCommits(output)
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	c := commits[0]
	if c.Subject != "feat: foo | bar" {
		t.Errorf("subject = %q, want %q", c.Subject, "feat: foo | bar")
	}
	if c.AuthorName != "Alice" {
		t.Errorf("authorName = %q, want %q", c.AuthorName, "Alice")
	}
	if !c.IsHead {
		t.Error("expected IsHead = true")
	}
	if len(c.Parents) != 1 || c.Parents[0] != "def5678" {
		t.Errorf("parents = %v, want [def5678]", c.Parents)
	}
}

func TestParseCommits_HEAD無分支(t *testing.T) {
	output := "abc\x00detached\x00Dev\x00now\x00HEAD\x00\n"
	commits := parseCommits(output)
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if !commits[0].IsHead {
		t.Error("expected IsHead = true for detached HEAD")
	}
}

func TestAssignColumns_線性歷史(t *testing.T) {
	raw := []rawCommit{
		{Hash: "c3", Parents: []string{"c2"}},
		{Hash: "c2", Parents: []string{"c1"}},
		{Hash: "c1"},
	}
	nodes, maxCol := assignColumns(raw)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
	// All should be in column 0
	for _, n := range nodes {
		if n.Column != 0 {
			t.Errorf("commit %s column = %d, want 0", n.Hash, n.Column)
		}
	}
	if maxCol != 0 {
		t.Errorf("maxCol = %d, want 0", maxCol)
	}
	// All same color
	if nodes[0].Color != nodes[1].Color || nodes[1].Color != nodes[2].Color {
		t.Error("linear history should have same color for all commits")
	}
}

func TestAssignColumns_分支合併(t *testing.T) {
	// c1 merges c2 and c3; c2 is on main, c3 is from a branch
	raw := []rawCommit{
		{Hash: "c1", Parents: []string{"c2", "c3"}}, // merge commit
		{Hash: "c2", Parents: []string{"c4"}},
		{Hash: "c3", Parents: []string{"c4"}},
		{Hash: "c4"},
	}
	nodes, maxCol := assignColumns(raw)
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(nodes))
	}
	// c1 and c2 should be in column 0 (main line)
	if nodes[0].Column != 0 {
		t.Errorf("c1 column = %d, want 0", nodes[0].Column)
	}
	if nodes[1].Column != 0 {
		t.Errorf("c2 column = %d, want 0", nodes[1].Column)
	}
	// c3 should be in a different column
	if nodes[2].Column == 0 {
		t.Error("c3 should be in a different column from main line")
	}
	if maxCol < 1 {
		t.Errorf("maxCol = %d, want >= 1", maxCol)
	}
}

func TestAssignColumns_多分支(t *testing.T) {
	// Two merge commits creating multiple lanes
	raw := []rawCommit{
		{Hash: "m1", Parents: []string{"a1", "b1"}},
		{Hash: "a1", Parents: []string{"root"}},
		{Hash: "b1", Parents: []string{"root"}},
		{Hash: "root"},
	}
	nodes, maxCol := assignColumns(raw)
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(nodes))
	}
	if maxCol < 1 {
		t.Errorf("maxCol = %d, want >= 1 for multi-branch", maxCol)
	}
	// Ensure b1 is in a different column than a1
	a1Col := -1
	b1Col := -1
	for _, n := range nodes {
		if n.Hash == "a1" {
			a1Col = n.Column
		}
		if n.Hash == "b1" {
			b1Col = n.Column
		}
	}
	if a1Col == b1Col {
		t.Errorf("a1 and b1 should be in different columns, both in %d", a1Col)
	}
}

func TestAssignColumns_空輸入(t *testing.T) {
	nodes, maxCol := assignColumns(nil)
	if nodes != nil {
		t.Errorf("expected nil nodes, got %v", nodes)
	}
	if maxCol != 0 {
		t.Errorf("maxCol = %d, want 0", maxCol)
	}
}

func TestBuildEdges_直線連接(t *testing.T) {
	nodes := []CommitNode{
		{Hash: "c2", Column: 0, Color: 0, Parents: []string{"c1"}},
		{Hash: "c1", Column: 0, Color: 0},
	}
	edges := buildEdges(nodes)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	e := edges[0]
	if e.FromHash != "c2" || e.ToHash != "c1" {
		t.Errorf("edge = %s→%s, want c2→c1", e.FromHash, e.ToHash)
	}
	if e.FromColumn != 0 || e.ToColumn != 0 {
		t.Errorf("edge columns = %d→%d, want 0→0", e.FromColumn, e.ToColumn)
	}
}

func TestBuildEdges_跨欄連接(t *testing.T) {
	nodes := []CommitNode{
		{Hash: "m1", Column: 0, Color: 0, Parents: []string{"p1", "p2"}},
		{Hash: "p1", Column: 0, Color: 0},
		{Hash: "p2", Column: 1, Color: 1},
	}
	edges := buildEdges(nodes)
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}
	// Find the cross-column edge
	var crossEdge *Edge
	for i, e := range edges {
		if e.ToHash == "p2" {
			crossEdge = &edges[i]
		}
	}
	if crossEdge == nil {
		t.Fatal("cross-column edge not found")
	}
	if crossEdge.FromColumn != 0 || crossEdge.ToColumn != 1 {
		t.Errorf("cross edge columns = %d→%d, want 0→1", crossEdge.FromColumn, crossEdge.ToColumn)
	}
}

func TestBuildEdges_Parent不在範圍內(t *testing.T) {
	nodes := []CommitNode{
		{Hash: "c1", Column: 0, Color: 0, Parents: []string{"invisible"}},
	}
	edges := buildEdges(nodes)
	if len(edges) != 0 {
		t.Errorf("expected 0 edges when parent not visible, got %d", len(edges))
	}
}

func TestBuildEdges_空輸入(t *testing.T) {
	edges := buildEdges(nil)
	if edges != nil {
		t.Errorf("expected nil, got %v", edges)
	}
}

func TestBuildRefArgs_單一Remote(t *testing.T) {
	args := buildRefArgs([]string{"origin"})
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
	if args[2] != "--remotes" {
		t.Errorf("args[2] = %q, want --remotes", args[2])
	}
}

func TestBuildRefArgs_多Remote無隱藏時顯示全部(t *testing.T) {
	args := buildRefArgs([]string{"upstream", "origin", "fork"})
	found := false
	for _, a := range args {
		if a == "--remotes" {
			found = true
		}
	}
	if !found {
		t.Errorf("should include --remotes when no hide, got %v", args)
	}
}

func TestBuildRefArgs_隱藏部分Remote(t *testing.T) {
	args := buildRefArgs([]string{"origin", "upstream", "fork"}, "upstream")
	// Should have --branches, --tags, --glob=refs/remotes/origin, --glob=refs/remotes/fork
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}
	hasOrigin := false
	hasFork := false
	hasUpstream := false
	for _, a := range args {
		if a == "--glob=refs/remotes/origin" {
			hasOrigin = true
		}
		if a == "--glob=refs/remotes/fork" {
			hasFork = true
		}
		if a == "--glob=refs/remotes/upstream" {
			hasUpstream = true
		}
	}
	if !hasOrigin {
		t.Error("should include origin")
	}
	if !hasFork {
		t.Error("should include fork")
	}
	if hasUpstream {
		t.Error("should NOT include upstream (hidden)")
	}
}

func TestBuildRefArgs_隱藏全部Remote(t *testing.T) {
	args := buildRefArgs([]string{"origin", "upstream"}, "origin", "upstream")
	// Only --branches --tags, no remote args
	if len(args) != 2 {
		t.Fatalf("expected 2 args (--branches --tags), got %d: %v", len(args), args)
	}
}

func TestBuildRefArgs_無Remote(t *testing.T) {
	args := buildRefArgs(nil)
	if len(args) != 2 {
		t.Fatalf("expected 2 args (--branches --tags), got %d: %v", len(args), args)
	}
}

// graphMockCommander allows configuring different outputs per command.
type graphMockCommander struct {
	outputs map[string]struct {
		data []byte
		err  error
	}
}

func newGraphMock() *graphMockCommander {
	return &graphMockCommander{
		outputs: make(map[string]struct {
			data []byte
			err  error
		}),
	}
}

func (m *graphMockCommander) setOutput(key string, data string, err error) {
	m.outputs[key] = struct {
		data []byte
		err  error
	}{[]byte(data), err}
}

func (m *graphMockCommander) Output(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	// Try exact match first
	if out, ok := m.outputs[key]; ok {
		return out.data, out.err
	}
	// Try prefix match for commands with variable args
	for k, v := range m.outputs {
		if strings.HasPrefix(key, k) {
			return v.data, v.err
		}
	}
	return nil, nil
}

func (m *graphMockCommander) Run(name string, args ...string) error {
	key := name + " " + strings.Join(args, " ")
	if out, ok := m.outputs[key]; ok {
		return out.err
	}
	return nil
}

func TestGetGraph_正常倉庫(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo remote", "origin\n", nil)
	mock.setOutput("git -C /repo log", "abc\x00feat: hello\x00Alice\x001 hour ago\x00HEAD -> main\x00def\ndef\x00init\x00Bob\x002 hours ago\x00\x00\n", nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraph("/repo", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsRepo {
		t.Error("expected IsRepo = true")
	}
	if result.Branch != "main" {
		t.Errorf("branch = %q, want %q", result.Branch, "main")
	}
	if len(result.Remotes) != 1 || result.Remotes[0] != "origin" {
		t.Errorf("remotes = %v, want [origin]", result.Remotes)
	}
	if len(result.Commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(result.Commits))
	}
	if result.Commits[0].Hash != "abc" {
		t.Errorf("first commit hash = %q, want %q", result.Commits[0].Hash, "abc")
	}
	if !result.Commits[0].IsHead {
		t.Error("first commit should be HEAD")
	}
	if len(result.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(result.Edges))
	}
}

func TestGetGraph_非Git目錄(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /notrepo rev-parse --git-dir", "", errors.New("not a git repository"))

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraph("/notrepo", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsRepo {
		t.Error("expected IsRepo = false")
	}
}

func TestGetGraph_空倉庫無Commit(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /empty rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /empty branch --show-current", "\n", nil)
	mock.setOutput("git -C /empty remote", "", nil)
	mock.setOutput("git -C /empty log", "", errors.New("no commits"))

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraph("/empty", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsRepo {
		t.Error("expected IsRepo = true for empty repo")
	}
	if len(result.Commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(result.Commits))
	}
}

func TestGetGraph_無Upstream有Remote時用RemoteBranch算Ahead(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo remote", "origin\n", nil)
	// @{upstream} fails — no tracking branch
	mock.setOutput("git -C /repo rev-list --left-right --count HEAD...@{upstream}", "", errors.New("no upstream"))
	// Fallback: count commits ahead of origin/main
	mock.setOutput("git -C /repo rev-list --count origin/main..HEAD", "3\n", nil)
	mock.setOutput("git -C /repo log", "abc\x00feat: hello\x00Alice\x001 hour ago\x00HEAD -> main\x00\n", nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraph("/repo", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Ahead != 3 {
		t.Errorf("ahead = %d, want 3", result.Ahead)
	}
}

func TestGetGraph_無Upstream且RemoteBranch不存在時算全部Commit(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo remote", "origin\n", nil)
	// @{upstream} fails
	mock.setOutput("git -C /repo rev-list --left-right --count HEAD...@{upstream}", "", errors.New("no upstream"))
	// origin/main doesn't exist either
	mock.setOutput("git -C /repo rev-list --count origin/main..HEAD", "", errors.New("unknown revision"))
	// Fall back to counting all commits
	mock.setOutput("git -C /repo rev-list --count HEAD", "5\n", nil)
	mock.setOutput("git -C /repo log", "abc\x00init\x00Alice\x001 hour ago\x00HEAD -> main\x00\n", nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraph("/repo", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Ahead != 5 {
		t.Errorf("ahead = %d, want 5", result.Ahead)
	}
}

func TestParseNameStatus_解析檔案變更(t *testing.T) {
	output := "abc1234\n\nM\tfile1.go\nA\tnew_file.go\n\ndef5678\n\nD\told.go\nR100\told_name.go\tnew_name.go\n"
	result := parseNameStatus(output)

	if len(result["abc1234"]) != 2 {
		t.Fatalf("abc1234: expected 2 files, got %d", len(result["abc1234"]))
	}
	if result["abc1234"][0].Status != "M" || result["abc1234"][0].Path != "file1.go" {
		t.Errorf("abc1234[0] = %+v", result["abc1234"][0])
	}
	if result["abc1234"][1].Status != "A" || result["abc1234"][1].Path != "new_file.go" {
		t.Errorf("abc1234[1] = %+v", result["abc1234"][1])
	}

	if len(result["def5678"]) != 2 {
		t.Fatalf("def5678: expected 2 files, got %d", len(result["def5678"]))
	}
	if result["def5678"][0].Status != "D" || result["def5678"][0].Path != "old.go" {
		t.Errorf("def5678[0] = %+v", result["def5678"][0])
	}
	if result["def5678"][1].Status != "R" || result["def5678"][1].Path != "new_name.go" {
		t.Errorf("def5678[1] = %+v, want R new_name.go", result["def5678"][1])
	}
}

func TestParseNameStatus_空輸出(t *testing.T) {
	result := parseNameStatus("")
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestGetCommitBody_回傳Body內容(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo log -1 --format=%b abc1234",
		"這是 commit body 第一行\n第二行說明\n\nCo-Authored-By: Someone\n", nil)

	fetcher := NewGraphFetcher(mock)
	detail, err := fetcher.GetCommitBody("/repo", "abc1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Hash != "abc1234" {
		t.Errorf("hash = %q, want abc1234", detail.Hash)
	}
	if !strings.Contains(detail.Body, "commit body 第一行") {
		t.Errorf("body = %q, want to contain 'commit body 第一行'", detail.Body)
	}
}

func TestGetCommitBody_空Body(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo log -1 --format=%b abc1234", "\n", nil)

	fetcher := NewGraphFetcher(mock)
	detail, err := fetcher.GetCommitBody("/repo", "abc1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Body != "" {
		t.Errorf("body = %q, want empty", detail.Body)
	}
}

func TestParseNumstat_解析行數統計(t *testing.T) {
	output := "abc1234\n\n10\t5\tfile1.go\n3\t0\tnew_file.go\n\ndef5678\n\n0\t20\told.go\n-\t-\tbinary.png\n"
	result := parseNumstat(output)

	if s, ok := result["abc1234"]; !ok {
		t.Fatal("abc1234 not found")
	} else {
		if s.Insertions != 13 {
			t.Errorf("abc1234 insertions = %d, want 13", s.Insertions)
		}
		if s.Deletions != 5 {
			t.Errorf("abc1234 deletions = %d, want 5", s.Deletions)
		}
	}

	if s, ok := result["def5678"]; !ok {
		t.Fatal("def5678 not found")
	} else {
		if s.Insertions != 0 {
			t.Errorf("def5678 insertions = %d, want 0", s.Insertions)
		}
		if s.Deletions != 20 {
			t.Errorf("def5678 deletions = %d, want 20", s.Deletions)
		}
	}
}

func TestParseNumstat_空輸出(t *testing.T) {
	result := parseNumstat("")
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestParseNumstat_忽略Binary檔案(t *testing.T) {
	output := "abc1234\n\n-\t-\tbinary.png\n"
	result := parseNumstat(output)
	s := result["abc1234"]
	if s.Insertions != 0 || s.Deletions != 0 {
		t.Errorf("binary should be skipped, got +%d -%d", s.Insertions, s.Deletions)
	}
}

func TestParseRemotes_多個Remote(t *testing.T) {
	remotes := parseRemotes("origin\nupstream\n")
	if len(remotes) != 2 {
		t.Fatalf("expected 2 remotes, got %d", len(remotes))
	}
	if remotes[0] != "origin" || remotes[1] != "upstream" {
		t.Errorf("remotes = %v", remotes)
	}
}

func TestParseRemotes_空(t *testing.T) {
	remotes := parseRemotes("")
	if remotes != nil {
		t.Errorf("expected nil, got %v", remotes)
	}
}

func TestGetBranches_回傳本地與遠端分支(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo branch --format=%(refname:short)", "main\nfeature/login\nfix/bug\n", nil)
	mock.setOutput("git -C /repo branch -r --format=%(refname:short)", "origin/main\norigin/feature/login\norigin/HEAD\n", nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetBranches("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Current != "main" {
		t.Errorf("current = %q, want %q", result.Current, "main")
	}

	// 3 local + 2 remote (HEAD filtered out)
	if len(result.Branches) != 5 {
		t.Fatalf("expected 5 branches, got %d: %+v", len(result.Branches), result.Branches)
	}

	// Verify current branch is marked
	var mainBranch *BranchInfo
	for i, b := range result.Branches {
		if b.Name == "main" && !b.IsRemote {
			mainBranch = &result.Branches[i]
			break
		}
	}
	if mainBranch == nil {
		t.Fatal("main branch not found")
	}
	if !mainBranch.IsCurrent {
		t.Error("main branch should be marked as current")
	}

	// Verify remote branches are marked
	remoteCount := 0
	for _, b := range result.Branches {
		if b.IsRemote {
			remoteCount++
		}
	}
	if remoteCount != 2 {
		t.Errorf("expected 2 remote branches, got %d", remoteCount)
	}
}

func TestGetBranches_非Git目錄(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /notrepo rev-parse --git-dir", "", errors.New("not a git repository"))

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetBranches("/notrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Branches) != 0 {
		t.Errorf("expected empty branches, got %v", result.Branches)
	}
}

func TestGetGraphRef_指定分支只顯示該分支歷史(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo remote", "origin\n", nil)
	// When ref="feature/login", the log command should use "feature/login" instead of --branches --tags
	mock.setOutput("git -C /repo log feature/login", "abc\x00feat: login\x00Alice\x001 hour ago\x00feature/login\x00def\n", nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraphRef("/repo", 50, "feature/login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsRepo {
		t.Error("expected IsRepo = true")
	}
	if result.Branch != "main" {
		t.Errorf("branch should still report current branch = %q, want %q", result.Branch, "main")
	}
	if len(result.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(result.Commits))
	}
	if result.Commits[0].Hash != "abc" {
		t.Errorf("commit hash = %q, want %q", result.Commits[0].Hash, "abc")
	}
}

func TestGetGraphRef_空ref等同GetGraph(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo remote", "origin\n", nil)
	mock.setOutput("git -C /repo log", "abc\x00init\x00Bob\x001 day ago\x00HEAD -> main\x00\n", nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraphRef("/repo", 50, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(result.Commits))
	}
}

// makeCommitLines generates N fake git log lines in the expected format.
func makeCommitLines(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		hash := strings.Repeat(string(rune('a'+i%26)), 7)
		parent := ""
		if i < n-1 {
			parent = strings.Repeat(string(rune('a'+(i+1)%26)), 7)
		}
		sb.WriteString(hash + "\x00msg" + string(rune('0'+i%10)) + "\x00Author\x001 day ago\x00\x00" + parent + "\n")
	}
	return sb.String()
}

func TestGetGraph_HasMore_超過limit時回傳true(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo remote", "", nil)
	// limit=3, backend requests 4 (limit+1), mock returns 4 commits
	mock.setOutput("git -C /repo log", makeCommitLines(4), nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraph("/repo", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasMore {
		t.Error("expected HasMore = true when commits exceed limit")
	}
	if len(result.Commits) != 3 {
		t.Errorf("expected 3 commits returned, got %d", len(result.Commits))
	}
}

func TestGetGraph_HasMore_不足limit時回傳false(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo remote", "", nil)
	// limit=10, mock returns only 3 commits
	mock.setOutput("git -C /repo log", makeCommitLines(3), nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraph("/repo", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasMore {
		t.Error("expected HasMore = false when commits < limit")
	}
	if len(result.Commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(result.Commits))
	}
}

func TestGetGraph_HasMore_剛好等於limit時回傳false(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	mock.setOutput("git -C /repo branch --show-current", "main\n", nil)
	mock.setOutput("git -C /repo remote", "", nil)
	// limit=5, backend requests 6 (limit+1), mock returns exactly 5
	mock.setOutput("git -C /repo log", makeCommitLines(5), nil)

	fetcher := NewGraphFetcher(mock)
	result, err := fetcher.GetGraph("/repo", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasMore {
		t.Error("expected HasMore = false when commits == limit (boundary case)")
	}
	if len(result.Commits) != 5 {
		t.Errorf("expected 5 commits, got %d", len(result.Commits))
	}
}
