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
import { useParams, useSearchParams, useNavigate } from 'react-router-dom'
import * as monaco from 'monaco-editor'
import 'monaco-editor/esm/vs/editor/editor.all.js'
import '@vscode/codicons/dist/codicon.css'
import { message } from 'antd'
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
  deleteDir,
  moveFile,
  listVersions,
  diffVersions,
  diffDraft,
  languageOfPath,
  type ManifestFileEntry,
  type ManifestEditorContext,
  type ManifestVersion,
  type DiffEntry,
} from './manifestApi'
import { exportManifestZip } from '../../../services/manifestApi'
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
  const navigate = useNavigate()
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
  // diff 视图:独立的 Monaco DiffEditor 实例 + 宿主容器(与普通编辑器并存,按需显隐)
  const diffContainerRef = useRef<HTMLDivElement | null>(null)
  const diffEditorRef = useRef<monaco.editor.IStandaloneDiffEditor | null>(null)

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
  // 内联删除确认:pendingDelete={path, isDir} → 该行变 "确认删除? ✓ ✗"
  // (antd v5 静态 Modal.confirm 在 React19 下静默失效,改树内联确认)
  const [pendingDelete, setPendingDelete] = useState<{ path: string; isDir: boolean } | null>(null)
  // 幽灵目录:新建文件夹时树里临时出现的空目录前缀(建文件后实体化,刷新即消失)
  const [ghostDir, setGhostDir] = useState<string | null>(null)
  // 内联新建目录输入:!== null 时 sidebar 顶部出现目录名输入行
  const [creatingDir, setCreatingDir] = useState<string | null>(null)
  // sidebar 当前视图:explorer(文件树) | history(版本历史)
  const [activeView, setActiveView] = useState<'explorer' | 'history'>('explorer')
  const [versions, setVersions] = useState<ManifestVersion[]>([])
  const [versionsLoading, setVersionsLoading] = useState(false)
  // diff tab: 打开一个左旧右新的对比视图。key 唯一标识,激活时显示 DiffEditor、隐藏普通编辑器。
  //   title 显示在 tab 上,如 "main.tf (草稿 ↔ v1.2.0)"
  type DiffTab = { key: string; title: string; path: string; leftRef: string; rightRef: string }
  const [diffTabs, setDiffTabs] = useState<DiffTab[]>([])
  const [activeDiffKey, setActiveDiffKey] = useState<string | null>(null)
  // 历史面板"未提交更改"区:当前用户草稿 vs 最新已发布版本
  const [draftDiff, setDraftDiff] = useState<{ baseVersionId: string; files: DiffEntry[] }>({ baseVersionId: '', files: [] })
  // 版本行展开:versionId -> 该版本 vs 上一版本的变更文件
  const [expandedVersion, setExpandedVersion] = useState<string | null>(null)
  const [versionDiffCache, setVersionDiffCache] = useState<Record<string, DiffEntry[]>>({})
  // 未提交更改行的内联撤销确认:pendingDiscard = 该文件 path(仅草稿区可撤销)
  const [pendingDiscard, setPendingDiscard] = useState<string | null>(null)

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
          // 关掉 Code Action 灯泡图标:它显示在行首 gutter,会和 source 等代码文字重叠。
          // Code Action provider 本身保留,用户仍可用 Cmd+. 或右键触发"应用 demo"。
          lightbulb: { enabled: monaco.editor.ShowLightbulbIconMode.Off },
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
            void flushSaveRef.current()
          }, AUTOSAVE_DEBOUNCE_MS)
        })

        // 劫持 Cmd/Ctrl+S:立即保存当前文件,阻止浏览器"保存网页"。
        // 用 ref 调最新 save 逻辑(addCommand 注册一次,闭包会过期)。
        editorRef.current.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
          void flushSaveRef.current()
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
      const dm = diffEditorRef.current?.getModel()
      dm?.original?.dispose()
      dm?.modified?.dispose()
      diffEditorRef.current?.dispose()
      diffEditorRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 窗口级 Cmd/Ctrl+S 拦截:焦点不在 Monaco 内(如文件树)时也阻止浏览器"保存网页"
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && (e.key === 's' || e.key === 'S')) {
        e.preventDefault()
        void flushSaveRef.current()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
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

  // flushSaveRef: 立即保存"当前 model 的内容到当前 path",取消 pending 防抖。
  // 用 ref 持有,避免一次性注册的编辑器监听器/快捷键拿到 stale 闭包。
  // 切文件前必须 flush,否则 1s 防抖未触发时切走会丢上一个文件的修改。
  const flushSaveRef = useRef<() => Promise<void>>(async () => {})

  const openFile = useCallback(
    async (path: string) => {
      // 等编辑器创建完成
      if (!editorRef.current) {
        await ensureVscodeServicesReady()
      }
      const ed = editorRef.current
      if (!ed) return

      // 打开普通文件 → 退出 diff 视图
      setActiveDiffKey(null)

      // 切到新文件前,先把上一个文件的待保存内容刷盘(防 1s 防抖窗口内切走丢改动)
      if (currentFileRef.current && currentFileRef.current !== path) {
        await flushSaveRef.current()
      }

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
            void openFile(fallback) // openFile 内部会先 flush 当前文件
          } else {
            // 关掉最后一个 tab:先 flush 再清空(防丢未保存改动)
            void flushSaveRef.current().then(() => {
              setCurrentFile(null)
              editorRef.current?.getModel()?.setValue('')
            })
          }
        }
        return next
      })
    },
    [currentFile, openFile],
  )

  // ========== 保存当前文件 ==========
  const saveCurrentFile = useCallback(async () => {
    // 取消 pending 防抖(无论是 flush 还是 Cmd+S 主动触发,都应吃掉排队的那次)
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current)
      saveTimerRef.current = null
    }
    const path = currentFileRef.current
    const ed = editorRef.current
    if (!path || !ed) return
    // dirty 才需要写;非 dirty 的 flush 直接跳过,省一次 PUT
    if (!dirtyFilesRef.current.has(path)) return
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

  // 让一次性注册的监听器/快捷键始终调到最新 saveCurrentFile
  const dirtyFilesRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    dirtyFilesRef.current = dirtyFiles
  }, [dirtyFiles])
  useEffect(() => {
    flushSaveRef.current = saveCurrentFile
  }, [saveCurrentFile])

  // 关闭编辑器(左上红灯):先 flush 当前文件,再返回 manifest 列表
  const handleClose = useCallback(async () => {
    await flushSaveRef.current()
    navigate('/admin/manifests')
  }, [navigate])

  // 拉已发布版本列表(切到历史视图 / 发布后刷新时调)
  const loadVersions = useCallback(() => {
    setVersionsLoading(true)
    listVersions(ctx)
      .then((vs) => setVersions(vs))
      .catch(() => setVersions([]))
      .finally(() => setVersionsLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ctx])

  // 拉"未提交更改"(草稿 vs 最新已发布版本),只取真正有变更的。
  // 先 flush 当前文件的待保存内容(autosave 是 1s 防抖,不 flush 会读到旧草稿、漏掉刚改的差异)。
  const loadDraftDiff = useCallback(async () => {
    await flushSaveRef.current()
    try {
      const r = await diffDraft(ctx)
      setDraftDiff({ baseVersionId: r.baseVersionId, files: r.files.filter((f) => f.state !== 'unchanged') })
    } catch {
      setDraftDiff({ baseVersionId: '', files: [] })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ctx])

  // 切到历史视图时拉版本列表 + 未提交更改
  useEffect(() => {
    if (activeView === 'history') {
      loadVersions()
      void loadDraftDiff()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeView])

  // 展开某版本 → 拉它 vs 上一版本(SemVer 排序紧邻的前一个)的变更文件
  const toggleVersionExpand = useCallback(
    (v: ManifestVersion) => {
      if (expandedVersion === v.id) {
        setExpandedVersion(null)
        return
      }
      setExpandedVersion(v.id)
      if (versionDiffCache[v.id]) return
      // versions 已按 SemVer 降序(新→旧),找紧邻的下一个(更旧)作为 base
      const idx = versions.findIndex((x) => x.id === v.id)
      const prev = idx >= 0 && idx < versions.length - 1 ? versions[idx + 1] : null
      if (!prev) {
        // 没有更早版本 → 该版本是首个,全部算新增(用空 base 对比)
        setVersionDiffCache((c) => ({ ...c, [v.id]: [] }))
        return
      }
      diffVersions(ctx, v.id, prev.id)
        .then((files) => setVersionDiffCache((c) => ({ ...c, [v.id]: files.filter((f) => f.state !== 'unchanged') })))
        .catch(() => setVersionDiffCache((c) => ({ ...c, [v.id]: [] })))
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [expandedVersion, versionDiffCache, versions, ctx],
  )

  // diff 宿主从 display:none 显示出来时,Monaco 需要重新 layout 才能正确铺满
  useEffect(() => {
    if (activeDiffKey) {
      requestAnimationFrame(() => diffEditorRef.current?.layout())
    }
  }, [activeDiffKey])

  // 导出某版本为 zip(走 api client 带 token,不能用裸 <a href> 否则 401)
  const exportVersion = useCallback(
    async (versionId: string, label: string) => {
      try {
        const blob = await exportManifestZip(String(orgId), manifestId, versionId)
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `${manifestId}-${label}.zip`
        document.body.appendChild(a)
        a.click()
        document.body.removeChild(a)
        URL.revokeObjectURL(url)
      } catch (err: any) {
        message.error(`导出失败: ${err?.message ?? err}`)
      }
    },
    [orgId, manifestId],
  )

  // 懒创建 Monaco DiffEditor(首次打开 diff 时),宿主 diffContainerRef
  const ensureDiffEditor = useCallback(() => {
    if (diffEditorRef.current || !diffContainerRef.current) return diffEditorRef.current
    diffEditorRef.current = monaco.editor.createDiffEditor(diffContainerRef.current, {
      theme: 'vs-dark-manifest',
      automaticLayout: true,
      readOnly: true, // diff 只读(对比用,不在此编辑)
      originalEditable: false,
      renderSideBySide: true,
      fontFamily: 'Menlo, Monaco, "Cascadia Code", Consolas, "Courier New", monospace',
    })
    return diffEditorRef.current
  }, [])

  // 打开一个 diff tab:左=base(rightRef 实际是 base 旧内容)... 统一约定:
  //   leftRef = 旧(base), rightRef = 新(target);Monaco original=左=旧,modified=右=新
  const openDiff = useCallback(
    async (path: string, leftRef: string, rightRef: string, title: string) => {
      const key = `diff::${leftRef}::${rightRef}::${path}`
      // 切走普通编辑器前先 flush(避免丢草稿改动)
      if (currentFileRef.current) await flushSaveRef.current()
      try {
        // 两侧内容:ref 为 '__absent__' 表示该侧不存在(added/removed)
        const fetchSide = async (ref: string): Promise<string> => {
          if (ref === '__absent__') return ''
          const f = await readFile(ctx, path, ref)
          return f.is_binary ? '(binary file)' : f.content ?? ''
        }
        const [leftContent, rightContent] = await Promise.all([fetchSide(leftRef), fetchSide(rightRef)])
        const lang = languageOfPath(path)
        const ed = ensureDiffEditor()
        if (!ed) return
        const oldModel = ed.getModel()
        ed.setModel({
          original: monaco.editor.createModel(leftContent, lang),
          modified: monaco.editor.createModel(rightContent, lang),
        })
        oldModel?.original?.dispose()
        oldModel?.modified?.dispose()
        setDiffTabs((prev) => (prev.some((t) => t.key === key) ? prev : [...prev, { key, title, path, leftRef, rightRef }]))
        setActiveDiffKey(key)
        setCurrentFile(null) // 普通编辑器让位
        setBinaryView(null)
      } catch (err: any) {
        message.error(`打开 diff 失败: ${err?.message ?? err}`)
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [ctx, ensureDiffEditor],
  )

  // 关闭 diff tab
  const closeDiffTab = useCallback((key: string) => {
    setDiffTabs((prev) => prev.filter((t) => t.key !== key))
    setActiveDiffKey((cur) => (cur === key ? null : cur))
  }, [])

  // 撤销某文件的未提交更改:把草稿里该文件还原到 base 版本(最新已发布)。
  //   changed → 用 base 内容覆盖草稿;added → 删草稿该文件;removed → 从 base 恢复到草稿。
  const discardDraftFile = useCallback(
    async (f: DiffEntry, baseRef: string) => {
      try {
        if (f.state === 'added') {
          // base 没有 → 删掉草稿里这个新增文件
          await deleteFile(ctx, f.path)
        } else {
          // changed / removed → 取 base 内容写回草稿
          if (!baseRef) throw new Error('无基线版本可还原')
          const base = await readFile(ctx, f.path, baseRef)
          if (base.is_binary) {
            await putFileB64(ctx, f.path, base.content_b64 ?? '')
          } else {
            await putFile(ctx, f.path, base.content ?? '')
          }
        }
        // 清缓存 + 若该文件正打开则刷新编辑器内容
        fileContentCache.current.delete(f.path)
        if (currentFileRef.current === f.path) {
          // 重新打开以加载还原后的内容(added 被删则关闭 tab)
          if (f.state === 'added') {
            setOpenTabs((prev) => prev.filter((p) => p !== f.path))
            setCurrentFile(null)
            editorRef.current?.getModel()?.setValue('')
          } else {
            void openFile(f.path)
          }
        }
        setPendingDiscard(null)
        // 刷新文件树 + 未提交更改列表
        const items = await listFiles(ctx)
        setFiles(items)
        void loadDraftDiff()
        message.success(`已撤销 ${f.path} 的更改`)
      } catch (err: any) {
        message.error(`撤销失败: ${err?.message ?? err}`)
        setPendingDiscard(null)
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [ctx],
  )

  // 渲染历史面板里的一条变更文件行。点击打开 diff(左=base 旧,右=target 新)。
  //   targetRef: 'draft' 或 version_id;baseRef: 对比基线 version_id(可空=无基线)
  //   added → base 侧不存在;removed → target 侧不存在
  //   仅草稿区(targetRef==='draft')显示撤销按钮(已发布版本不可变,不能撤销)
  const renderChangeRow = useCallback(
    (f: DiffEntry, targetRef: string, baseRef: string) => {
      const badge = { added: 'A', removed: 'D', changed: 'M', unchanged: ' ' }[f.state]
      const badgeColor = { added: '#4ec9b0', removed: '#f48771', changed: '#e2c08d', unchanged: '#858585' }[f.state]
      const leftRef = f.state === 'added' ? '__absent__' : baseRef || '__absent__'
      const rightRef = f.state === 'removed' ? '__absent__' : targetRef
      const baseLabel = baseRef ? baseRef.slice(0, 8) : '∅'
      const targetLabel = targetRef === 'draft' ? '草稿' : targetRef.slice(0, 8)
      const title = `${f.path.split('/').pop()} (${baseLabel} ↔ ${targetLabel})`
      const canDiscard = targetRef === 'draft'
      const confirming = pendingDiscard === f.path
      return (
        <div
          key={`${targetRef}:${f.path}`}
          className={styles.changeRow}
          title={f.path}
          onClick={() => void openDiff(f.path, leftRef, rightRef, title)}
        >
          <span className={styles.changeBadge} style={{ color: badgeColor }}>{badge}</span>
          <span className={styles.changeName}>{f.path}</span>
          {canDiscard && (
            confirming ? (
              <span className={styles.changeActions} style={{ opacity: 1 }} onClick={(e) => e.stopPropagation()}>
                <span style={{ color: '#e2c08d', fontSize: 11, marginRight: 2 }}>撤销?</span>
                <i className="codicon codicon-check" title="确认撤销" style={{ color: '#e2c08d' }} onClick={() => void discardDraftFile(f, baseRef)} />
                <i className="codicon codicon-close" title="取消" onClick={() => setPendingDiscard(null)} />
              </span>
            ) : (
              <span className={styles.changeActions}>
                <i
                  className="codicon codicon-discard"
                  title="撤销此文件的未提交更改(还原到最新版本)"
                  onClick={(e) => {
                    e.stopPropagation()
                    setPendingDiscard(f.path)
                  }}
                />
              </span>
            )
          )}
        </div>
      )
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [openDiff, pendingDiscard, discardDraftFile],
  )

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
    const raw = (creating || '').trim()
    // 在幽灵目录下新建时,自动加目录前缀
    const path = ghostDir && !raw.includes('/') ? `${ghostDir}/${raw}` : raw
    const errMsg = validatePath(path)
    if (errMsg) {
      setInlineError(errMsg)
      return
    }
    try {
      await putFile(ctx, path, '')
      setCreating(null)
      setGhostDir(null) // 文件已实体化,幽灵目录转正
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
  }, [creating, ghostDir, ctx, validatePath])

  const startCreateFile = useCallback(() => {
    setRenamingPath(null)
    setCreatingDir(null)
    setInlineError(null)
    setCreating('')
  }, [])

  // ========== 新建目录(幽灵目录)==========
  const startCreateDir = useCallback(() => {
    setRenamingPath(null)
    setCreating(null)
    setInlineError(null)
    setCreatingDir('')
  }, [])

  // 确认目录名 → 设为幽灵目录 + 立即在该目录下起新建文件输入
  const commitCreateDir = useCallback(() => {
    const dir = (creatingDir || '').trim().replace(/\/+$/, '')
    if (!dir) {
      setCreatingDir(null)
      return
    }
    // 校验目录路径(按文件路径规则,目录段不允许 . ..)
    const errMsg = validatePath(dir + '/placeholder')
    if (errMsg) {
      setInlineError(errMsg)
      return
    }
    setCreatingDir(null)
    setInlineError(null)
    setGhostDir(dir)
    setCollapsedDirs((prev) => {
      const next = new Set(prev)
      next.delete(dir) // 确保展开
      return next
    })
    setCreating('') // 直接进入"在该目录下新建文件"
  }, [creatingDir, validatePath])

  // ========== 删除文件 / 目录(内联确认)==========
  // 关闭"匹配 path(文件)或 path 前缀(目录)"的所有 tab + 清缓存
  const forgetPaths = useCallback(
    (match: (p: string) => boolean) => {
      fileContentCache.current.forEach((_v, k) => {
        if (match(k)) fileContentCache.current.delete(k)
      })
      setOpenTabs((prev) => prev.filter((p) => !match(p)))
      if (currentFileRef.current && match(currentFileRef.current)) {
        setCurrentFile(null)
        editorRef.current?.getModel()?.setValue('')
      }
    },
    [],
  )

  const confirmDelete = useCallback(async () => {
    const target = pendingDelete
    if (!target) return
    try {
      if (target.isDir) {
        await deleteDir(ctx, target.path)
        const prefix = target.path + '/'
        forgetPaths((p) => p === target.path || p.startsWith(prefix))
      } else {
        await deleteFile(ctx, target.path)
        forgetPaths((p) => p === target.path)
      }
      const items = await listFiles(ctx)
      setFiles(items)
      setPendingDelete(null)
    } catch (err: any) {
      const msg = typeof err === 'string' ? err : err?.message
      message.error(`删除失败: ${msg ?? '未知错误'}`)
      setPendingDelete(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingDelete, ctx, forgetPaths])

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

  const fileTree = useMemo(
    () => buildFileTree(files.map((f) => f.path), ghostDir),
    [files, ghostDir],
  )

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

  // 内联删除确认 UI(文件/目录共用):替换该行右侧操作区
  const deleteConfirmActions = (
    <span className={styles.rowActions} style={{ opacity: 1 }} onClick={(e) => e.stopPropagation()}>
      <span style={{ color: '#f48771', fontSize: 11, marginRight: 4 }}>删除?</span>
      <i className="codicon codicon-check" title="确认删除" style={{ color: '#f48771' }} onClick={() => void confirmDelete()} />
      <i className="codicon codicon-close" title="取消" onClick={() => setPendingDelete(null)} />
    </span>
  )

  // 新建文件输入行(在指定缩进下渲染,供根目录与幽灵目录复用)
  const newFileInputRow = (indent: number) => (
    <div className={styles.treeNode} style={{ paddingLeft: indent }}>
      <span className={`${styles.chevron} ${styles.empty}`} />
      <span className={styles.icon}>
        <i className={`codicon ${iconClassFor(creating || 'x.tf')}`} />
      </span>
      <input
        className={styles.inlineInput}
        autoFocus
        value={creating ?? ''}
        placeholder="文件名,如 main.tf"
        onChange={(e) => {
          setCreating(e.target.value)
          if (inlineError) setInlineError(null)
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') void commitCreateFile()
          else if (e.key === 'Escape') {
            setCreating(null)
            setGhostDir(null)
            setInlineError(null)
          }
        }}
        onBlur={() => {
          if ((creating || '').trim()) void commitCreateFile()
          else {
            setCreating(null)
            setGhostDir(null)
          }
        }}
      />
    </div>
  )

  // 递归渲染文件树节点。depth 控制缩进(每层 +12px,与 VS Code 一致)
  const renderTreeNodes = (nodes: TreeNode[], depth: number): ReactNode =>
    nodes.map((node) => {
      const indent = 8 + depth * 12
      if (node.type === 'dir') {
        const collapsed = collapsedDirs.has(node.path)
        const deleting = pendingDelete?.isDir && pendingDelete.path === node.path
        // 在该目录下新建文件:creating 进行中且 ghostDir 指向本目录
        const childInput = creating !== null && ghostDir === node.path
        return (
          <div key={`dir:${node.path}`}>
            <div
              className={`${styles.treeNode} ${deleting ? styles.deleting : ''}`}
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
              {deleting ? (
                deleteConfirmActions
              ) : (
                <span className={styles.rowActions}>
                  <i
                    className="codicon codicon-trash"
                    title="删除整个目录"
                    onClick={(e) => {
                      e.stopPropagation()
                      setPendingDelete({ path: node.path, isDir: true })
                    }}
                  />
                </span>
              )}
            </div>
            {!collapsed && node.children && renderTreeNodes(node.children, depth + 1)}
            {!collapsed && childInput && newFileInputRow(8 + (depth + 1) * 12)}
          </div>
        )
      }
      // file
      const deleting = !pendingDelete?.isDir && pendingDelete?.path === node.path
      return (
        <div
          key={`file:${node.path}`}
          className={`${styles.treeNode} ${currentFile === node.path ? styles.selected : ''} ${deleting ? styles.deleting : ''}`}
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
          {deleting ? (
            deleteConfirmActions
          ) : (
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
                  setPendingDelete({ path: node.path, isDir: false })
                }}
              />
            </span>
          )}
        </div>
      )
    })

  // ========== 渲染 ==========
  return (
    <div className={styles.root}>
      <div className={styles.titleBar}>
        <div className={styles.traffic}>
          <span
            className={styles.red}
            title="关闭编辑器(返回 Manifest 列表)"
            role="button"
            onClick={() => void handleClose()}
          />
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
        <div
          className={`${styles.item} ${activeView === 'explorer' ? styles.active : ''}`}
          title="资源管理器"
          role="button"
          onClick={() => setActiveView('explorer')}
        >
          <i className="codicon codicon-files" />
        </div>
        <div className={`${styles.item} ${styles.disabled}`} title="搜索 (用编辑器内 Cmd+F)">
          <i className="codicon codicon-search" />
        </div>
        <div className={`${styles.item} ${styles.disabled}`} title="源代码管理 (本期不启用 — 直接存 Postgres)">
          <i className="codicon codicon-source-control" />
        </div>
        <div
          className={`${styles.item} ${activeView === 'history' ? styles.active : ''}`}
          title="版本历史"
          role="button"
          onClick={() => setActiveView('history')}
        >
          <i className="codicon codicon-history" />
        </div>
        <div className={styles.spacer} />
        <div className={`${styles.item} ${styles.disabled}`} title="配置">
          <i className="codicon codicon-settings-gear" />
        </div>
      </div>

      <div className={styles.sideBar}>
        {activeView === 'explorer' && (
        <>
        <div className={styles.header}>
          <span>资源管理器</span>
          <span className={styles.actions}>
            <i
              className="codicon codicon-new-file"
              title="新建文件"
              onClick={startCreateFile}
            />
            <i
              className="codicon codicon-new-folder"
              title="新建文件夹"
              onClick={startCreateDir}
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
          {!manifestMissing && files.length === 0 && creating === null && creatingDir === null && ghostDir === null && (
            <div style={{ padding: '8px 12px', color: '#858585', fontSize: 12 }}>
              草稿为空,点上方 + 新建文件 / 新建文件夹,或在编辑器内输入即自动保存
            </div>
          )}

          {/* 内联新建目录输入行 */}
          {creatingDir !== null && (
            <div className={styles.treeNode} style={{ paddingLeft: 8 }}>
              <span className={styles.chevron}>
                <i className="codicon codicon-chevron-right" />
              </span>
              <span className={styles.icon}>
                <i className={`codicon codicon-folder ${styles.iconFolder}`} />
              </span>
              <input
                className={styles.inlineInput}
                autoFocus
                value={creatingDir}
                placeholder="目录名,如 modules/vpc"
                onChange={(e) => {
                  setCreatingDir(e.target.value)
                  if (inlineError) setInlineError(null)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') commitCreateDir()
                  else if (e.key === 'Escape') {
                    setCreatingDir(null)
                    setInlineError(null)
                  }
                }}
                onBlur={() => {
                  if ((creatingDir || '').trim()) commitCreateDir()
                  else setCreatingDir(null)
                }}
              />
            </div>
          )}

          {/* 根目录新建文件输入行(在幽灵目录下新建时不在这里,而在树内对应目录下)*/}
          {creating !== null && ghostDir === null && newFileInputRow(8)}

          {renderTreeNodes(fileTree, 0)}

          {inlineError && (creating !== null || renamingPath !== null) && (
            <div style={{ padding: '2px 12px 6px 28px', color: '#f48771', fontSize: 11 }}>
              {inlineError}
            </div>
          )}
        </div>
        </>
        )}

        {activeView === 'history' && (
          <>
            <div className={styles.header}>
              <span>版本历史</span>
              <span className={styles.actions}>
                <i
                  className="codicon codicon-refresh"
                  title="刷新"
                  onClick={() => {
                    loadVersions()
                    loadDraftDiff()
                  }}
                />
              </span>
            </div>
            <div className={styles.tree}>
              {/* 未提交更改:草稿 vs 最新已发布版本 */}
              <div className={styles.changesHeader}>
                未提交更改{draftDiff.files.length > 0 ? ` (${draftDiff.files.length})` : ''}
              </div>
              {draftDiff.files.length === 0 && (
                <div style={{ padding: '4px 12px 8px', color: '#858585', fontSize: 12 }}>
                  草稿与最新版本一致,无未提交更改。
                </div>
              )}
              {draftDiff.files.map((f) => renderChangeRow(f, 'draft', draftDiff.baseVersionId))}

              {/* 已发布版本 */}
              <div className={styles.changesHeader}>已发布版本</div>
              {versionsLoading && (
                <div style={{ padding: '8px 12px', color: '#858585', fontSize: 12 }}>加载中...</div>
              )}
              {!versionsLoading && versions.length === 0 && (
                <div style={{ padding: '8px 12px', color: '#858585', fontSize: 12, lineHeight: 1.5 }}>
                  还没有已发布版本。
                  <br />
                  点顶栏「发布版本」把当前草稿固化为 vX.Y.Z。
                </div>
              )}
              {!versionsLoading &&
                versions.map((v, idx) => {
                  const expanded = expandedVersion === v.id
                  const prev = idx < versions.length - 1 ? versions[idx + 1] : null
                  const changes = versionDiffCache[v.id]
                  return (
                    <div key={v.id} className={styles.versionRow}>
                      <div
                        className={styles.versionHead}
                        style={{ cursor: 'pointer' }}
                        onClick={() => toggleVersionExpand(v)}
                      >
                        <i className={`codicon ${expanded ? 'codicon-chevron-down' : 'codicon-chevron-right'}`} style={{ color: '#858585' }} />
                        <i className="codicon codicon-tag" style={{ color: '#4ec9b0' }} />
                        <span className={styles.versionTag}>{v.version}</span>
                        <i
                          className={`codicon codicon-cloud-download ${styles.versionExport}`}
                          title="导出该版本为 zip"
                          role="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            void exportVersion(v.id, v.version)
                          }}
                        />
                      </div>
                      {v.changelog && <div className={styles.versionChangelog}>{v.changelog}</div>}
                      <div className={styles.versionMeta}>
                        {v.created_by}
                        {v.created_at ? ` · ${new Date(v.created_at).toLocaleString()}` : ''}
                      </div>
                      {expanded && (
                        <div className={styles.versionChanges}>
                          {!prev && <div className={styles.changeEmpty}>首个版本,无可对比的上一版本。</div>}
                          {prev && changes === undefined && <div className={styles.changeEmpty}>加载中...</div>}
                          {prev && changes && changes.length === 0 && (
                            <div className={styles.changeEmpty}>与上一版本 {prev.version} 无文件变更。</div>
                          )}
                          {prev &&
                            changes &&
                            changes.map((f) => renderChangeRow(f, v.id, prev.id))}
                        </div>
                      )}
                    </div>
                  )
                })}
            </div>
          </>
        )}
      </div>

      <div className={styles.editorArea}>
        <div className={styles.tabs}>
          {openTabs.map((path) => (
            <div
              key={path}
              className={`${styles.tab} ${currentFile === path && !activeDiffKey ? styles.active : ''} ${dirtyFiles.has(path) ? styles.dirty : ''}`}
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
          {/* diff tabs */}
          {diffTabs.map((t) => (
            <div
              key={t.key}
              className={`${styles.tab} ${activeDiffKey === t.key ? styles.active : ''}`}
              onClick={() => setActiveDiffKey(t.key)}
              title={t.title}
            >
              <i className="codicon codicon-git-compare" />
              <span>{t.title}</span>
              <span
                className={styles.close}
                onClick={(e) => {
                  e.stopPropagation()
                  closeDiffTab(t.key)
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
          {!activeDiffKey && binaryView && currentFile === binaryView.path ? (
            <div className={styles.binaryView}>
              <i className="codicon codicon-file-binary" />
              <div>该文件是二进制文件,不在编辑器中显示。</div>
              <div className={styles.binaryMeta}>
                {binaryView.mime} · {formatBytes(binaryView.size)}
              </div>
            </div>
          ) : null}
          {/* diff 宿主:激活 diff tab 时显示,盖在普通编辑器之上 */}
          <div
            ref={diffContainerRef}
            style={{
              position: 'absolute',
              inset: 0,
              display: activeDiffKey ? 'block' : 'none',
            }}
          />
          <div
            ref={containerRef}
            style={{
              width: '100%',
              height: '100%',
              visibility:
                activeDiffKey || (binaryView && currentFile === binaryView.path) ? 'hidden' : 'visible',
            }}
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
        onPublished={() => {
          // 发布成功后刷新历史 + 未提交更改(发布后草稿通常与新版本一致)
          if (activeView === 'history') {
            loadVersions()
            loadDraftDiff()
          }
        }}
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
// ghostDir 非空时确保该空目录也出现在树里(新建目录的临时态)。
function buildFileTree(paths: string[], ghostDir?: string | null): TreeNode[] {
  type DirAcc = { dirs: Map<string, DirAcc>; files: string[] }
  const root: DirAcc = { dirs: new Map(), files: [] }

  const ensureDir = (segs: string[]) => {
    let node = root
    for (const seg of segs) {
      if (!node.dirs.has(seg)) node.dirs.set(seg, { dirs: new Map(), files: [] })
      node = node.dirs.get(seg)!
    }
    return node
  }

  for (const full of paths) {
    const parts = full.split('/')
    ensureDir(parts.slice(0, -1)).files.push(full)
  }
  // 幽灵目录:即使没有文件也建出目录节点
  if (ghostDir) ensureDir(ghostDir.split('/'))

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
