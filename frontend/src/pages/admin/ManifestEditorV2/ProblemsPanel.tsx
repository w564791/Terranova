/**
 * Problems 面板 — 聚合 Monaco markers,点击定位到行列。
 * 对标 VS Code Problems 视图的最小实现(无筛选/分组树,按文件平铺)。
 */
import { useMemo } from 'react'
import * as monaco from 'monaco-editor'
import styles from './ManifestEditorV2.module.css'

export interface ProblemItem {
  path: string
  severity: monaco.MarkerSeverity
  message: string
  startLineNumber: number
  startColumn: number
  endLineNumber: number
  endColumn: number
  owner: string
  /** 来源标记(如 'publish' = 发布被 bundle 规则拒绝);缺省为 Monaco markers */
  source?: string
}

/** path 为空的问题(bundle 级规则)无文件可跳转 */
const NO_FILE_LABEL = '(整个 bundle)'

interface Props {
  problems: ProblemItem[]
  onOpenAt: (path: string, line: number, column: number, endColumn: number) => void
}

function severityIcon(sev: monaco.MarkerSeverity): { icon: string; color: string; label: string } {
  if (sev === monaco.MarkerSeverity.Error) {
    return { icon: 'codicon-error', color: 'var(--red)', label: 'Error' }
  }
  if (sev === monaco.MarkerSeverity.Warning) {
    return { icon: 'codicon-warning', color: '#cca700', label: 'Warning' }
  }
  if (sev === monaco.MarkerSeverity.Info) {
    return { icon: 'codicon-info', color: '#3794ff', label: 'Info' }
  }
  return { icon: 'codicon-circle-outline', color: '#858585', label: 'Hint' }
}

export default function ProblemsPanel({ problems, onOpenAt }: Props) {
  const grouped = useMemo(() => {
    const map = new Map<string, ProblemItem[]>()
    for (const p of problems) {
      const list = map.get(p.path) ?? []
      list.push(p)
      map.set(p.path, list)
    }
    return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [problems])

  const errors = problems.filter((p) => p.severity === monaco.MarkerSeverity.Error).length
  const warnings = problems.filter((p) => p.severity === monaco.MarkerSeverity.Warning).length

  return (
    <div className={styles.problemsPanel}>
      <div className={styles.header}>
        <span>问题</span>
        <span className={styles.problemsSummary}>
          <i className="codicon codicon-error" style={{ color: 'var(--red)' }} /> {errors}
          <i className="codicon codicon-warning" style={{ color: '#cca700', marginLeft: 8 }} /> {warnings}
        </span>
      </div>
      <div className={styles.problemsList}>
        {problems.length === 0 ? (
          <div className={styles.problemsEmpty}>未检测到问题</div>
        ) : (
          grouped.map(([path, items]) => (
            <div key={path} className={styles.problemsFileGroup}>
              <div className={styles.problemsFilePath}>
                <i className="codicon codicon-file-code" />
                <span title={path || NO_FILE_LABEL}>{path || NO_FILE_LABEL}</span>
                <span className={styles.problemsFileCount}>{items.length}</span>
              </div>
              {items.map((item, i) => {
                const sev = severityIcon(item.severity)
                return (
                  <div
                    key={`${path}-${item.startLineNumber}-${item.startColumn}-${i}`}
                    className={styles.problemsItem}
                    role="button"
                    onClick={() => {
                      if (item.path) onOpenAt(item.path, item.startLineNumber, item.startColumn, item.endColumn)
                    }}
                    title={item.source ? `[${item.source}] ${item.message}` : item.message}
                  >
                    <i className={`codicon ${sev.icon}`} style={{ color: sev.color }} />
                    <span className={styles.problemsMsg}>{item.message}</span>
                    <span className={styles.problemsLoc}>
                      [{item.startLineNumber},{item.startColumn}]
                    </span>
                  </div>
                )
              })}
            </div>
          ))
        )}
      </div>
    </div>
  )
}
