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
import { useParams, useSearchParams } from 'react-router-dom'
import * as monaco from 'monaco-editor'
import 'monaco-editor/esm/vs/editor/editor.all.js'
import '@vscode/codicons/dist/codicon.css'
import { Modal, Form, Input, message } from 'antd'
import { ensureVscodeServicesReady } from './initServices'
import { registerHclLanguage } from './hclLanguage'
import { registerHclProviders } from './hclProviders'
import PublishVersionDialog from './PublishVersionDialog'
import DeployDialog from './DeployDialog'
import RunDialog from './RunDialog'
import {
  listFiles,
  readFile,
  putFile,
  deleteFile,
  moveFile,
  languageOfPath,
  type ManifestFileEntry,
  type ManifestEditorContext,
} from './manifestApi'
import styles from './ManifestEditorV2.module.css'

const AUTOSAVE_DEBOUNCE_MS = 1000

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error'

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
  const [newFileOpen, setNewFileOpen] = useState(false)
  const [newFileForm] = Form.useForm<{ path: string }>()
  const [renamingPath, setRenamingPath] = useState<string | null>(null)
  const [renameForm] = Form.useForm<{ to: string }>()
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
        // 注册 HCL 语言 + 高亮 (idempotent, 全进程一次)
        registerHclLanguage()
        // 注册 4 个 demo provider (Completion / Hover / InlayHint / CodeAction)
        registerHclProviders()
        editorRef.current = monaco.editor.create(containerRef.current, {
          value: '',
          language: 'plaintext',
          // 设 fallback monaco 默认主题 'vs-dark', 即使 vscode-api 主题加载失败也保持深色
          // (vscode-api 加载成功后会自动切到 'Default Dark+', 不冲突)
          theme: 'vs-dark',
          automaticLayout: true,
          fontFamily: 'Menlo, Monaco, "Cascadia Code", Consolas, "Courier New", monospace',
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

  // ========== 新建文件 ==========
  const handleCreateFile = useCallback(async () => {
    try {
      const { path } = await newFileForm.validateFields()
      // 草稿区私有,后端会同步建条目;先 PUT 空内容
      await putFile(ctx, path, '')
      message.success(`已创建 ${path}`)
      newFileForm.resetFields()
      setNewFileOpen(false)
      setManifestMissing(false)
      // 刷新文件树 + 自动打开
      const items = await listFiles(ctx)
      setFiles(items)
      void openFile(path)
    } catch (err: any) {
      if (err?.errorFields) return
      const msg = typeof err === 'string' ? err : err?.message
      if (msg) message.error(`创建失败: ${msg}`)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orgId, manifestId, newFileForm])

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

  // ========== 重命名文件 ==========
  const handleRenameFile = useCallback(async () => {
    const fromPath = renamingPath
    if (!fromPath) return
    try {
      const { to } = await renameForm.validateFields()
      if (to === fromPath) {
        setRenamingPath(null)
        return
      }
      await moveFile(ctx, fromPath, to)
      // 转移内存缓存
      const cached = fileContentCache.current.get(fromPath)
      fileContentCache.current.delete(fromPath)
      if (cached !== undefined) fileContentCache.current.set(to, cached)
      // 转移打开 tab
      setOpenTabs((prev) => prev.map((p) => (p === fromPath ? to : p)))
      if (currentFile === fromPath) {
        setCurrentFile(to)
      }
      const items = await listFiles(ctx)
      setFiles(items)
      message.success(`已重命名为 ${to}`)
      renameForm.resetFields()
      setRenamingPath(null)
    } catch (err: any) {
      if (err?.errorFields) return
      const msg = typeof err === 'string' ? err : err?.message
      if (msg) message.error(`重命名失败: ${msg}`)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orgId, manifestId, renamingPath, currentFile, renameForm])

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
              onClick={() => {
                newFileForm.resetFields()
                setNewFileOpen(true)
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
        <div className={styles.tree}>
          {manifestMissing && (
            <div style={{ padding: '8px 12px', color: '#cca700', fontSize: 12, lineHeight: 1.5 }}>
              Manifest <code style={{ color: '#cccccc' }}>{manifestId}</code> 不存在或无权访问。
              <br />
              此页面是 vscode-api 集成的脚手架沙箱。
            </div>
          )}
          {!manifestMissing && files.length === 0 && (
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
                <span className={styles.rowActions}>
                  <i
                    className="codicon codicon-edit"
                    title="重命名"
                    onClick={(e) => {
                      e.stopPropagation()
                      renameForm.setFieldsValue({ to: f.path })
                      setRenamingPath(f.path)
                    }}
                  />
                  <i
                    className="codicon codicon-trash"
                    title="删除"
                    onClick={(e) => {
                      e.stopPropagation()
                      handleDeleteFile(f.path)
                    }}
                  />
                </span>
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

      {/* 新建文件 */}
      <Modal
        title="新建文件"
        open={newFileOpen}
        onCancel={() => setNewFileOpen(false)}
        onOk={handleCreateFile}
        okText="创建"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={newFileForm} layout="vertical" preserve={false}>
          <Form.Item
            label="文件路径"
            name="path"
            rules={[
              { required: true, message: '请输入路径' },
              {
                pattern: /^[A-Za-z0-9_\-./]+$/,
                message: '只允许字母数字 _ - . / ,不允许空格或其他特殊字符',
              },
              {
                validator(_, value: string) {
                  if (!value) return Promise.resolve()
                  if (value.startsWith('/'))
                    return Promise.reject(new Error('不允许绝对路径'))
                  if (value.split('/').some((s) => s === '.' || s === '..'))
                    return Promise.reject(new Error('不允许 . 或 .. 路径段'))
                  if (value.length > 256)
                    return Promise.reject(new Error('路径过长 (>256)'))
                  return Promise.resolve()
                },
              },
            ]}
            extra="例如: main.tf / variables.tf / modules/vpc/main.tf"
          >
            <Input placeholder="main.tf" autoFocus />
          </Form.Item>
        </Form>
      </Modal>

      {/* 重命名文件 */}
      <Modal
        title={`重命名 ${renamingPath ?? ''}`}
        open={renamingPath !== null}
        onCancel={() => setRenamingPath(null)}
        onOk={handleRenameFile}
        okText="重命名"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={renameForm} layout="vertical" preserve={false}>
          <Form.Item
            label="新路径"
            name="to"
            rules={[
              { required: true, message: '请输入新路径' },
              {
                pattern: /^[A-Za-z0-9_\-./]+$/,
                message: '只允许字母数字 _ - . / ',
              },
            ]}
          >
            <Input autoFocus />
          </Form.Item>
        </Form>
      </Modal>
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
