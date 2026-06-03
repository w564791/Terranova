/**
 * Manifest Editor v2 — VS Code Web 工作区(B2 模式)
 *
 * 视觉 1:1 对齐 manifest-vscode-mockup.html demo,数据接 manifest_files API。
 *
 * 能力: layout shell + 文件树 + tab + Monaco(HCL 高亮 + 4 个 provider)接 manifest_files;
 * Toolbar 三按钮 Run / 发布 / 部署 分别挂 RunDialog / PublishVersionDialog / DeployDialog。
 */
import type { ReactNode } from 'react'
import { useEffect, useRef, useState, useCallback, useMemo } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import * as monaco from 'monaco-editor'
import 'monaco-editor/esm/vs/editor/editor.all.js'
import '@vscode/codicons/dist/codicon.css'
import { Modal, message } from 'antd'
import { ensureVscodeServicesReady } from './initServices'
import { registerHclLanguage } from './hclLanguage'
import { registerHclProviders } from './hclProviders'
import { registerHclCompletion } from './hclCompletion'
import PublishVersionDialog from './PublishVersionDialog'
import DeployDialog from './DeployDialog'
import RunDialog from './RunDialog'
import {
  listFiles,
  readFile,
  putFile,
  putFileB64,
  deleteFile,
  moveFile,
  languageOfPath,
  type ManifestFileEntry,
  type ManifestEditorContext,
} from './manifestApi'
import styles from './ManifestEditorV2.module.css'

const AUTOSAVE_DEBOUNCE_MS = 1000
// 单文件上限,与后端 MANIFEST_MAX_FILE_SIZE 默认值(1MB)对齐
const MANIFEST_MAX_FILE_SIZE = 1024 * 1024

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error'

// fileToBase64 读本地 File 为 base64(去掉 data URL 前缀),用于拖拽上传
function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const res = reader.result as string
      const comma = res.indexOf(',')
      resolve(comma >= 0 ? res.slice(comma + 1) : res)
    }
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

