import type { StoryMapData } from '../types/story-map'

const CHARS = 'abcdefghijklmnopqrstuvwxyz0123456789'
const ID_LENGTH = 5

/** Generate a unique ID with given prefix + 5-char alphanumeric short code. */
export function generateId(existingIds: Set<string>, prefix = 'story-'): string {
  let id: string
  do {
    let code = ''
    for (let i = 0; i < ID_LENGTH; i++) {
      code += CHARS[Math.floor(Math.random() * CHARS.length)]
    }
    id = prefix + code
  } while (existingIds.has(id))
  return id
}

/** Collect all IDs (activity, step, story, release) from a StoryMapData. */
export function collectAllIds(data: StoryMapData): Set<string> {
  const ids = new Set<string>()
  for (const rc of data.meta.releases) {
    ids.add(rc.id)
  }
  for (const act of data.activities) {
    ids.add(act.id)
    for (const step of act.steps) {
      ids.add(step.id)
      for (const story of step.stories) {
        ids.add(story.id)
      }
    }
  }
  return ids
}
