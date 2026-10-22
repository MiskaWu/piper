// Git 相關型別

export interface GitChangedFile {
  status: string  // "A", "M", "D", "R"
  path: string
}

export interface GitCommitStats {
  insertions: number
  deletions: number
}

export interface GitCommitNode {
  hash: string
  subject: string
  authorName: string
  relDate: string
  refs: string[]
  isHead: boolean
  column: number
  color: number
  parents: string[]
  files: GitChangedFile[]
  stats: GitCommitStats
}

export interface GitEdge {
  fromHash: string
  toHash: string
  fromCol: number
  toCol: number
  color: number
}

export interface GitGraphResult {
  isRepo: boolean
  branch: string
  commits: GitCommitNode[]
  edges: GitEdge[]
  maxCol: number
  remotes: string[]
  ahead: number
  behind: number
  hasMore: boolean
}

export interface GitBranchInfo {
  name: string
  isCurrent: boolean
  isRemote: boolean
}

export interface GitBranchListResult {
  current: string
  branches: GitBranchInfo[]
}

export interface SubmoduleInfo {
  path: string
  ahead: number
}

export interface GitFileStatus {
  path: string
  status: string // M, A, D, ?, R
}
