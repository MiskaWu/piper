// Epic / StoryMap / Idea 相關型別

export interface EpicInfo {
  id: string    // filesystem slug (used in API paths)
  name: string  // display name (user-facing)
  updated: string
}

export interface Idea {
  id: string
  text: string
  createdAt: number
  enrichedDesc?: string
  context?: IdeaContext[]
}

export interface IdeaContext {
  kind: string
  sessionName: string
  detail: string
}
