/**
 * 文件树右键菜单(自建轻量菜单,不用 antd Dropdown —— React19 下静态 Dropdown 有静默失效前科)。
 *
 * 绝对定位到鼠标处;点击菜单项 / 点击外部 / Esc / 滚动 即关闭。
 */
import { useEffect, useRef } from 'react'
import styles from './ManifestEditorV2.module.css'

export interface ContextMenuItem {
  label: string
  icon?: string // codicon 名(不含 'codicon-' 前缀外的额外类)
  danger?: boolean
  disabled?: boolean
  separatorBefore?: boolean // 该项前画一条分隔线
  onClick: () => void
}

interface Props {
  x: number
  y: number
  items: ContextMenuItem[]
  onClose: () => void
}

export default function TreeContextMenu({ x, y, items, onClose }: Props) {
  const ref = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const onDocMouseDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    const onScroll = () => onClose()
    document.addEventListener('mousedown', onDocMouseDown, true)
    document.addEventListener('keydown', onKeyDown, true)
    // 捕获阶段监听滚动(侧栏滚动时关闭菜单)
    window.addEventListener('scroll', onScroll, true)
    return () => {
      document.removeEventListener('mousedown', onDocMouseDown, true)
      document.removeEventListener('keydown', onKeyDown, true)
      window.removeEventListener('scroll', onScroll, true)
    }
  }, [onClose])

  // 防止菜单超出视口右/下边界
  const style: React.CSSProperties = {
    left: Math.min(x, window.innerWidth - 200),
    top: Math.min(y, window.innerHeight - items.length * 26 - 8),
  }

  return (
    <div ref={ref} className={styles.ctxMenu} style={style} role="menu">
      {items.map((it, i) => (
        <div key={`${it.label}:${i}`}>
          {it.separatorBefore && <div className={styles.ctxMenuSep} />}
          <div
            className={`${styles.ctxMenuItem} ${it.danger ? styles.ctxMenuDanger : ''} ${
              it.disabled ? styles.ctxMenuDisabled : ''
            }`}
            role="menuitem"
            onClick={() => {
              if (it.disabled) return
              onClose()
              it.onClick()
            }}
          >
            <span className={styles.ctxMenuIcon}>
              {it.icon && <i className={`codicon codicon-${it.icon}`} />}
            </span>
            <span className={styles.ctxMenuLabel}>{it.label}</span>
          </div>
        </div>
      ))}
    </div>
  )
}
