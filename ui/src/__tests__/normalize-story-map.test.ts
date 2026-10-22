import { describe, it, expect } from 'vitest'
import { normalizeStoryMap } from '../plan/lib/yaml-loader'

describe('normalizeStoryMap', () => {
  it('null 輸入回傳空的預設結構', () => {
    const result = normalizeStoryMap(null)
    expect(result.meta).toBeDefined()
    expect(result.meta.name).toBe('')
    expect(result.meta.releases).toEqual([])
    expect(result.activities).toEqual([])
  })

  it('undefined 輸入回傳空的預設結構', () => {
    const result = normalizeStoryMap(undefined)
    expect(result.activities).toEqual([])
  })

  it('meta 缺失時補預設值', () => {
    const result = normalizeStoryMap({ activities: [] } as any)
    expect(result.meta.name).toBe('')
    expect(result.meta.updated).toBe('')
    expect(result.meta.owner).toBe('')
    expect(result.meta.releases).toEqual([])
  })

  it('meta.releases 缺失時補空陣列', () => {
    const result = normalizeStoryMap({
      meta: { name: 'test', updated: '2024-01-01', owner: 'me' },
      activities: [],
    } as any)
    expect(result.meta.releases).toEqual([])
  })

  it('activities 缺失時補空陣列', () => {
    const result = normalizeStoryMap({ meta: { name: 'x', updated: '', owner: '', releases: [] } } as any)
    expect(result.activities).toEqual([])
  })

  it('activity.steps 缺失時補空陣列', () => {
    const result = normalizeStoryMap({
      meta: { name: 'x', updated: '', owner: '', releases: [] },
      activities: [{ id: 'a1', name: 'Act1' }],
    } as any)
    expect(result.activities[0].steps).toEqual([])
  })

  it('step.stories 缺失時補空陣列', () => {
    const result = normalizeStoryMap({
      meta: { name: 'x', updated: '', owner: '', releases: [] },
      activities: [{ id: 'a1', name: 'Act1', steps: [{ id: 's1', name: 'Step1' }] }],
    } as any)
    expect(result.activities[0].steps[0].stories).toEqual([])
  })

  it('story.tags 缺失時補空陣列', () => {
    const result = normalizeStoryMap({
      meta: { name: 'x', updated: '', owner: '', releases: [] },
      activities: [{
        id: 'a1', name: 'Act1',
        steps: [{
          id: 's1', name: 'Step1',
          stories: [{ id: 'st1', name: 'Story1', release: 'mvp', status: 'todo' }],
        }],
      }],
    } as any)
    const story = result.activities[0].steps[0].stories[0]
    expect(story.tags).toEqual([])
    expect(story.description).toBe('')
    expect(story.spike).toBeNull()
    expect(story.adr).toBeNull()
    expect(story.tasks).toEqual([])
    expect(story.acceptance).toEqual([])
  })

  it('story.tasks 缺失時補空陣列', () => {
    const result = normalizeStoryMap({
      meta: { name: 'x', updated: '', owner: '', releases: [] },
      activities: [{
        id: 'a1', name: 'Act1',
        steps: [{
          id: 's1', name: 'Step1',
          stories: [{ id: 'st1', name: 'Story1', release: 'mvp', status: 'todo' }],
        }],
      }],
    } as any)
    expect(result.activities[0].steps[0].stories[0].tasks).toEqual([])
  })

  it('task 欄位缺失時補預設值', () => {
    const result = normalizeStoryMap({
      meta: { name: 'x', updated: '', owner: '', releases: [] },
      activities: [{
        id: 'a1', name: 'Act1',
        steps: [{
          id: 's1', name: 'Step1',
          stories: [{
            id: 'st1', name: 'Story1', release: 'mvp', status: 'todo',
            tasks: [{ id: 't1' }],
          }],
        }],
      }],
    } as any)
    const task = result.activities[0].steps[0].stories[0].tasks[0]
    expect(task.id).toBe('t1')
    expect(task.name).toBe('')
    expect(task.description).toBe('')
    expect(task.targetProject).toBe('')
    expect(task.status).toBe('todo')
  })

  it('完整合法資料原樣保留', () => {
    const input = {
      meta: { name: 'Epic', updated: '2024-01-01', owner: 'me', releases: [{ id: 'mvp', label: 'MVP', color: 'blue' }] },
      activities: [{
        id: 'a1', name: 'Act1',
        steps: [{
          id: 's1', name: 'Step1',
          stories: [{
            id: 'st1', name: 'Story1', description: 'desc',
            tags: ['tag1'], release: 'mvp', spike: null, adr: null, status: 'todo' as const,
            tasks: [{ id: 't1', name: 'Task1', description: '', targetProject: 'piper', status: 'todo' as const }],
            acceptance: ['AC1'],
          }],
        }],
      }],
    }
    const result = normalizeStoryMap(input)
    expect(result).toEqual(input)
  })
})
