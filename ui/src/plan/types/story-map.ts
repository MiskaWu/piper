export type Release = string
export type StoryStatus = 'todo' | 'in-progress' | 'in-review' | 'done'
export type TaskStatus = 'todo' | 'in-progress' | 'in-review' | 'done'

export interface ReleaseConfig {
  id: string
  label: string
  color: string
}

export interface Task {
  id: string
  name: string
  description: string
  targetProject: string
  status: TaskStatus
}

export interface Story {
  id: string
  name: string
  description: string
  tags: string[]
  release: Release
  spike: string | null
  adr: string | null
  status: StoryStatus
  tasks: Task[]
  acceptance: string[]
}

export interface Step {
  id: string
  name: string
  stories: Story[]
}

export interface Activity {
  id: string
  name: string
  steps: Step[]
}

export interface StoryMapMeta {
  name: string
  updated: string
  owner: string
  releases: ReleaseConfig[]
}

export interface StoryMapData {
  meta: StoryMapMeta
  activities: Activity[]
}
