import { useState, useCallback } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { RetroAction, RetroActionStatus } from '../../api'
import { addRetroAction, updateRetroActionStatus, deleteRetroAction } from '../../api'
import { planKeys, useRetroActions } from '../hooks/usePlanQueries'
import './RetroActions.css'

interface RetroActionsProps {
  epicName: string
}

type ActionsSnapshot = { actions: RetroAction[] }

export function RetroActions({ epicName }: RetroActionsProps) {
  const queryClient = useQueryClient()
  const { data, isLoading } = useRetroActions(epicName)
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState('')

  const invalidate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: planKeys.retroActions(epicName) })
  }, [queryClient, epicName])

  // Optimistic update: mutate the cache immediately so the UI reflects the
  // change without waiting for the server round-trip. On error we restore the
  // pre-mutation snapshot. onSettled does NOT invalidate — local cache already
  // reflects the authoritative state (server validated it via the PATCH).
  const toggleMutation = useMutation({
    mutationFn: (vars: { id: string; status: RetroActionStatus }) =>
      updateRetroActionStatus(epicName, vars.id, vars.status),
    onMutate: async (vars) => {
      await queryClient.cancelQueries({ queryKey: planKeys.retroActions(epicName) })
      const prev = queryClient.getQueryData<ActionsSnapshot>(planKeys.retroActions(epicName))
      queryClient.setQueryData<ActionsSnapshot>(planKeys.retroActions(epicName), (old) => {
        if (!old) return old
        return {
          actions: old.actions.map((a) =>
            a.id === vars.id
              ? { ...a, status: vars.status, applied_at: vars.status === 'done' ? new Date().toISOString() : undefined }
              : a,
          ),
        }
      })
      return { prev }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(planKeys.retroActions(epicName), ctx.prev)
    },
  })
  const addMutation = useMutation({
    mutationFn: (description: string) => addRetroAction(epicName, description),
    onSuccess: () => {
      setDraft('')
      setAdding(false)
      invalidate()
    },
  })
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteRetroAction(epicName, id),
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: planKeys.retroActions(epicName) })
      const prev = queryClient.getQueryData<ActionsSnapshot>(planKeys.retroActions(epicName))
      queryClient.setQueryData<ActionsSnapshot>(planKeys.retroActions(epicName), (old) =>
        old ? { actions: old.actions.filter((a) => a.id !== id) } : old,
      )
      return { prev }
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(planKeys.retroActions(epicName), ctx.prev)
    },
  })

  const actions: RetroAction[] = data?.actions ?? []
  const todos = actions.filter((a) => a.status !== 'done')
  const dones = actions.filter((a) => a.status === 'done')

  return (
    <div className="retro-actions">
      <div className="retro-actions-header">
        <span className="retro-actions-title">改善行動</span>
        <span className="retro-actions-count">
          {todos.length} 待處理 · {dones.length} 已完成
        </span>
      </div>

      {isLoading && <div className="retro-actions-loading">載入中…</div>}

      {!isLoading && actions.length === 0 && !adding && (
        <div className="retro-actions-empty">尚無改善行動</div>
      )}

      <ul className="retro-actions-list">
        {todos.map((a) => (
          <ActionRow
            key={a.id}
            action={a}
            onToggle={() => toggleMutation.mutate({ id: a.id, status: 'done' })}
            onDelete={() => deleteMutation.mutate(a.id)}
          />
        ))}
      </ul>

      {adding ? (
        <form
          className="retro-actions-add"
          onSubmit={(e) => {
            e.preventDefault()
            if (draft.trim()) addMutation.mutate(draft.trim())
          }}
        >
          <input
            className="retro-actions-input"
            placeholder="新增一條改善行動…"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            autoFocus
          />
          <button
            type="submit"
            className="retro-actions-btn retro-actions-btn--primary"
            disabled={!draft.trim() || addMutation.isPending}
          >
            新增
          </button>
          <button
            type="button"
            className="retro-actions-btn"
            onClick={() => {
              setAdding(false)
              setDraft('')
            }}
          >
            取消
          </button>
        </form>
      ) : (
        <button className="retro-actions-add-btn" onClick={() => setAdding(true)}>
          + 新增 Action
        </button>
      )}

      {dones.length > 0 && (
        <details className="retro-actions-done">
          <summary>已完成 ({dones.length})</summary>
          <ul className="retro-actions-list">
            {dones.map((a) => (
              <ActionRow
                key={a.id}
                action={a}
                onToggle={() => toggleMutation.mutate({ id: a.id, status: 'todo' })}
                onDelete={() => deleteMutation.mutate(a.id)}
              />
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}

interface ActionRowProps {
  action: RetroAction
  onToggle: () => void
  onDelete: () => void
}

function ActionRow({ action, onToggle, onDelete }: ActionRowProps) {
  const done = action.status === 'done'
  return (
    <li className={`retro-action-row${done ? ' retro-action-row--done' : ''}`}>
      <input
        type="checkbox"
        checked={done}
        onChange={onToggle}
        className="retro-action-checkbox"
      />
      <span className="retro-action-desc">{action.description}</span>
      {action.from_retro && (
        <span className="retro-action-badge">{action.from_retro}</span>
      )}
      <button
        type="button"
        className="retro-action-delete"
        onClick={onDelete}
        aria-label="刪除"
      >
        ✕
      </button>
    </li>
  )
}
