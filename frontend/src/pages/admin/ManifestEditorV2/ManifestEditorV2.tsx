/**
 * Manifest Editor v2 — VS Code Web 工作区(B2 模式)
 *
 * 视觉 1:1 对齐 manifest-vscode-mockup.html demo。
 * 数据接 PR1 实现的 manifest_files API。
 *
 * 当前阶段(PR2-B 验收): layout shell + 文件树 + tab + Monaco 接 manifest_files。
 * Toolbar 三按钮(Run/发布/部署)是占位,弹窗在 PR2-D。
 * HCL 高亮在 PR2-C。
 */
import { useEffect, useRef, useState, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import * as monaco from 'monaco-editor'
import 'monaco-editor/esm/vs/editor/editor.all.js'
import '@vscode/codicons/dist/codicon.css'
import { ensureVscodeServicesReady } from './initServices'
import {
  listFiles,
  readFile,
  putFile,
  languageOfPath,
  type ManifestFileEntry,
  type ManifestEditorContext,
} from './manifestApi'
import styles from './ManifestEditorV2.module.css'

const AUTOSAVE_DEBOUNCE_MS = 1000

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error'

export default function ManifestEditorV2() {
  const params = useParams<{ id: string; org_id?: string }>()
  const manifestId = params.id || 'sandbox'
  // org_id 路由暂未带,先从 localStorage 取(项目其他页面也是这模式)
  const orgId = params.org_id || localStorage.getItem('current_org_id') || '1'
  const ctx: ManifestEditorContext = { orgId, manifestId }

  const containerRef = useRef<HTMLDivElement | null>(null)
  const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)
  const fileContentCache = useRef<Map<string, string>>(new Map())
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const [bootError, setBootError] = useState<string | null>(null)
  const [files, setFiles] = useState<ManifestFileEntry[]>([])
  const [openTabs, setOpenTabs] = useState<string[]>([])
  const [currentFile, setCurrentFile] = useState<string | null>(null)
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle')
  const [cursor, setCursor] = useState({ line: 1, col: 1 })

  // ========== 初始化: 起 vscode-api + 创建编辑器 ==========
  useEffect(() => {
    let cancelled = false

    ensureVscodeServicesReady()
      .then(() => {
        if (cancelled || !containerRef.current) return
        editorRef.current = monaco.editor.create(containerRef.current, {
          value: '',
          language: 'plaintext',
          theme: 'Default Dark Modern',
          automaticLayout: true,
          minimap: { enabled: true },
          fontSize: 13,
          fontFamily: 'Menlo, Monaco, "Cascadia Code", Consolas, "Courier New", monospace',
          tabSize: 2,
          renderWhitespace: 'selection',
        })
        editorRef.current.onDidChangeCursorPosition((e) => {
          setCursor({ line: e.position.lineNumber, col: e.position.column })
        })
        editorRef.current.onDidChangeModelContent(() => {
          if (!currentFileRef.current) return
          setSaveStatus('saving')
          if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
          saveTimerRef.current = setTimeout(() => {
            void saveCurrentFile()
          }, AUTOSAVE_DEBOUNCE_MS)
        })
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : String(err)
        // eslint-disable-next-line no-console
        console.error('[ManifestEditorV2] init failed', err)
        setBootError(msg)
      })

    return () => {
      cancelled = true
      editorRef.current?.dispose()
      editorRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ========== 加载文件树 ==========
  useEffect(() => {
    let cancelled = false
    listFiles(ctx)
      .then((items) => {
        if (cancelled) return
        setFiles(items)
        // 自动打开第一个 .tf, 否则第一个文件
        const firstTf = items.find((f) => f.path.endsWith('.tf'))
        const first = firstTf ?? items[0]
        if (first) {
          openFile(first.path)
        }
      })
      .catch((err: unknown) => {
        // 列表失败不致命, 显示空文件树
        // eslint-disable-next-line no-console
        console.warn('[ManifestEditorV2] list files failed', err)
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [manifestId, orgId])

  // ========== 打开文件 ==========
  const currentFileRef = useRef<string | null>(null)
  useEffect(() => {
    currentFileRef.current = currentFile
  }, [currentFile])

  const openFile = useCallback(
    async (path: string) => {
      // 等编辑器创建完成
      if (!editorRef.current) {
        await ensureVscodeServicesReady()
      }
      const ed = editorRef.current
      if (!ed) return

      setOpenTabs((prev) => (prev.includes(path) ? prev : [...prev, path]))
      setCurrentFile(path)

      let content = fileContentCache.current.get(path)
      if (content === undefined) {
        try {
          const f = await readFile(ctx, path)
          if (f.is_binary) {
            content = ''
          } else {
            content = f.content ?? ''
          }
          fileContentCache.current.set(path, content)
        } catch (err) {
          // eslint-disable-next-line no-console
          console.error('[ManifestEditorV2] read file failed', err)
          return
        }
      }

      const lang = languageOfPath(path)
      const old = ed.getModel()
      const model = monaco.editor.createModel(content, lang)
      ed.setModel(model)
      old?.dispose()
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [orgId, manifestId],
  )

  // ========== 关闭 tab ==========
  const closeTab = useCallback(
    (path: string) => {
      setOpenTabs((prev) => {
        const next = prev.filter((p) => p !== path)
        if (path === currentFile) {
          const fallback = next[0] ?? null
          if (fallback) {
            void openFile(fallback)
          } else {
            setCurrentFile(null)
            const m = editorRef.current?.getModel()
            m?.setValue('')
          }
        }
        return next
      })
    },
    [currentFile, openFile],
  )

  // ========== 保存当前文件 ==========
  const saveCurrentFile = useCallback(async () => {
    const path = currentFileRef.current
    const ed = editorRef.current
    if (!path || !ed) return
    const value = ed.getModel()?.getValue() ?? ''
    fileContentCache.current.set(path, value)
    try {
      await putFile(ctx, path, value)
      setSaveStatus('saved')
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ManifestEditorV2] save failed', err)
      setSaveStatus('error')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orgId, manifestId])

  // ========== 渲染 ==========
  return (
    <div className={styles.root}>
      <div className={styles.titleBar}>
        <div className={styles.traffic}>
          <span className={styles.red} />
          <span className={styles.yellow} />
          <span className={styles.green} />
        </div>
        <div className={styles.breadcrumb}>
          <span className={styles.muted}>Terranova</span>
          <span className={styles.muted}> › </span>
          <span className={styles.muted}>manifests</span>
          <span className={styles.muted}> › </span>
          <span>{manifestId}</span>
          <span className={styles.badge}>DRAFT</span>
        </div>
      </div>

      <div className={styles.toolbar}>
        <div className={styles.group}>
          <span className={styles.meta}>
            <span className={`${styles.dot} ${saveStatus === 'saved' ? styles.saved : ''}`} />
            <span>
              {saveStatus === 'saving' && '保存中...'}
              {saveStatus === 'saved' && '已保存到草稿'}
              {saveStatus === 'error' && '保存失败'}
              {saveStatus === 'idle' && '准备就绪'}
            </span>
          </span>
        </div>
        <div className={styles.spacer} />
        <div className={styles.group}>
          <button title="对当前草稿在已部署 workspace 跑 plan-only 检测" disabled>
            <i className="codicon codicon-play" /> Run
          </button>
          <button title="把当前草稿固化为新的不可变版本" disabled>
            <i className="codicon codicon-tag" /> 发布版本
          </button>
          <button className={styles.primary} title="把已发布版本部署到 Workspace" disabled>
            <i className="codicon codicon-rocket" /> 部署到 Workspace
          </button>
        </div>
      </div>

      <div className={styles.activityBar}>
        <div className={`${styles.item} ${styles.active}`} title="资源管理器">
          <i className="codicon codicon-files" />
        </div>
        <div className={styles.item} title="搜索">
          <i className="codicon codicon-search" />
        </div>
        <div className={`${styles.item} ${styles.disabled}`} title="源代码管理 (本期不启用 — 直接存 Postgres)">
          <i className="codicon codicon-source-control" />
        </div>
        <div className={styles.item} title="版本与部署历史">
          <i className="codicon codicon-history" />
        </div>
        <div className={styles.spacer} />
        <div className={styles.item} title="配置">
          <i className="codicon codicon-settings-gear" />
        </div>
      </div>

      <div className={styles.sideBar}>
        <div className={styles.header}>
          <span>资源管理器</span>
          <span className={styles.actions}>
            <i className="codicon codicon-new-file" title="新建文件" />
            <i className="codicon codicon-new-folder" title="新建目录" />
            <i className="codicon codicon-refresh" title="刷新" />
          </span>
        </div>
        <div className={styles.project}>
          <i className="codicon codicon-chevron-down" />
          <span>{manifestId.toUpperCase()}</span>
        </div>
        <div className={styles.tree}>
          {files.length === 0 && (
            <div style={{ padding: '8px 12px', color: '#858585', fontSize: 12 }}>
              草稿为空,在编辑器内输入即自动保存
            </div>
          )}
          {files
            .slice()
            .sort((a, b) => a.path.localeCompare(b.path))
            .map((f) => (
              <div
                key={f.path}
                className={`${styles.treeNode} ${currentFile === f.path ? styles.selected : ''}`}
                onClick={() => void openFile(f.path)}
              >
                <span className={`${styles.chevron} ${styles.empty}`} />
                <span className={styles.icon}>
                  <i className={`codicon ${iconClassFor(f.path)}`} />
                </span>
                <span className={styles.name}>{f.path}</span>
              </div>
            ))}
        </div>
      </div>

      <div className={styles.editorArea}>
        <div className={styles.tabs}>
          {openTabs.map((path) => (
            <div
              key={path}
              className={`${styles.tab} ${currentFile === path ? styles.active : ''}`}
              onClick={() => void openFile(path)}
            >
              <i className={`codicon ${iconClassFor(path)}`} />
              <span>{path.split('/').pop()}</span>
              <span
                className={styles.close}
                onClick={(e) => {
                  e.stopPropagation()
                  closeTab(path)
                }}
              >
                <i className="codicon codicon-close" />
              </span>
            </div>
          ))}
        </div>
        <div className={styles.editorHost}>
          {bootError ? (
            <div className={`${styles.overlay} ${styles.error}`}>
              {`vscode-api 初始化失败:\n\n${bootError}`}
            </div>
          ) : null}
          <div ref={containerRef} style={{ width: '100%', height: '100%' }} />
        </div>
      </div>

      <div className={styles.statusBar}>
        <span className={styles.item}>
          <i className="codicon codicon-cloud-upload" /> 草稿
        </span>
        <span className={styles.item}>
          <i className="codicon codicon-circle-filled" style={{ color: saveStatus === 'saved' ? '#4ec9b0' : '#cca700' }} />
          {saveStatus === 'saved' ? '自动保存' : '编辑中'}
        </span>
        <div className={styles.spacer} />
        <span className={styles.item}>{`行 ${cursor.line}, 列 ${cursor.col}`}</span>
        <span className={styles.item}>UTF-8</span>
        <span className={styles.item}>LF</span>
        <span className={styles.item}>{currentFile ? languageDisplay(currentFile) : ''}</span>
      </div>
    </div>
  )
}

// ========== 工具 ==========

function iconClassFor(path: string): string {
  if (path.endsWith('.tf') || path.endsWith('.tfvars') || path.endsWith('.hcl')) return 'codicon-symbol-namespace'
  if (path.endsWith('.yaml') || path.endsWith('.yml')) return 'codicon-file-code'
  if (path.endsWith('.md')) return 'codicon-markdown'
  if (path.endsWith('.sh') || path.endsWith('.tpl')) return 'codicon-terminal'
  if (path.endsWith('.pem') || path.endsWith('.key') || path.endsWith('.crt')) return 'codicon-key'
  return 'codicon-file'
}

function languageDisplay(path: string): string {
  const lang = languageOfPath(path)
  switch (lang) {
    case 'hcl':
      return 'Terraform'
    case 'markdown':
      return 'Markdown'
    case 'shellscript':
      return 'Shell Script'
    case 'yaml':
      return 'YAML'
    case 'json':
      return 'JSON'
    default:
      return 'Plain Text'
  }
}
