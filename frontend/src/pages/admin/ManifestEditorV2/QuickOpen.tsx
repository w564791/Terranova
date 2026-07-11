/**
 * Cmd/Ctrl+P 快速打开文件 — 对标 VS Code Quick Open 的轻量实现。
 */
import { useEffect, useMemo, useRef, useState } from 'react'
import styles from './ManifestEditorV2.module.css'

interface Props {
  open: boolean
  files: string[]
  onSelect: (path: string) => void
  onClose: () => void
}

/** 简单 subsequence 模糊分:连续匹配加分,大小写敏感次优。 */
function fuzzyScore(query: string, target: string): number {
  if (!query) return 1
  const q = query.toLowerCase()
  const t = target.toLowerCase()
  const base = target.split('/').pop() || target
  const baseL = base.toLowerCase()

  // 文件名优先精确/前缀
  if (baseL === q) return 1000
  if (baseL.startsWith(q)) return 800 - baseL.length
  if (baseL.includes(q)) return 600 - baseL.indexOf(q)
  if (t.includes(q)) return 400 - t.indexOf(q)

  // subsequence
  let qi = 0
  let score = 0
  let last = -1
  for (let i = 0; i < t.length && qi < q.length; i++) {
    if (t[i] === q[qi]) {
      score += last === i - 1 ? 5 : 1
      last = i
      qi++
    }
  }
  return qi === q.length ? score : 0
}

export default function QuickOpen({ open, files, onSelect, onClose }: Props) {
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setQuery('')
      setActive(0)
      // 等 portal/DOM 就绪
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [open])

  const ranked = useMemo(() => {
    const scored = files
      .map((path) => ({ path, score: fuzzyScore(query.trim(), path) }))
      .filter((x) => x.score > 0)
      .sort((a, b) => b.score - a.score || a.path.localeCompare(b.path))
    return scored.slice(0, 50)
  }, [files, query])

  useEffect(() => {
    setActive(0)
  }, [query])

  if (!open) return null

  const choose = (path: string) => {
    onSelect(path)
    onClose()
  }

  return (
    <div className={styles.quickOpenOverlay} onMouseDown={onClose}>
      <div
        className={styles.quickOpenBox}
        onMouseDown={(e) => e.stopPropagation()}
        role="dialog"
        aria-label="快速打开"
      >
        <div className={styles.quickOpenInputRow}>
          <i className="codicon codicon-search" />
          <input
            ref={inputRef}
            className={styles.quickOpenInput}
            value={query}
            placeholder="按文件名搜索并打开..."
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.preventDefault()
                onClose()
              } else if (e.key === 'ArrowDown') {
                e.preventDefault()
                setActive((i) => Math.min(i + 1, Math.max(ranked.length - 1, 0)))
              } else if (e.key === 'ArrowUp') {
                e.preventDefault()
                setActive((i) => Math.max(i - 1, 0))
              } else if (e.key === 'Enter') {
                e.preventDefault()
                const hit = ranked[active]
                if (hit) choose(hit.path)
              }
            }}
          />
        </div>
        <div className={styles.quickOpenList}>
          {ranked.length === 0 ? (
            <div className={styles.quickOpenEmpty}>无匹配文件</div>
          ) : (
            ranked.map((item, i) => {
              const name = item.path.split('/').pop() || item.path
              const dir = item.path.includes('/')
                ? item.path.slice(0, item.path.lastIndexOf('/'))
                : ''
              return (
                <div
                  key={item.path}
                  className={`${styles.quickOpenItem} ${i === active ? styles.quickOpenItemActive : ''}`}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => choose(item.path)}
                >
                  <i className="codicon codicon-file-code" />
                  <span className={styles.quickOpenName}>{name}</span>
                  {dir ? <span className={styles.quickOpenDir}>{dir}</span> : null}
                </div>
              )
            })
          )}
        </div>
        <div className={styles.quickOpenHint}>
          ↑↓ 选择 · Enter 打开 · Esc 关闭 · Cmd/Ctrl+P
        </div>
      </div>
    </div>
  )
}
