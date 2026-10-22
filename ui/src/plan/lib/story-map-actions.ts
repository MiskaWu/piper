import { stringify } from 'yaml'
import type { StoryMapData, Story, Task, Release, StoryStatus } from '../types/story-map'
import { nextAvailableColor } from './release-palette'
import { generateId, collectAllIds } from './id-generator'

/** Immutably move a story to a different release tier. Returns new data. */
export function moveStoryRelease(
  data: StoryMapData,
  storyId: string,
  newRelease: Release,
): StoryMapData {
  let found = false

  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => ({
      ...step,
      stories: step.stories.map((story) => {
        if (story.id === storyId) {
          found = true
          return { ...story, release: newRelease }
        }
        return story
      }),
    })),
  }))

  if (!found) return data

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities,
  }
}

/** Immutably update a story's status. Returns new data. */
export function updateStoryStatus(
  data: StoryMapData,
  storyId: string,
  newStatus: StoryStatus,
): StoryMapData {
  let found = false

  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => ({
      ...step,
      stories: step.stories.map((story) => {
        if (story.id === storyId) {
          found = true
          return { ...story, status: newStatus }
        }
        return story
      }),
    })),
  }))

  if (!found) return data

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities,
  }
}

function todayStr(): string {
  return new Date().toISOString().slice(0, 10)
}

/** Check if a release can be removed (no stories reference it). */
export function canRemoveRelease(data: StoryMapData, releaseId: string): boolean {
  for (const act of data.activities) {
    for (const step of act.steps) {
      for (const story of step.stories) {
        if (story.release === releaseId) return false
      }
    }
  }
  return true
}

/** Immutably add a new release. Auto-generates rel- short code and assigns a palette color. */
export function addRelease(
  data: StoryMapData,
  label: string,
): StoryMapData {
  const usedColors = data.meta.releases.map((r) => r.color)
  const color = nextAvailableColor(usedColors)
  const id = generateId(collectAllIds(data), 'rel-')
  return {
    ...data,
    meta: {
      ...data.meta,
      updated: todayStr(),
      releases: [...data.meta.releases, { id, label, color }],
    },
  }
}

/** Immutably remove a release. Returns same data if release has stories. */
export function removeRelease(
  data: StoryMapData,
  releaseId: string,
): StoryMapData {
  if (!canRemoveRelease(data, releaseId)) return data
  return {
    ...data,
    meta: {
      ...data.meta,
      updated: todayStr(),
      releases: data.meta.releases.filter((r) => r.id !== releaseId),
    },
  }
}

/** Immutably reorder a story within a step. Returns new data. */
export function reorderStoryInStep(
  data: StoryMapData,
  stepId: string,
  storyId: string,
  insertBeforeStoryId: string | null,
): StoryMapData {
  // Self-reference is a no-op
  if (storyId === insertBeforeStoryId) return data

  // Find the target step
  let targetStep: { stories: typeof data.activities[0]['steps'][0]['stories'] } | undefined
  for (const act of data.activities) {
    for (const step of act.steps) {
      if (step.id === stepId) {
        targetStep = step
        break
      }
    }
    if (targetStep) break
  }
  if (!targetStep) return data

  // Validate both IDs exist in the step
  const storyIdx = targetStep.stories.findIndex((s) => s.id === storyId)
  if (storyIdx === -1) return data
  if (insertBeforeStoryId !== null) {
    const beforeIdx = targetStep.stories.findIndex((s) => s.id === insertBeforeStoryId)
    if (beforeIdx === -1) return data
  }

  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => {
      if (step.id !== stepId) return step

      const stories = [...step.stories]
      const [moved] = stories.splice(storyIdx, 1)

      if (insertBeforeStoryId === null) {
        stories.push(moved)
      } else {
        const insertIdx = stories.findIndex((s) => s.id === insertBeforeStoryId)
        stories.splice(insertIdx, 0, moved)
      }

      return { ...step, stories }
    }),
  }))

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities,
  }
}

/** Immutably add a new story to a step. Returns new data + the created story (or null if step not found). */
export function addStory(
  data: StoryMapData,
  stepId: string,
  releaseId: Release,
  name: string,
): { data: StoryMapData; story: Story | null } {
  const ids = collectAllIds(data)
  const storyId = generateId(ids, 'story-')
  const newStory: Story = {
    id: storyId,
    name,
    description: '',
    tags: [],
    release: releaseId,
    spike: null,
    adr: null,
    status: 'todo',
    tasks: [],
    acceptance: [],
  }

  let found = false
  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => {
      if (step.id === stepId) {
        found = true
        return { ...step, stories: [...step.stories, newStory] }
      }
      return step
    }),
  }))

  if (!found) return { data, story: null }

  return {
    data: {
      ...data,
      meta: { ...data.meta, updated: todayStr() },
      activities,
    },
    story: newStory,
  }
}

