import { useRetroTrends } from '../hooks/usePlanQueries'
import { RetroTrends } from './RetroTrends'
import { RetroActions } from './RetroActions'
import { RetroReportViewer } from './RetroReportViewer'
import './RetroPanel.css'

interface RetroPanelProps {
  epicName: string
  onClose: () => void
}

export function RetroPanel({ epicName, onClose }: RetroPanelProps) {
  const { data: trends, isLoading, error } = useRetroTrends(epicName)

  return (
    <div className="retro-panel">
      <div className="retro-panel-header">
        <span className="retro-panel-title">Retro</span>
        <button
          type="button"
          className="retro-panel-close"
          onClick={onClose}
          aria-label="關閉"
        >
          ✕
        </button>
      </div>

      <div className="retro-panel-body">{renderBody(epicName, trends, isLoading, error)}</div>
    </div>
  )
}

function renderBody(
  epicName: string,
  trends: ReturnType<typeof useRetroTrends>['data'],
  isLoading: boolean,
  error: unknown,
) {
  if (isLoading) return <div className="retro-panel-loading">載入中…</div>
  if (error) return <div className="retro-panel-error">讀取失敗: {String(error)}</div>
  if (!trends) return null
  return (
    <>
      <RetroTrends releases={trends.releases} />
      <RetroActions epicName={epicName} />
      <RetroReportViewer epicName={epicName} releases={trends.releases} />
    </>
  )
}
