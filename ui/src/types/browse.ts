// 目錄瀏覽相關型別

export interface BrowseEntry {
  name: string
  path: string
}

export interface BrowseResult {
  current: string
  parent: string
  entries: BrowseEntry[]
}

// Files Tree types
export interface FileEntry {
  name: string
  path: string
  isDir: boolean
}

export interface FilesResult {
  current: string
  parent: string
  entries: FileEntry[]
}

// File copy/move operation types
export interface FileOpEntry {
  source: string
  destination: string
}

export interface FileOpResult {
  entries: FileOpEntry[]
}

export type FileClipboardOp = 'copy' | 'cut'

export interface FileClipboard {
  sources: string[]
  operation: FileClipboardOp
}