/** Immutably add a new step to an activity. Returns new data. */
export function addStep(
  data: StoryMapData,
  activityId: string,
  name: string,
): StoryMapData {
  const ids = collectAllIds(data)
  const stepId = generateId(ids, 'step-')

  let found = false
  const activities = data.activities.map((act) => {
    if (act.id === activityId) {
      found = true
      return {
        ...act,
        steps: [...act.steps, { id: stepId, name, stories: [] }],
      }
    }
    return act
  })

  if (!found) return data

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities,
  }
}

/** Immutably add a new activity with a default step. Returns new data. */
export function addActivity(
  data: StoryMapData,
  name: string,
): StoryMapData {
  const ids = collectAllIds(data)
  const actId = generateId(ids, 'act-')
  ids.add(actId)
  const stepId = generateId(ids, 'step-')

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities: [
      ...data.activities,
      {
        id: actId,
        name,
        steps: [{ id: stepId, name: '新步驟', stories: [] }],
      },
    ],
  }
}

/** Immutably delete a story by id. Returns same data if not found. */
export function deleteStory(
  data: StoryMapData,
  storyId: string,
): StoryMapData {
  let found = false
  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => {
      if (step.stories.some((s) => s.id === storyId)) {
        found = true
        return { ...step, stories: step.stories.filter((s) => s.id !== storyId) }
      }
      return step
    }),
  }))

  if (!found) return data

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities,
  }
}

/** Immutably add a new task to a story. Returns new data + the created task. */
export function addTask(
  data: StoryMapData,
  storyId: string,
  name: string,
  targetProject: string,
): { data: StoryMapData; task: Task | null } {
  const ids = collectAllIds(data)
  const taskId = generateId(ids, 'task-')
  const newTask: Task = {
    id: taskId,
    name,
    description: '',
    targetProject,
    status: 'todo',
  }

  let found = false
  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => ({
      ...step,
      stories: step.stories.map((story) => {
        if (story.id === storyId) {
          found = true
          return { ...story, tasks: [...story.tasks, newTask] }
        }
        return story
      }),
    })),
  }))

  if (!found) return { data, task: null }

  return {
    data: {
      ...data,
      meta: { ...data.meta, updated: todayStr() },
      activities,
    },
    task: newTask,
  }
}

/** Immutably update a task within a story. Returns new data. */
export function updateTask(
  data: StoryMapData,
  storyId: string,
  taskId: string,
  changes: Partial<Pick<Task, 'name' | 'description' | 'targetProject' | 'status'>>,
): StoryMapData {
  let found = false

  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => ({
      ...step,
      stories: step.stories.map((story) => {
        if (story.id !== storyId) return story
        return {
          ...story,
          tasks: story.tasks.map((task) => {
            if (task.id === taskId) {
              found = true
              return { ...task, ...changes }
            }
            return task
          }),
        }
      }),
    })),
  }))

  if (!found) return data

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities,
  }
}

/** Immutably delete a task from a story. Returns new data. */
export function deleteTask(
  data: StoryMapData,
  storyId: string,
  taskId: string,
): StoryMapData {
  let found = false

  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => ({
      ...step,
      stories: step.stories.map((story) => {
        if (story.id !== storyId) return story
        if (story.tasks.some((t) => t.id === taskId)) {
          found = true
          return { ...story, tasks: story.tasks.filter((t) => t.id !== taskId) }
        }
        return story
      }),
    })),
  }))

  if (!found) return data

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities,
  }
}

/** Immutably update a story's acceptance criteria. Returns new data. */
export function updateStoryAcceptance(
  data: StoryMapData,
  storyId: string,
  acceptance: string[],
): StoryMapData {
  let found = false

  const activities = data.activities.map((act) => ({
    ...act,
    steps: act.steps.map((step) => ({
      ...step,
      stories: step.stories.map((story) => {
        if (story.id === storyId) {
          found = true
          return { ...story, acceptance }
        }
        return story
      }),
    })),
  }))

  if (!found) return data

  return {
    ...data,
    meta: { ...data.meta, updated: todayStr() },
    activities,
  }
}

/** Serialize StoryMapData back to YAML string. */
export function serializeStoryMap(data: StoryMapData): string {
  return stringify(data, { nullStr: 'null', lineWidth: 0 })
}

/** Save StoryMapData to the dev server via PUT. */
export async function saveStoryMap(
  epicName: string,
  data: StoryMapData,
): Promise<void> {
  const yaml = serializeStoryMap(data)
  const res = await fetch(`/api/epics/${encodeURIComponent(epicName)}/story-map`, {
    method: 'PUT',
    headers: { 'Content-Type': 'text/yaml' },
    body: yaml,
  })
  if (!res.ok) {
    throw new Error(`Failed to save: ${res.status} ${res.statusText}`)
  }
}