export default function ManifestEditorV2() {
  const params = useParams<{ id: string; org_id?: string }>()
  const [searchParams] = useSearchParams()
  const manifestId = params.id || 'sandbox'
  // org_id 多源 fallback: path param > query string ?org= > localStorage > '1'
  const orgId =
    params.org_id ||
    searchParams.get('org') ||
    localStorage.getItem('current_org_id') ||
    '1'
  const ctx: ManifestEditorContext = { orgId, manifestId }

  const containerRef = useRef<HTMLDivElement | null>(null)
  const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)
  const fileContentCache = useRef<Map<string, string>>(new Map())
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const [bootError, setBootError] = useState<string | null>(null)
  const [manifestMissing, setManifestMissing] = useState(false)
  const [publishOpen, setPublishOpen] = useState(false)
  const [deployOpen, setDeployOpen] = useState(false)
  const [runOpen, setRunOpen] = useState(false)
  // 内联新建/重命名(VS Code 风格,不弹窗):
  //   creating !== null  → 文件树顶部出现一个输入行,值即 creating
  //   renamingPath        → 该行 name 就地变输入框,值即 renameValue
  const [creating, setCreating] = useState<string | null>(null)
  const [renamingPath, setRenamingPath] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [inlineError, setInlineError] = useState<string | null>(null)
  const [files, setFiles] = useState<ManifestFileEntry[]>([])
  const [openTabs, setOpenTabs] = useState<string[]>([])
  const [currentFile, setCurrentFile] = useState<string | null>(null)
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle')
  const [cursor, setCursor] = useState({ line: 1, col: 1 })
  // 有未保存修改的文件(tab 显示白点);autosave 成功后移除
  const [dirtyFiles, setDirtyFiles] = useState<Set<string>>(new Set())
  // 当前打开的是二进制文件(spec §5.2 走只读视图,隐藏 Monaco)
  const [binaryView, setBinaryView] = useState<{ path: string; size: number; mime: string } | null>(null)
  // 文件树展开的目录(默认全展开);collapsedDirs 记录被手动折叠的目录,默认不在集合即展开
  const [collapsedDirs, setCollapsedDirs] = useState<Set<string>>(new Set())
  // 拖拽上传:鼠标拖文件悬停在文件树上时高亮
  const [dragOver, setDragOver] = useState(false)

  // ========== 初始化: 起 vscode-api + 创建编辑器 ==========
  useEffect(() => {
    let cancelled = false

    ensureVscodeServicesReady()
      .then(() => {
        if (cancelled || !containerRef.current) return
        // 注册 HCL 语言 + 高亮 (idempotent, 全进程一次)
        registerHclLanguage()
        // 注册 4 个 demo provider (Completion / Hover / InlayHint / CodeAction)
        registerHclProviders()
        // 通用 HCL 补全 (Tier1 关键字/骨架 + Tier2 引用 + Tier3 平台 module 属性)
        registerHclCompletion()
        // spec §10.3.3: Inlay Hint (· N demos) 不用 monaco 默认灰,覆盖为高对比青绿,
        // 让 demo 标签看起来既"是信息"也"是按钮"。
        monaco.editor.defineTheme('vs-dark-manifest', {
          base: 'vs-dark',
          inherit: true,
          rules: [],
          colors: {
            'editorInlayHint.foreground': '#4ec9b0',
            'editorInlayHint.background': '#4ec9b022',
            'editorInlayHint.typeForeground': '#4ec9b0',
            'editorInlayHint.typeBackground': '#4ec9b022',
          },
        })
        editorRef.current = monaco.editor.create(containerRef.current, {
          value: '',
          language: 'plaintext',
          // fallback 主题:即使 vscode-api 主题加载失败也保持深色 + Inlay Hint 青绿
          // (vscode-api 加载成功后会自动切到 'Default Dark+',不冲突)
          theme: 'vs-dark-manifest',
          automaticLayout: true,
          fontFamily: 'Menlo, Monaco, "Cascadia Code", Consolas, "Courier New", monospace',
        })
        editorRef.current.onDidChangeCursorPosition((e) => {
          setCursor({ line: e.position.lineNumber, col: e.position.column })
        })
        editorRef.current.onDidChangeModelContent(() => {
          const p = currentFileRef.current
          if (!p) return
          // 标记该文件为 dirty(tab 显示白点),autosave 成功后清除
          setDirtyFiles((prev) => {
            if (prev.has(p)) return prev
            const next = new Set(prev)
            next.add(p)
            return next
          })
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
        setManifestMissing(false)
        setFiles(items)
        // 自动打开第一个 .tf, 否则第一个文件
        const firstTf = items.find((f) => f.path.endsWith('.tf'))
        const first = firstTf ?? items[0]
        if (first) {
          openFile(first.path)
        }
      })
      .catch((err: unknown) => {
        if (cancelled) return
        // axios 全局 interceptor 把 error 拦成 errorMessage 字符串,这里拿不到
        // status code,凡是 listFiles 失败统一当作"未就绪/未授权",显示友好提示。
        // eslint-disable-next-line no-console
        console.warn('[ManifestEditorV2] list files unavailable:', err)
        setManifestMissing(true)
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

      // 二进制文件:spec §5.2 走只读视图,不灌进 Monaco
      const meta = files.find((f) => f.path === path)
      if (meta?.is_binary) {
        setBinaryView({ path, size: meta.size, mime: meta.mime })
        return
      }
      setBinaryView(null)

      let content = fileContentCache.current.get(path)
      if (content === undefined) {
        try {
          const f = await readFile(ctx, path)
          // 列表元信息没标 binary 但后端读出来是 binary 时兜底
          if (f.is_binary) {
            setBinaryView({ path, size: f.size, mime: f.mime })
            return
          }
          content = f.content ?? ''
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
    [orgId, manifestId, files],
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
      // 保存成功 → 清除 dirty 标记
      setDirtyFiles((prev) => {
        if (!prev.has(path)) return prev
        const next = new Set(prev)
        next.delete(path)
        return next
      })
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ManifestEditorV2] save failed', err)
      setSaveStatus('error')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orgId, manifestId])

  // 路径合法性校验(与后端 normalizeAndValidatePath 对齐),返回错误文案或 null
  const validatePath = useCallback(
    (raw: string, opts?: { excludeSelf?: string }): string | null => {
      const v = (raw || '').trim()
      if (!v) return '请输入文件名'
      if (!/^[A-Za-z0-9_\-./]+$/.test(v)) return '只允许字母数字 _ - . /'
      if (v.startsWith('/')) return '不允许绝对路径'
      if (v.endsWith('/')) return '不能以 / 结尾'
      if (v.split('/').some((s) => s === '.' || s === '..')) return '不允许 . 或 .. 路径段'
      if (v.length > 256) return '路径过长 (>256)'
      if (files.some((f) => f.path === v && v !== opts?.excludeSelf)) return '已存在同名文件'
      return null
    },
    [files],
  )

  // ========== 新建文件(内联确认)==========
  const commitCreateFile = useCallback(async () => {
    const path = (creating || '').trim()
    const errMsg = validatePath(path)
    if (errMsg) {
      setInlineError(errMsg)
      return
    }
    try {
      await putFile(ctx, path, '')
      setCreating(null)
      setInlineError(null)
      setManifestMissing(false)
      const items = await listFiles(ctx)
      setFiles(items)
      void openFile(path)
    } catch (err: any) {
      const msg = typeof err === 'string' ? err : err?.message
      setInlineError(msg ?? '创建失败')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [creating, ctx, validatePath])

  const startCreateFile = useCallback(() => {
    setRenamingPath(null)
    setInlineError(null)
    setCreating('')
  }, [])

  // ========== 删除文件 ==========
  const handleDeleteFile = useCallback(
    (path: string) => {
      Modal.confirm({
        title: '删除文件?',
        content: `确认删除 ${path}?(仅作用于当前用户私有草稿)`,
        okText: '删除',
        okButtonProps: { danger: true },
        cancelText: '取消',
        onOk: async () => {
          try {
            await deleteFile(ctx, path)
            fileContentCache.current.delete(path)
            // 关闭对应 tab
            setOpenTabs((prev) => prev.filter((p) => p !== path))
            if (currentFile === path) {
              setCurrentFile(null)
              const m = editorRef.current?.getModel()
              m?.setValue('')
            }
            // 刷新文件树
            const items = await listFiles(ctx)
            setFiles(items)
            message.success('已删除')
          } catch (err: any) {
            const msg = typeof err === 'string' ? err : err?.message
            message.error(`删除失败: ${msg ?? '未知错误'}`)
          }
        },
      })
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [orgId, manifestId, currentFile],
  )

  const startRename = useCallback((path: string) => {
    setCreating(null)
    setInlineError(null)
    setRenameValue(path)
    setRenamingPath(path)
  }, [])

  // ========== 重命名文件(内联确认)==========
  const commitRenameFile = useCallback(async () => {
    const fromPath = renamingPath
    if (!fromPath) return
    const to = (renameValue || '').trim()
    if (to === fromPath) {
      setRenamingPath(null)
      setInlineError(null)
      return
    }
    const errMsg = validatePath(to, { excludeSelf: fromPath })
    if (errMsg) {
      setInlineError(errMsg)
      return
    }
    try {
      await moveFile(ctx, fromPath, to)
      const cached = fileContentCache.current.get(fromPath)
      fileContentCache.current.delete(fromPath)
      if (cached !== undefined) fileContentCache.current.set(to, cached)
      setOpenTabs((prev) => prev.map((p) => (p === fromPath ? to : p)))
      if (currentFile === fromPath) setCurrentFile(to)
      const items = await listFiles(ctx)
      setFiles(items)
      setRenamingPath(null)
      setInlineError(null)
    } catch (err: any) {
      const msg = typeof err === 'string' ? err : err?.message
      setInlineError(msg ?? '重命名失败')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [renamingPath, renameValue, currentFile, ctx, validatePath])

  const fileTree = useMemo(() => buildFileTree(files.map((f) => f.path)), [files])

  const toggleDir = useCallback((dirPath: string) => {
    setCollapsedDirs((prev) => {
      const next = new Set(prev)
      if (next.has(dirPath)) next.delete(dirPath)
      else next.add(dirPath)
      return next
    })
  }, [])

  // ========== 拖拽上传本地文件(spec §4.3)==========
  const handleDropFiles = useCallback(
    async (dropped: FileList) => {
      setDragOver(false)
      const list = Array.from(dropped)
      let ok = 0
      for (const file of list) {
        if (file.size > MANIFEST_MAX_FILE_SIZE) {
          message.error(`${file.name} 超过 ${MANIFEST_MAX_FILE_SIZE / 1024 / 1024}MB,跳过`)
          continue
        }
        const errMsg = validatePath(file.name)
        if (errMsg) {
          message.error(`${file.name}: ${errMsg}`)
          continue
        }
        try {
          const b64 = await fileToBase64(file)
          await putFileB64(ctx, file.name, b64)
          ok++
        } catch (err: any) {
          message.error(`${file.name} 上传失败: ${err?.message ?? err}`)
        }
      }
      if (ok > 0) {
        message.success(`已上传 ${ok} 个文件`)
        setManifestMissing(false)
        const items = await listFiles(ctx)
        setFiles(items)
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [ctx, validatePath],
  )

  // 递归渲染文件树节点。depth 控制缩进(每层 +12px,与 VS Code 一致)
  const renderTreeNodes = (nodes: TreeNode[], depth: number): ReactNode =>
    nodes.map((node) => {
      const indent = 8 + depth * 12
      if (node.type === 'dir') {
        const collapsed = collapsedDirs.has(node.path)
        return (
          <div key={`dir:${node.path}`}>
            <div
              className={styles.treeNode}
              style={{ paddingLeft: indent }}
              onClick={() => toggleDir(node.path)}
            >
              <span className={styles.chevron}>
                <i className={`codicon ${collapsed ? 'codicon-chevron-right' : 'codicon-chevron-down'}`} />
              </span>
              <span className={styles.icon}>
                <i className={`codicon ${collapsed ? 'codicon-folder' : 'codicon-folder-opened'} ${styles.iconFolder}`} />
              </span>
              <span className={styles.name}>{node.name}</span>
            </div>
            {!collapsed && node.children && renderTreeNodes(node.children, depth + 1)}
          </div>
        )
      }
      // file
      return (
        <div
          key={`file:${node.path}`}
          className={`${styles.treeNode} ${currentFile === node.path ? styles.selected : ''}`}
          style={{ paddingLeft: indent }}
          onClick={() => {
            if (renamingPath === node.path) return
            void openFile(node.path)
          }}
        >
          <span className={`${styles.chevron} ${styles.empty}`} />
          <span className={styles.icon}>
            <i className={`codicon ${iconClassFor(node.path)}`} />
          </span>
          {renamingPath === node.path ? (
            <input
              className={styles.inlineInput}
              autoFocus
              value={renameValue}
              onChange={(e) => {
                setRenameValue(e.target.value)
                if (inlineError) setInlineError(null)
              }}
              onClick={(e) => e.stopPropagation()}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void commitRenameFile()
                else if (e.key === 'Escape') {
                  setRenamingPath(null)
                  setInlineError(null)
                }
              }}
              onBlur={() => {
                if ((renameValue || '').trim() && renameValue !== node.path) void commitRenameFile()
                else setRenamingPath(null)
              }}
            />
          ) : (
            <span className={styles.name}>{node.name}</span>
          )}
          <span className={styles.rowActions}>
            <i
              className="codicon codicon-edit"
              title="重命名"
              onClick={(e) => {
                e.stopPropagation()
                startRename(node.path)
              }}
            />
            <i
              className="codicon codicon-trash"
              title="删除"
              onClick={(e) => {
                e.stopPropagation()
                handleDeleteFile(node.path)
              }}
            />
          </span>
        </div>
      )
    })

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
          <button
            title="对当前草稿在已部署 workspace 跑 plan-only 检测"
            disabled={manifestMissing}
            onClick={() => setRunOpen(true)}
          >
            <i className="codicon codicon-play" /> Run
          </button>
          <button
            title="把当前草稿固化为新的不可变版本"
            disabled={manifestMissing}
            onClick={() => setPublishOpen(true)}
          >
            <i className="codicon codicon-tag" /> 发布版本
          </button>
          <button
            className={styles.primary}
            title="把已发布版本部署到 Workspace"
            disabled={manifestMissing}
            onClick={() => setDeployOpen(true)}
          >
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
            <i
              className="codicon codicon-new-file"
              title="新建文件"
              onClick={startCreateFile}
            />
            <i
              className="codicon codicon-collapse-all"
              title="折叠全部目录"
              onClick={() => {
                // 折叠所有目录(把当前树里所有 dir path 塞进 collapsedDirs)
                const allDirs = new Set<string>()
                const walk = (nodes: TreeNode[]) => {
                  for (const n of nodes) {
                    if (n.type === 'dir') {
                      allDirs.add(n.path)
                      if (n.children) walk(n.children)
                    }
                  }
                }
                walk(fileTree)
                setCollapsedDirs(allDirs)
              }}
            />
            <i
              className="codicon codicon-refresh"
              title="刷新"
              onClick={() => {
                listFiles(ctx)
                  .then((items) => {
                    setManifestMissing(false)
                    setFiles(items)
                  })
                  .catch(() => setManifestMissing(true))
              }}
            />
          </span>
        </div>
        <div className={styles.project}>
          <i className="codicon codicon-chevron-down" />
          <span>{manifestId.toUpperCase()}</span>
        </div>
        <div
          className={`${styles.tree} ${dragOver ? styles.dragOver : ''}`}
          onDragOver={(e) => {
            e.preventDefault()
            if (!dragOver) setDragOver(true)
          }}
          onDragLeave={(e) => {
            // 仅当离开整个 tree 容器(而非内部子元素)时才取消高亮
            if (e.currentTarget === e.target) setDragOver(false)
          }}
          onDrop={(e) => {
            e.preventDefault()
            if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
              void handleDropFiles(e.dataTransfer.files)
            } else {
              setDragOver(false)
            }
          }}
        >
          {manifestMissing && (
            <div style={{ padding: '8px 12px', color: '#cca700', fontSize: 12, lineHeight: 1.5 }}>
              Manifest <code style={{ color: '#cccccc' }}>{manifestId}</code> 不存在或无权访问。
              <br />
              此页面是 vscode-api 集成的脚手架沙箱。
            </div>
          )}
          {!manifestMissing && files.length === 0 && creating === null && (
            <div style={{ padding: '8px 12px', color: '#858585', fontSize: 12 }}>
              草稿为空,点上方 + 新建文件,或在编辑器内输入即自动保存
            </div>
          )}

          {/* 内联新建行(VS Code 风格): + 后直接在树里出现输入框 */}
          {creating !== null && (
            <div className={styles.treeNode}>
              <span className={`${styles.chevron} ${styles.empty}`} />
              <span className={styles.icon}>
                <i className={`codicon ${iconClassFor(creating || 'x.tf')}`} />
              </span>
              <input
                className={styles.inlineInput}
                autoFocus
                value={creating}
                placeholder="文件名,如 main.tf"
                onChange={(e) => {
                  setCreating(e.target.value)
                  if (inlineError) setInlineError(null)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') void commitCreateFile()
                  else if (e.key === 'Escape') {
                    setCreating(null)
                    setInlineError(null)
                  }
                }}
                onBlur={() => {
                  // 失焦: 有内容则尝试提交,空则取消(VS Code 行为)
                  if ((creating || '').trim()) void commitCreateFile()
                  else setCreating(null)
                }}
              />
            </div>
          )}

          {renderTreeNodes(fileTree, 0)}

          {inlineError && (creating !== null || renamingPath !== null) && (
            <div style={{ padding: '2px 12px 6px 28px', color: '#f48771', fontSize: 11 }}>
              {inlineError}
            </div>
          )}
        </div>
      </div>

      <div className={styles.editorArea}>
        <div className={styles.tabs}>
          {openTabs.map((path) => (
            <div
              key={path}
              className={`${styles.tab} ${currentFile === path ? styles.active : ''} ${dirtyFiles.has(path) ? styles.dirty : ''}`}
              onClick={() => void openFile(path)}
            >
              <i className={`codicon ${iconClassFor(path)}`} />
              <span>{path.split('/').pop()}</span>
              {/* dirty 白点(有未保存修改时显示);非 dirty 时显示关闭按钮 */}
              <span className={styles.dirtyDot} />
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
          {/* 二进制文件只读视图(spec §5.2):覆盖在 Monaco 之上 */}
          {binaryView && currentFile === binaryView.path ? (
            <div className={styles.binaryView}>
              <i className="codicon codicon-file-binary" />
              <div>该文件是二进制文件,不在编辑器中显示。</div>
              <div className={styles.binaryMeta}>
                {binaryView.mime} · {formatBytes(binaryView.size)}
              </div>
            </div>
          ) : null}
          <div
            ref={containerRef}
            style={{ width: '100%', height: '100%', visibility: binaryView && currentFile === binaryView.path ? 'hidden' : 'visible' }}
          />
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

      {/* === 弹窗 === */}
      <PublishVersionDialog
        open={publishOpen}
        ctx={ctx}
        onClose={() => setPublishOpen(false)}
      />
      <DeployDialog
        open={deployOpen}
        ctx={ctx}
        onClose={() => setDeployOpen(false)}
      />
      <RunDialog
        open={runOpen}
        ctx={ctx}
        onClose={() => setRunOpen(false)}
      />
    </div>
  )
}

// ========== 工具 ==========

// 文件树节点。dir 的 path 是目录前缀(如 "modules/vpc"),file 的 path 是完整路径。
interface TreeNode {
  type: 'dir' | 'file'
  name: string // 该层显示名(目录段名 / 文件名)
  path: string
  children?: TreeNode[]
}

// buildFileTree 从扁平 path 列表构建目录树。spec §5.1/§22: 顶层目录无白名单限制,
// 任意命名/任意嵌套。目录在前、文件在后,各自按名字排序。
function buildFileTree(paths: string[]): TreeNode[] {
  type DirAcc = { dirs: Map<string, DirAcc>; files: string[] }
  const root: DirAcc = { dirs: new Map(), files: [] }

  for (const full of paths) {
    const parts = full.split('/')
    let node = root
    for (let i = 0; i < parts.length - 1; i++) {
      const seg = parts[i]
      if (!node.dirs.has(seg)) node.dirs.set(seg, { dirs: new Map(), files: [] })
      node = node.dirs.get(seg)!
    }
    node.files.push(full)
  }

  const build = (acc: DirAcc, prefix: string): TreeNode[] => {
    const dirNodes: TreeNode[] = [...acc.dirs.keys()]
      .sort((a, b) => a.localeCompare(b))
      .map((seg) => {
        const childPrefix = prefix ? `${prefix}/${seg}` : seg
        return {
          type: 'dir' as const,
          name: seg,
          path: childPrefix,
          children: build(acc.dirs.get(seg)!, childPrefix),
        }
      })
    const fileNodes: TreeNode[] = acc.files
      .sort((a, b) => a.localeCompare(b))
      .map((full) => ({
        type: 'file' as const,
        name: full.split('/').pop() || full,
        path: full,
      }))
    return [...dirNodes, ...fileNodes]
  }

  return build(root, '')
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

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
