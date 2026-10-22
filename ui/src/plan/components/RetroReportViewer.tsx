import { useState, useMemo } from 'react'
import type { TrendEntry } from '../../api'
import { useRetroReport } from '../hooks/usePlanQueries'
import './RetroReportViewer.css'

interface RetroReportViewerProps {
  epicName: string
  releases: TrendEntry[]
}

export function RetroReportViewer({ epicName, releases }: RetroReportViewerProps) {
  const ordered = useMemo(() => {
    // Latest first (reverse of the trends' ascending order)
    return [...releases].reverse()
  }, [releases])

  const [selected, setSelected] = useState<string>(() => ordered[0]?.reportName ?? '')
  const { data: markdown, isLoading, error } = useRetroReport(epicName, selected, !!selected)

  if (ordered.length === 0) {
    return (
      <div className="retro-report-viewer retro-report-viewer--empty">
        <p>尚無 retro 報告。完成一個 release 後，跑「派鼠回顧」session 產出報告。</p>
      </div>
    )
  }

  return (
    <div className="retro-report-viewer">
      <div className="retro-report-tabs" role="tablist">
        {ordered.map((r) => (
          <button
            key={r.reportName}
            type="button"
            role="tab"
            aria-selected={selected === r.reportName}
            className={`retro-report-tab${selected === r.reportName ? ' retro-report-tab--active' : ''}`}
            onClick={() => setSelected(r.reportName)}
          >
            {r.releaseLabel}
          </button>
        ))}
      </div>
      <div className="retro-report-body">
        {isLoading && <div className="retro-report-loading">載入中…</div>}
        {error && <div className="retro-report-error">讀取失敗: {String(error)}</div>}
        {!isLoading && !error && markdown && (
          <pre className="retro-report-md">{markdown}</pre>
        )}
      </div>
    </div>
  )
}
