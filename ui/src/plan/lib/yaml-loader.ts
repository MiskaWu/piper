import { parse } from 'yaml'
import type { StoryMapData, ReleaseConfig, Activity, Step, Story, Task } from '../types/story-map'
import { PALETTE_NAMES } from './release-palette'

/** Normalize a single task, filling missing fields with defaults */
function normalizeTask(raw: Partial<Task>): Task {
  return {
    id: raw.id ?? '',
    name: raw.name ?? '',
    description: raw.description ?? '',
    targetProject: raw.targetProject ?? '',
    status: raw.status ?? 'todo',
  }
}

/** Normalize a single story, filling missing fields with defaults */
function normalizeStory(raw: Partial<Story>): Story {
  return {
    id: raw.id ?? '',
    name: raw.name ?? '',
    description: raw.description ?? '',
    tags: Array.isArray(raw.tags) ? raw.tags : [],
    release: raw.release ?? '',
    spike: raw.spike ?? null,
    adr: raw.adr ?? null,
    status: raw.status ?? 'todo',
    tasks: Array.isArray(raw.tasks) ? raw.tasks.map((t: Partial<Task>) => normalizeTask(t)) : [],
    acceptance: Array.isArray(raw.acceptance) ? raw.acceptance : [],
  }
}

/** Normalize parsed YAML into a valid StoryMapData, filling any missing fields */
export function normalizeStoryMap(raw: unknown): StoryMapData {
  if (!raw || typeof raw !== 'object') {
    return { meta: { name: '', updated: '', owner: '', releases: [] }, activities: [] }
  }

  const obj = raw as Record<string, unknown>
  const rawMeta = (obj.meta && typeof obj.meta === 'object' ? obj.meta : {}) as Record<string, unknown>

  const meta = {
    name: typeof rawMeta.name === 'string' ? rawMeta.name : '',
    updated: typeof rawMeta.updated === 'string' ? rawMeta.updated : '',
    owner: typeof rawMeta.owner === 'string' ? rawMeta.owner : '',
    releases: Array.isArray(rawMeta.releases) ? rawMeta.releases as ReleaseConfig[] : [],
  }

  const rawActivities = Array.isArray(obj.activities) ? obj.activities : []
  const activities: Activity[] = rawActivities.map((act: Partial<Activity>) => {
    const rawSteps = Array.isArray(act.steps) ? act.steps : []
    const steps: Step[] = rawSteps.map((step: Partial<Step>) => ({
      id: step.id ?? '',
      name: step.name ?? '',
      stories: Array.isArray(step.stories) ? step.stories.map((s: Partial<Story>) => normalizeStory(s)) : [],
    }))
    return { id: act.id ?? '', name: act.name ?? '', steps }
  })

  return { meta, activities }
}

/** Auto-derive releases from stories when meta.releases is missing */
function deriveReleases(data: StoryMapData): ReleaseConfig[] {
  const seen = new Set<string>()
  for (const act of data.activities) {
    for (const step of act.steps) {
      for (const story of step.stories) {
        if (story.release) seen.add(story.release)
      }
    }
  }
  let colorIdx = 0
  return [...seen].map((id) => ({
    id,
    label: id,
    color: PALETTE_NAMES[colorIdx++ % PALETTE_NAMES.length],
  }))
}

export async function loadStoryMap(epicName: string): Promise<StoryMapData> {
  const url = `/api/epics/${encodeURIComponent(epicName)}/story-map`
  const res = await fetch(url)
  if (!res.ok) {
    throw new Error(`Failed to load ${url}: ${res.status}`)
  }
  const text = await res.text()
  const data = normalizeStoryMap(parse(text))
  if (data.meta.releases.length === 0) {
    data.meta.releases = deriveReleases(data)
  }
  return data
}

export function extractAllTags(data: StoryMapData): string[] {
  const tags = new Set<string>()
  for (const activity of data.activities) {
    for (const step of activity.steps) {
      for (const story of step.stories) {
        for (const tag of story.tags) {
          tags.add(tag)
        }
      }
    }
  }
  return [...tags].sort()
}
