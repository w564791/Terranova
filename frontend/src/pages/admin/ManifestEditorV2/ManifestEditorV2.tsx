/**
 * Manifest Editor v2 — VS Code Web 工作区(B2 模式)
 *
 * 视觉 1:1 对齐 manifest-vscode-mockup.html demo,数据接 manifest_files API。
 *
 * 能力: layout shell + 文件树 + tab + Monaco(HCL 高亮 + 4 个 provider)接 manifest_files;
 * Toolbar 三按钮 Run / 发布 / 部署:RunDialog 和 DeployPanel 为右侧停靠面板(含 WebSocket 日志流),PublishVersionDialog 为弹窗。
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
import { registerHclCompletion, getProviderSchemaVersion, setProviderTypeCatalog } from './hclCompletion'
import { attachHclSuggestRetrigger } from './hclSuggestRetrigger'
import {
  registerHclDefinition,
  pathToManifestUri,
  manifestUriToPath,
  buildDefinitionIndex,
  indexFile,
  removePathFromIndex,
  emptyIndex,
  type DefinitionIndex,
} from './hclDefinitions'
import { computeHclDiagnostics, HCL_DIAG_OWNER } from './hclDiagnostics'
import PublishVersionDialog, { type PublishCheckSummary } from './PublishVersionDialog'
import CheckPanel from './CheckPanel'
import DeployPanel from './DeployPanel'
import RunDialog from './RunDialog'
import SearchPanel from './SearchPanel'
import ProblemsPanel, { type ProblemItem } from './ProblemsPanel'
import { collectManifestProblems } from './collectProblems'
import QuickOpen from './QuickOpen'
import TreeContextMenu, { type ContextMenuItem } from './TreeContextMenu'
import ManifestAiTools, { type EditorBridge, type CheckFile } from './ManifestAiTools'
import { buildBlockIndex, findExternalRefs, locateBlock } from './hclBlockIndex'
import {
  listFiles,
  listVersionFiles,
  readFile,
  putFile,
  putFileB64,
  deleteFile,
  deleteDir,
  moveFile,
  moveDir,
  listVersions,
  listDeployments,
  diffVersions,
  diffDraft,
  getProviderSchemas,
  languageOfPath,
  type ManifestFileEntry,
  type ManifestEditorContext,
  type ManifestVersion,
  type ManifestDeployment,
  type DiffEntry,
} from './manifestApi'
import { workspaceService } from '../../../services/workspaces'
import {
  checkManifestDraft,
  type ManifestIssue,
  type ManifestProgressEvent,
  type ManifestCompletedStep,
  type ConversationTurn,
} from '../../../services/manifestAi'
import { exportManifestZip, getManifest, updateManifest } from '../../../services/manifestApi'
import { AI_PANEL_WIDTH } from './manifestAiStyles'
import styles from './ManifestEditorV2.module.css'

const AUTOSAVE_DEBOUNCE_MS = 1000
// 单文件上限,与后端 MANIFEST_MAX_FILE_SIZE 默认值(1MB)对齐
const MANIFEST_MAX_FILE_SIZE = 1024 * 1024
type RightPanel = 'ai' | 'check' | 'run' | 'deploy'

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

// 递归遍历 FileSystemEntry(拖拽/粘贴的目录结构),收集所有文件及其相对路径
type TraversedFile = { relativePath: string; file: File }
async function traverseFileSystemEntry(entry: FileSystemEntry, basePath = ''): Promise<TraversedFile[]> {
  if (entry.isFile) {
    const fe = entry as FileSystemFileEntry
    const file = await new Promise<File>((resolve, reject) => fe.file(resolve, reject))
    const relativePath = basePath ? `${basePath}/${entry.name}` : entry.name
    return [{ relativePath, file }]
  }
  if (entry.isDirectory) {
    const de = entry as FileSystemDirectoryEntry
    const reader = de.createReader()
    const allEntries: FileSystemEntry[] = []
    try {
      // readEntries 可能分批返回,需循环读到空
      while (true) {
        const batch = await new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject))
        if (batch.length === 0) break
        allEntries.push(...batch)
      }
    } catch (err) {
      console.warn(`读取目录 ${entry.name} 失败,跳过:`, err)
      return []
    }
    const dirPath = basePath ? `${basePath}/${entry.name}` : entry.name
    const results: TraversedFile[] = []
    for (const child of allEntries) {
      try {
        results.push(...await traverseFileSystemEntry(child, dirPath))
      } catch (err) {
        console.warn(`遍历 ${child.name} 失败,跳过:`, err)
      }
    }
    return results
  }
  return []
}

// 同步提取 FileSystemEntry 列表(必须在事件处理器同步阶段调用,事件返回后 items 失效)
function collectFileEntries(items: DataTransferItemList): (FileSystemEntry | File)[] {
  const result: (FileSystemEntry | File)[] = []
  for (const item of Array.from(items)) {
    const entry = item.webkitGetAsEntry?.() ?? item.getAsEntry?.()
    if (entry) {
      result.push(entry)
    } else {
      const file = item.getAsFile?.()
      if (file) result.push(file)
    }
  }
  return result
}

// 异步遍历已提取的条目列表
async function traverseCollectedEntries(entries: (FileSystemEntry | File)[]): Promise<TraversedFile[]> {
  const all: TraversedFile[] = []
  for (const entry of entries) {
    if (entry instanceof File) {
      all.push({ relativePath: entry.name, file: entry })
    } else {
      all.push(...await traverseFileSystemEntry(entry))
    }
  }
  return all
}

function normalizeManifestSubpath(value: string | null): string {
  if (!value) return ''
  return value.replace(/\\/g, '/').replace(/^\/+|\/+$/g, '')
}

function isTopLevelTfUnderSubpath(path: string, subpath: string): boolean {
  if (!path.endsWith('.tf')) return false
  const cleanPath = path.replace(/\\/g, '/').replace(/^\/+/, '')
  const cleanSubpath = normalizeManifestSubpath(subpath)
  let relativePath = cleanPath
  if (cleanSubpath) {
    if (!cleanPath.startsWith(`${cleanSubpath}/`)) return false
    relativePath = cleanPath.slice(cleanSubpath.length + 1)
  }
  return relativePath.length > 0 && !relativePath.includes('/')
}

function isHclBlockHeader(line: string): boolean {
  const text = line.trim()
  return text.startsWith('module ') ||
    text.startsWith('resource ') ||
    text.startsWith('data ') ||
    text.startsWith('variable ') ||
    text.startsWith('output ') ||
    text.startsWith('locals ') ||
    text === 'locals {' ||
    text.startsWith('provider ') ||
    text.startsWith('terraform ')
}

function firstNonEmptyLine(text: string): string {
  return text.split('\n').find((line) => line.trim()) ?? ''
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
  const ctx: ManifestEditorContext = useMemo(() => ({ orgId, manifestId }), [orgId, manifestId])

  const rootRef = useRef<HTMLDivElement | null>(null) // 根容器(全屏目标)
  const containerRef = useRef<HTMLDivElement | null>(null)
  // 右侧面板系统:AI 生成 / 检查 / Run / 部署 四选一(互斥),共享同一个宽度状态
  const [activeRightPanel, setActiveRightPanel] = useState<RightPanel | null>(null)
  const [rightPanelWidth, setRightPanelWidth] = useState(AI_PANEL_WIDTH)
  const [checkBusy, setCheckBusy] = useState(false)
  const [checkIssues, setCheckIssues] = useState<ManifestIssue[]>([])
  const [checkCompletedSteps, setCheckCompletedSteps] = useState<ManifestCompletedStep[]>([])
  const [checkError, setCheckError] = useState<string | null>(null)
  const [checkCurrentStep, setCheckCurrentStep] = useState('')
  const [checkContext, setCheckContext] = useState<{ kind: 'selection' | 'file'; filePath: string; startLine?: number; endLine?: number } | null>(null)
  const checkAbortRef = useRef<AbortController | null>(null)
  // 发布弹窗读取此状态决定是否解锁表单
  const [publishCheckSummary, setPublishCheckSummary] = useState<PublishCheckSummary | null>(null)
  const treeRef = useRef<HTMLDivElement | null>(null) // 文件树容器(键盘导航需聚焦它才收 keydown)
  const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)
  const fileContentCache = useRef<Map<string, string>>(new Map())
  // 文件变更追踪:用于在文件树显示 U(新建)/M(修改) 标记
  const originalFilesRef = useRef<Set<string>>(new Set()) // 初始加载时的文件路径集合
  const originalContentRef = useRef<Map<string, string>>(new Map()) // 文件首次加载时的原始内容
  const [newFiles, setNewFiles] = useState<Set<string>>(new Set()) // 本次会话新建的文件
  const [modifiedFiles, setModifiedFiles] = useState<Set<string>>(new Set()) // 内容被修改过的文件
  // Gutter diff 指示条:对比上次发布版本的变更标记
  const baseVersionIdRef = useRef<string>('') // 上次发布的版本 ID
  const publishedContentCache = useRef<Map<string, string>>(new Map()) // path → 发布版内容(仅 changed 文件)
  const diffFilesSetRef = useRef<Set<string>>(new Set()) // diff 结果中 changed/added 的文件路径集合
  const diffDecorationsRef = useRef<string[]>([]) // 当前 decoration IDs(deltaDecorations 返回值)
  // 每文件一个 model(保留 undo 历史)+ viewState(保留光标/滚动),切 tab 回到原状态
  const modelCache = useRef<Map<string, monaco.editor.ITextModel>>(new Map())
  const viewStateCache = useRef<Map<string, monaco.editor.ICodeEditorViewState | null>>(new Map())
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // diff 视图:独立的 Monaco DiffEditor 实例 + 宿主容器(与普通编辑器并存,按需显隐)
  const diffContainerRef = useRef<HTMLDivElement | null>(null)
  const diffEditorRef = useRef<monaco.editor.IStandaloneDiffEditor | null>(null)
  // 跨文件「转到定义」索引(var./local. → 定义位置)。provider 读 ref.current 拿最新。
  const defIndexRef = useRef<DefinitionIndex>(emptyIndex())
  // 全量索引是否已建好至少一次(决定诊断是否报"未定义引用",避免索引未就绪时误报)
  const defIndexReadyRef = useRef(false)
  // editor opener(跨文件跳转路由)的 disposable,卸载时释放
  const openerDisposableRef = useRef<monaco.IDisposable | null>(null)
  // var. 退格后 re-trigger 补全
  const suggestRetriggerRef = useRef<monaco.IDisposable | null>(null)
  // openFile 的 ref:一次性注册的 opener 需调最新 openFile,避免 stale 闭包
  const openFileRef = useRef<(path: string) => Promise<void>>(async () => {})
  // 是否带深链参数(?file/?resource):有则首文件不自动打开,让深链 effect 定位。
  // 同步初始化(不能等深链 effect,否则 listFiles 先 resolve 会抢开首文件)。
  const hasDeepLinkRef = useRef(!!(searchParams.get('file') || searchParams.get('resource')))
  // 对某 .tf model 跑诊断并 setModelMarkers(用 ref 供 model 监听器调,读最新索引)
  const runDiagnosticsRef = useRef<(model: monaco.editor.ITextModel) => void>(() => {})
  runDiagnosticsRef.current = (model: monaco.editor.ITextModel) => {
    const markers = computeHclDiagnostics(model.getValue(), defIndexRef.current, defIndexReadyRef.current)
    monaco.editor.setModelMarkers(model, HCL_DIAG_OWNER, markers)
    // markers 变更后刷新 Problems 列表(ref 调最新)
    refreshProblemsRef.current()
  }
  const refreshProblemsRef = useRef<() => void>(() => {})
  refreshProblemsRef.current = () => {
    // 节流:合并同帧多次诊断刷新
    const tick = ++problemTickRef.current
    requestAnimationFrame(() => {
      if (tick !== problemTickRef.current) return
      setProblems(collectManifestProblems(manifestUriToPath))
    })
  }

  // ===== Gutter Diff 指示条工具函数 =====
  // 计算行级 diff:返回每行的状态 (added/modified/unchanged)
  // publishedContent: 发布版内容(仅 changed 文件有值);isInDiff: 该文件是否在 diff 结果中
  const computeLineDiff = useCallback((currentContent: string, publishedContent: string | undefined, isInDiff: boolean): ('added' | 'modified' | 'unchanged')[] => {
    const currentLines = currentContent.split('\n')
    if (!isInDiff) {
      // 文件不在 diff 结果中 → 与发布版相同,全部 unchanged
      return currentLines.map(() => 'unchanged')
    }
    if (!publishedContent) {
      // 新文件(在 diff 中但无发布版内容):所有行都是 added
      return currentLines.map(() => 'added')
    }
    const publishedLines = publishedContent.split('\n')
    const m = currentLines.length
    const n = publishedLines.length

    // LCS DP
    const dp: number[][] = Array(m + 1).fill(null).map(() => Array(n + 1).fill(0))
    for (let i = 1; i <= m; i++) {
      for (let j = 1; j <= n; j++) {
        if (currentLines[i - 1] === publishedLines[j - 1]) {
          dp[i][j] = dp[i - 1][j - 1] + 1
        } else {
          dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1])
        }
      }
    }

    // 正向回溯产出有序操作序列(match/insert/delete),并尽量让匹配落在最左
    // (左对齐)——旧的反向回溯会贪心匹配最右的同内容行,导致重复行/空行被
    // 错配成 unchanged(新行无色)。
    type Op = { t: 'match' | 'insert'; c: number } | { t: 'delete'; c?: undefined }
    const ops: Op[] = []
    let i = m, j = n
    while (i > 0 || j > 0) {
      if (i > 0 && j > 0 && currentLines[i - 1] === publishedLines[j - 1]) {
        ops.push({ t: 'match', c: i - 1 }); i--; j--
      } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
        ops.push({ t: 'delete' }); j--
      } else {
        ops.push({ t: 'insert', c: i - 1 }); i--
      }
    }
    ops.reverse()

    // 按改动 hunk 配对分类:
    //   连续的非 match 操作为一个 hunk;
    //   hunk 内既有 insert 又有 delete → 这些 insert 行是 modified(替换);
    //   只有 insert → added(纯新增,无对应旧行)。
    // 旧的 pubIdx 单游标会把所有未匹配行无脑当 modified,导致纯新增/空行被错标。
    const result: ('added' | 'modified' | 'unchanged')[] = new Array(m).fill('unchanged')
    let k = 0
    while (k < ops.length) {
      if (ops[k].t === 'match') { k++; continue }
      const group: Op[] = []
      let hasIns = false
      let hasDel = false
      while (k < ops.length && ops[k].t !== 'match') {
        group.push(ops[k])
        hasIns = hasIns || ops[k].t === 'insert'
        hasDel = hasDel || ops[k].t === 'delete'
        k++
      }
      const cls: 'added' | 'modified' = hasIns && hasDel ? 'modified' : 'added'
      for (const op of group) if (op.t === 'insert') result[op.c] = cls
    }

    return result
  }, [])

  // 应用 diff decorations 到编辑器
  const applyDiffDecorations = useCallback((path: string) => {
    const ed = editorRef.current
    if (!ed) return

    const model = ed.getModel()
    if (!model) return

    // 没有 base version(未发布过) → 清除装饰
    if (!baseVersionIdRef.current) {
      diffDecorationsRef.current = ed.deltaDecorations(diffDecorationsRef.current, [])
      return
    }

    const isInDiff = diffFilesSetRef.current.has(path)
    if (!isInDiff) {
      // 文件不在 diff 结果中 → 无变更,清除装饰
      diffDecorationsRef.current = ed.deltaDecorations(diffDecorationsRef.current, [])
      return
    }

    const currentContent = model.getValue()
    const publishedContent = publishedContentCache.current.get(path)
    const lineDiff = computeLineDiff(currentContent, publishedContent, isInDiff)

    const decorations: monaco.editor.IModelDeltaDecoration[] = []
    lineDiff.forEach((status, lineIdx) => {
      if (status === 'added') {
        decorations.push({
          range: new monaco.Range(lineIdx + 1, 1, lineIdx + 1, 1),
          options: {
            isWholeLine: true,
            linesDecorationsClassName: styles.lineAdded,
            overviewRuler: { color: '#73c991', position: monaco.editor.OverviewRulerLane.Left },
          },
        })
      } else if (status === 'modified') {
        decorations.push({
          range: new monaco.Range(lineIdx + 1, 1, lineIdx + 1, 1),
          options: {
            isWholeLine: true,
            linesDecorationsClassName: styles.lineModified,
            overviewRuler: { color: '#e2c08d', position: monaco.editor.OverviewRulerLane.Left },
          },
        })
      }
    })

    diffDecorationsRef.current = ed.deltaDecorations(diffDecorationsRef.current, decorations)
  }, [computeLineDiff])

  const [bootError, setBootError] = useState<string | null>(null)
  const [manifestMissing, setManifestMissing] = useState(false)
  const [publishOpen, setPublishOpen] = useState(false)
  const [runViewLast, setRunViewLast] = useState(false) // 打开 Run 面板时直接跳到上次任务
  const [lastRunTask, setLastRunTask] = useState<{ taskId: number; workspaceId: string } | null>(null)
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
  // 文件树右键菜单:{x,y,target}。target 描述右键对象(文件/目录/空白)
  const [contextMenu, setContextMenu] = useState<{
    x: number
    y: number
    target: { kind: 'file' | 'dir' | 'blank'; path: string }
  } | null>(null)
  // 编辑器 tab 右键菜单:{x,y,path}(被右键的 tab)
  const [tabMenu, setTabMenu] = useState<{ x: number; y: number; path: string } | null>(null)
  // 文件树键盘导航当前焦点节点(path)
  const [focusedPath, setFocusedPath] = useState<string | null>(null)
  // 拖拽移动:正在拖的节点 + 当前 hover 的放置目标目录(高亮用)
  const draggingRef = useRef<{ path: string; isDir: boolean } | null>(null)
  const [dragOverDir, setDragOverDir] = useState<string | null>(null)
  // 复制/剪切剪贴板(单文件)。菜单在右键打开时即时读取 ref,无需 state 驱动重渲染。
  const clipboardRef = useRef<{ path: string; mode: 'copy' | 'cut' } | null>(null)
  // 粘贴/拖拽上传进度:{current, total, fileName} 或 null(无进行中)
  const [pasteProgress, setPasteProgress] = useState<{ current: number; total: number; fileName: string } | null>(null)
  // 跨文件导航历史栈(Cmd+←/→)。NavLoc 记 path + 光标/滚动 viewState
  const navStackRef = useRef<{
    back: { path: string; viewState: monaco.editor.ICodeEditorViewState | null }[]
    fwd: { path: string; viewState: monaco.editor.ICodeEditorViewState | null }[]
  }>({ back: [], fwd: [] })
  // sidebar 当前视图:explorer(文件树) | search(搜索) | deploy(已部署 workspace) | history(版本历史)
  const [activeView, setActiveView] = useState<'explorer' | 'search' | 'problems' | 'deploy' | 'history'>('explorer')
  const [searchShowReplace, setSearchShowReplace] = useState(false) // Cmd+Shift+H 进来时默认展开替换
  const [quickOpen, setQuickOpen] = useState(false)
  const [problems, setProblems] = useState<ProblemItem[]>([])
  const problemTickRef = useRef(0)
  const gutterDiffTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // 侧栏宽度可拖拽(VS Code 行为),限 170–600px
  const [sidebarWidth, setSidebarWidth] = useState(260)
  const [versions, setVersions] = useState<ManifestVersion[]>([])
  const [versionsLoading, setVersionsLoading] = useState(false)
  // 已部署 workspace(active deployment)+ workspace 名称映射,用于左侧"部署"视图与 DRAFT 徽标
  const [deployments, setDeployments] = useState<ManifestDeployment[]>([])
  const [wsNameById, setWsNameById] = useState<Record<string, string>>({})
  const [deployLoading, setDeployLoading] = useState(false)
  // post_init 落库的 provider schema 版本(状态栏);无缓存为 —
  const [schemaVersionLabel, setSchemaVersionLabel] = useState<string>('—')
  // manifest 元信息(名称/描述),顶栏就地编辑。null=未加载
  const [manifestName, setManifestName] = useState<string>('')
  const [manifestDesc, setManifestDesc] = useState<string>('')
  // 顶栏就地编辑:'name' | 'desc' | null
  const [editingMeta, setEditingMeta] = useState<'name' | 'desc' | null>(null)
  const [metaDraft, setMetaDraft] = useState('')
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
        // 通用 HCL 补全 (跨文件工作区索引 + Tier1/Tier3)
        registerHclCompletion({ getIndex: () => defIndexRef.current })
        // 转到定义:var/local/module/resource/data + hover
        registerHclDefinition({ getIndex: () => defIndexRef.current })
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
          // ===== VS Code 基础编辑体验补齐 =====
          // off:避免全文单词候选淹没 HCL 关键字/snippet(改完后体感"补全变乱/变难用"的主因之一)
          wordBasedSuggestions: 'off',
          // strings:true — resource "/data " 引号内要出类型补全;false 时引号里几乎不弹建议
          quickSuggestions: { other: true, comments: false, strings: true },
          suggestOnTriggerCharacters: true,
          suggestSelection: 'first',
          folding: true, // 代码折叠
          foldingStrategy: 'indentation', // HCL 无 LSP,按缩进折叠
          stickyScroll: { enabled: true }, // 顶部显示当前嵌套块(resource/module)上下文
          scrollBeyondLastLine: true,
          renderWhitespace: 'selection',
          cursorBlinking: 'smooth',
          autoClosingBrackets: 'languageDefined', // 括号/引号自动闭合(规则来自 hclLanguage 配置)
          tabSize: 2,
          insertSpaces: true, // 与 DEFAULT_USER_CONFIG 一致,双保险
        })
        editorRef.current.onDidChangeCursorPosition((e) => {
          setCursor({ line: e.position.lineNumber, col: e.position.column })
        })
        // 退格把 var.. → var. 时 Monaco 不会自动再弹补全;主动 re-trigger
        suggestRetriggerRef.current?.dispose()
        suggestRetriggerRef.current = attachHclSuggestRetrigger(editorRef.current)
        // 注:脏检测不挂编辑器实例,而是每个 model 自己 onDidChangeContent(openFile 建 model 时绑定)。
        // 编辑器级监听依赖 currentFileRef,一旦 diff 让位/撤销/删除把 currentFile 置 null 就整条失效。

        // 劫持 Cmd/Ctrl+S:立即保存当前文件,阻止浏览器"保存网页"。
        // 用 ref 调最新 save 逻辑(addCommand 注册一次,闭包会过期)。
        editorRef.current.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
          void flushSaveRef.current()
        })
        // Cmd/Ctrl+W:关闭当前 tab(diff tab 或普通文件 tab),用 ref 避免 stale 闭包
        editorRef.current.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyW, () => {
          closeCurrentTabRef.current()
        })
        // Cmd/Ctrl+/ 切换行注释:Monaco 内置 action,显式绑定确保生效
        editorRef.current.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Slash, () => {
          editorRef.current?.getAction('editor.action.commentLine')?.run()
        })
        // Cmd/Ctrl+Shift+F 跨文件搜索;Cmd/Ctrl+Shift+H 跨文件替换(用 ref 调最新逻辑)
        editorRef.current.addCommand(
          monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.KeyF,
          () => openSearchRef.current(false),
        )
        editorRef.current.addCommand(
          monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.KeyH,
          () => openSearchRef.current(true),
        )
        // Alt+←/→ 跨文件导航回退/前进(对齐 VS Code;保留 Cmd/Ctrl+←/→ 行首行尾肌肉记忆)
        editorRef.current.addCommand(monaco.KeyMod.Alt | monaco.KeyCode.LeftArrow, () =>
          navBackRef.current(),
        )
        editorRef.current.addCommand(monaco.KeyMod.Alt | monaco.KeyCode.RightArrow, () =>
          navForwardRef.current(),
        )
        // Cmd/Ctrl+P 快速打开文件
        editorRef.current.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyP, () => {
          setQuickOpen(true)
        })

        // 跨文件「转到定义」路由:目标 model 非当前文件时(manifest:/<path>),
        // 拦截跳转 → 用 openFile 打开目标文件 → 定位到定义行列。同文件跳转 monaco 原生处理。
        openerDisposableRef.current = monaco.editor.registerEditorOpener({
          openCodeEditor: async (_source, resource, selectionOrPosition) => {
            const path = manifestUriToPath(resource)
            if (path === null) return false // 非 manifest 资源,交还默认处理
            await openFileRef.current(path)
            const ed = editorRef.current
            if (ed && selectionOrPosition) {
              if (monaco.Range.isIRange(selectionOrPosition)) {
                ed.setSelection(selectionOrPosition)
                ed.revealRangeInCenter(selectionOrPosition)
              } else {
                ed.setPosition(selectionOrPosition)
                ed.revealPositionInCenter(selectionOrPosition)
              }
            }
            ed?.focus()
            return true
          },
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
      suggestRetriggerRef.current?.dispose()
      suggestRetriggerRef.current = null
      editorRef.current?.dispose()
      editorRef.current = null
      const dm = diffEditorRef.current?.getModel()
      dm?.original?.dispose()
      dm?.modified?.dispose()
      diffEditorRef.current?.dispose()
      diffEditorRef.current = null
      openerDisposableRef.current?.dispose()
      openerDisposableRef.current = null
      // 释放所有缓存的 model
      modelCache.current.forEach((m) => m.dispose())
      modelCache.current.clear()
      viewStateCache.current.clear()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 窗口级快捷键:Cmd/Ctrl+S 保存;Cmd/Ctrl+P 快速打开;Alt+←/→ 导航
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && (e.key === 's' || e.key === 'S')) {
        e.preventDefault()
        void flushSaveRef.current()
      }
      if ((e.metaKey || e.ctrlKey) && (e.key === 'p' || e.key === 'P') && !e.shiftKey) {
        e.preventDefault()
        setQuickOpen(true)
      }
      if (e.altKey && e.key === 'ArrowLeft' && !e.metaKey && !e.ctrlKey) {
        e.preventDefault()
        navBackRef.current()
      }
      if (e.altKey && e.key === 'ArrowRight' && !e.metaKey && !e.ctrlKey) {
        e.preventDefault()
        navForwardRef.current()
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
        // 记录初始文件列表(用于区分新建/已有文件)
        originalFilesRef.current = new Set(items.map((f) => f.path))
        // 深链(?file=&line= / ?resource=<id>)交给专门的 effect 处理;此处仅在无深链时打开首文件
        if (hasDeepLinkRef.current) return
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

  // ========== 加载上次发布版本(用于 gutter diff 指示条) ==========
  useEffect(() => {
    let cancelled = false
    diffDraft(ctx)
      .then((result) => {
        if (cancelled) return
        baseVersionIdRef.current = result.baseVersionId
        // 记录所有 changed/added 的文件路径(用于区分"新文件" vs "未变更文件")
        const diffFiles = result.files.filter((f) => f.state === 'changed' || f.state === 'added')
        diffFilesSetRef.current = new Set(diffFiles.map((f) => f.path))
        // 预加载所有 changed 文件的发布版内容
        return Promise.all(
          diffFiles.map(async (f) => {
            if (f.state === 'added') {
              // 新增文件:发布版不存在,缓存空串
              publishedContentCache.current.set(f.path, '')
            } else if (baseVersionIdRef.current) {
              try {
                const content = await readFile(ctx, f.path, baseVersionIdRef.current)
                publishedContentCache.current.set(f.path, content.content ?? '')
              } catch {
                // 读取失败:忽略,该文件不会有 diff 标记
              }
            }
          }),
        )
      })
      .then(() => {
        if (cancelled) return
        // diff 加载完成后,对当前打开的文件重新应用 gutter 装饰(解决竞态:文件可能在 diff 之前打开)
        if (currentFileRef.current) {
          applyDiffDecorations(currentFileRef.current)
        }
      })
      .catch(() => {
        // diffDraft 失败(可能没有已发布版本):忽略,gutter 不会有标记
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
  // 关闭"当前" tab(diff 优先,否则当前文件),用 ref 给一次性注册的快捷键调最新逻辑
  const closeCurrentTabRef = useRef<() => void>(() => {})
  // 打开搜索视图(withReplace=true 默认展开替换),用 ref 给快捷键调
  const openSearchRef = useRef<(withReplace: boolean) => void>(() => {})
  // 导航回退/前进的 ref(一次性注册的快捷键调最新逻辑)
  const navBackRef = useRef<() => void>(() => {})
  const navForwardRef = useRef<() => void>(() => {})

  const openFile = useCallback(
    async (path: string, opts?: { fromNav?: boolean }) => {
      // 等编辑器创建完成
      if (!editorRef.current) {
        await ensureVscodeServicesReady()
      }
      const ed = editorRef.current
      if (!ed) return

      // 打开普通文件 → 退出 diff 视图
      setActiveDiffKey(null)

      // 切到新文件前:① flush 上个文件待保存内容 ② 存它的 viewState(光标/滚动)
      if (currentFileRef.current && currentFileRef.current !== path) {
        await flushSaveRef.current()
        const vs = ed.saveViewState()
        viewStateCache.current.set(currentFileRef.current, vs)
        // 导航历史:正常跳转(非回退/前进)时,记录离开前的位置,清空 fwd
        if (!opts?.fromNav) {
          navStackRef.current.back.push({ path: currentFileRef.current, viewState: vs })
          if (navStackRef.current.back.length > 50) navStackRef.current.back.shift()
          navStackRef.current.fwd = []
        }
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

      // model 复用:命中缓存直接用(保留 undo 历史 + 内容);否则读内容建 model
      let model = modelCache.current.get(path)
      if (!model) {
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
            // 记录原始内容(用于判断文件是否被修改)
            if (!originalContentRef.current.has(path)) {
              originalContentRef.current.set(path, content)
            }
          } catch (err) {
            // eslint-disable-next-line no-console
            console.error('[ManifestEditorV2] read file failed', err)
            return
          }
        }
        // 用可反解路径的稳定 URI(manifest:/<path>),让跨文件「转到定义」能定位目标文件。
        // 复用已存在的同 URI model(理论上不会有,兜底防重复 URI 抛错)。
        const uri = pathToManifestUri(path)
        model = monaco.editor.getModel(uri) ?? monaco.editor.createModel(content, languageOfPath(path), uri)
        modelCache.current.set(path, model)
        const m = model
        // 脏检测挂在 model 上,path 由闭包捕获 —— 不依赖 currentFileRef,
        // 即使 currentFile 被置 null(diff 让位/撤销/删除)也能正确标记。
        m.onDidChangeContent(() => {
          setDirtyFiles((prev) => {
            if (prev.has(path)) return prev
            const next = new Set(prev)
            next.add(path)
            return next
          })
          // 追踪文件修改状态(与原始内容对比)
          const currentContent = m.getValue()
          const originalContent = originalContentRef.current.get(path)
          const isModified = originalContent !== undefined && currentContent !== originalContent
          setModifiedFiles((prev) => {
            const wasModified = prev.has(path)
            if (isModified === wasModified) return prev
            const next = new Set(prev)
            if (isModified) next.add(path)
            else next.delete(path)
            return next
          })
          // 增量更新「转到定义」索引:该文件的 var/local 定义随编辑实时刷新
          if (path.endsWith('.tf')) {
            removePathFromIndex(defIndexRef.current, path)
            indexFile(defIndexRef.current, path, m.getValue())
            runDiagnosticsRef.current(m) // 索引更新后再算诊断(未定义引用依赖最新索引)
          }
          // Gutter diff 防抖:大文件 LCS 昂贵,避免每键全量重算
          if (gutterDiffTimerRef.current) clearTimeout(gutterDiffTimerRef.current)
          gutterDiffTimerRef.current = setTimeout(() => {
            applyDiffDecorations(path)
            refreshProblemsRef.current()
          }, 250)
          setSaveStatus('saving')
          if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
          saveTimerRef.current = setTimeout(() => {
            void flushSaveRef.current()
          }, AUTOSAVE_DEBOUNCE_MS)
        })
        // 打开即跑一次诊断(已建好初始索引;markers 在文件再打开时也会重算)
        if (path.endsWith('.tf')) runDiagnosticsRef.current(m)
      }

      // 不再 dispose 旧 model(由 modelCache 持有,关 tab 时才 dispose)
      ed.setModel(model)
      const vs = viewStateCache.current.get(path)
      if (vs) ed.restoreViewState(vs)
      ed.focus()

      // Gutter diff 指示条:对比上次发布版本,在行号旁显示新增/修改标记
      applyDiffDecorations(path)
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [orgId, manifestId, files],
  )

  // 搜索结果点击:打开文件并定位到行列(选中匹配片段 + 滚动居中)
  const openAt = useCallback(
    async (path: string, line: number, column: number, endColumn: number) => {
      await openFile(path)
      const ed = editorRef.current
      if (!ed) return
      const range = new monaco.Range(line, column, line, endColumn)
      ed.setSelection(range)
      ed.revealRangeInCenter(range)
      ed.focus()
    },
    [openFile],
  )

  // ========== 深链定位:?file=&line= 或 ?resource=<id>(来自 workspace 资源跳转)==========
  // ?file=path&line=N  直接定位;?resource=module.x / aws_xxx.y  解析块起始行后定位。
  // files 就绪后跑一次(标记 hasDeepLink 让首文件自动打开让位)。
  // 记录上次处理的深链 key,支持同一编辑器内连续跳转不同资源/文件。
  const deepLinkDoneRef = useRef<string | null>(null)
  useEffect(() => {
    const fileParam = searchParams.get('file')
    const resourceParam = searchParams.get('resource')
    if (!fileParam && !resourceParam) return
    hasDeepLinkRef.current = true
    const versionParam = searchParams.get('version')
    const hasSubpathParam = searchParams.has('subpath')
    const subpathParam = normalizeManifestSubpath(searchParams.get('subpath'))
    const deepLinkKey = fileParam
      ? `file:${fileParam}:${searchParams.get('line') || '1'}`
      : `resource:${resourceParam}:version:${versionParam || 'draft'}:subpath:${hasSubpathParam ? subpathParam : '*'}`
    if (deepLinkDoneRef.current === deepLinkKey || files.length === 0) return
    deepLinkDoneRef.current = deepLinkKey

    void (async () => {
      if (fileParam) {
        const line = parseInt(searchParams.get('line') || '1', 10) || 1
        await openAt(fileParam, line, 1, 1)
        return
      }
      // ?resource=<id>:先在草稿中查找;找不到则 fallback 到发布版本文件
      const entries = await collectDraftTfFilesRef.current()
      const scopedEntries = hasSubpathParam
        ? entries.filter((entry) => isTopLevelTfUnderSubpath(entry.path, subpathParam))
        : entries
      const index = buildBlockIndex(scopedEntries)
      const loc = locateBlock(index, resourceParam!)
      if (loc) {
        await openAt(loc.file, loc.line, 1, 1)
        return
      }
      // 草稿中找不到,尝试从发布版本文件中查找(?version= 来自 workspace 跳转)
      if (versionParam) {
        try {
          const versionEntries = await listVersionFiles(ctx, versionParam)
          const vFiles = await Promise.all(
            versionEntries
              .filter(
                (f) =>
                  f.type === 'file' &&
                  f.path.endsWith('.tf') &&
                  !f.is_binary &&
                  (!hasSubpathParam || isTopLevelTfUnderSubpath(f.path, subpathParam)),
              )
              .map(async (f) => {
                try {
                  const r = await readFile(ctx, f.path, versionParam)
                  return { path: f.path, content: r.is_binary ? '' : r.content ?? '' }
                } catch {
                  return { path: f.path, content: '' }
                }
              }),
          )
          const vIndex = buildBlockIndex(vFiles)
          const vLoc = locateBlock(vIndex, resourceParam!)
          if (vLoc) {
            // 在发布版本中找到:打开对应草稿文件(如果存在)并定位到该版本中的行号
            const draftFile = files.find((f) => f.path === vLoc.file)
            if (draftFile) {
              await openAt(vLoc.file, vLoc.line, 1, 1)
            } else {
              // 草稿中没有这个文件(发布版本新增的文件),直接打开发布版本内容
              await openAt(vLoc.file, vLoc.line, 1, 1)
            }
            return
          }
        } catch {
          // 获取发布版本文件失败,忽略
        }
      }
      // 都找不到:退而打开首个 .tf,并提示
      const firstTf = files.find((f) => f.path.endsWith('.tf')) ?? files[0]
      if (firstTf) await openFile(firstTf.path)
      message.warning(`未在 Manifest 中找到资源 ${resourceParam}`)
    })()
  }, [files, searchParams, openAt, openFile, ctx])

  // ========== 跨文件导航回退/前进(Cmd+←/→)==========
  // 跳到历史位置:打开目标文件(fromNav 避免再次记录)并恢复其 viewState
  const goToNavLoc = useCallback(
    async (loc: { path: string; viewState: monaco.editor.ICodeEditorViewState | null }) => {
      await openFile(loc.path, { fromNav: true })
      const ed = editorRef.current
      if (ed && loc.viewState) ed.restoreViewState(loc.viewState)
      ed?.focus()
    },
    [openFile],
  )
  const navigateBack = useCallback(() => {
    const st = navStackRef.current
    if (st.back.length === 0) return
    const ed = editorRef.current
    // 当前位置压入 fwd,再回退
    if (ed && currentFileRef.current) {
      st.fwd.push({ path: currentFileRef.current, viewState: ed.saveViewState() })
    }
    const loc = st.back.pop()!
    void goToNavLoc(loc)
  }, [goToNavLoc])
  const navigateForward = useCallback(() => {
    const st = navStackRef.current
    if (st.fwd.length === 0) return
    const ed = editorRef.current
    if (ed && currentFileRef.current) {
      st.back.push({ path: currentFileRef.current, viewState: ed.saveViewState() })
    }
    const loc = st.fwd.pop()!
    void goToNavLoc(loc)
  }, [goToNavLoc])

  // 全局替换落库后:被改文件的缓存 model 过期 → dispose,重开/未提交 diff 自然刷新
  const refreshAfterReplace = useCallback(
    (changedPaths: string[]) => {
      for (const p of changedPaths) {
        const isOpen = currentFileRef.current === p
        if (isOpen) editorRef.current?.setModel(null)
        const m = modelCache.current.get(p)
        if (m) {
          m.dispose()
          modelCache.current.delete(p)
        }
        fileContentCache.current.delete(p)
        viewStateCache.current.delete(p)
        if (isOpen) {
          setCurrentFile(null)
          void openFile(p)
        }
      }
    },
    [openFile],
  )

  // 释放某 path 的编辑器资源(model dispose + 清三类缓存),关 tab / 删文件时调
  const disposeFileResources = useCallback((path: string) => {
    const m = modelCache.current.get(path)
    if (m) {
      m.dispose()
      modelCache.current.delete(path)
    }
    viewStateCache.current.delete(path)
    fileContentCache.current.delete(path)
  }, [])

  // ========== 关闭 tab ==========
  const closeTab = useCallback(
    (path: string) => {
      setOpenTabs((prev) => {
        const next = prev.filter((p) => p !== path)
        if (path === currentFile) {
          const fallback = next[0] ?? null
          if (fallback) {
            // 先切到 fallback(openFile 会 flush + setModel 到 fallback),再 dispose 被关文件的 model
            void openFile(fallback).then(() => disposeFileResources(path))
          } else {
            // 关掉最后一个 tab:先 flush 再清空,然后 dispose
            void flushSaveRef.current().then(() => {
              setCurrentFile(null)
              editorRef.current?.setModel(null)
              disposeFileResources(path)
            })
          }
        } else {
          // 关的不是当前文件,直接 dispose
          disposeFileResources(path)
        }
        return next
      })
    },
    [currentFile, openFile, disposeFileResources],
  )

  // 批量关闭一组 tab(关闭其他/左侧/右侧/全部/全部已保存 共用)。
  // toClose: 要关闭的 path 集合。会 dispose 资源,并把 currentFile 切到剩余 tab(无则清空)。
  const closeTabs = useCallback(
    (toClose: string[]) => {
      if (toClose.length === 0) return
      const closeSet = new Set(toClose)
      setOpenTabs((prev) => {
        const next = prev.filter((p) => !closeSet.has(p))
        const curClosed = currentFileRef.current != null && closeSet.has(currentFileRef.current)
        if (curClosed) {
          const fallback = next[0] ?? null
          if (fallback) {
            void openFile(fallback).then(() => toClose.forEach(disposeFileResources))
          } else {
            void flushSaveRef.current().then(() => {
              setCurrentFile(null)
              editorRef.current?.setModel(null)
              toClose.forEach(disposeFileResources)
            })
          }
        } else {
          toClose.forEach(disposeFileResources)
        }
        return next
      })
    },
    [openFile, disposeFileResources],
  )

  // tab 右键菜单的各动作(基于右键的那个 tab path)
  const closeOtherTabs = useCallback(
    (keep: string) => closeTabs(openTabs.filter((p) => p !== keep)),
    [openTabs, closeTabs],
  )
  const closeTabsToRight = useCallback(
    (path: string) => {
      const i = openTabs.indexOf(path)
      if (i >= 0) closeTabs(openTabs.slice(i + 1))
    },
    [openTabs, closeTabs],
  )
  const closeTabsToLeft = useCallback(
    (path: string) => {
      const i = openTabs.indexOf(path)
      if (i > 0) closeTabs(openTabs.slice(0, i))
    },
    [openTabs, closeTabs],
  )
  const closeSavedTabs = useCallback(
    () => closeTabs(openTabs.filter((p) => !dirtyFiles.has(p))),
    [openTabs, dirtyFiles, closeTabs],
  )
  const closeAllTabs = useCallback(() => closeTabs([...openTabs]), [openTabs, closeTabs])

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
      // 历史视图下:草稿已落库,刷新"未提交更改"列表(直接拉 diff,不走 loadDraftDiff 以免再次 flush 递归)
      if (activeViewRef.current === 'history') {
        try {
          const r = await diffDraft(ctx)
          setDraftDiff({ baseVersionId: r.baseVersionId, files: r.files.filter((f) => f.state !== 'unchanged') })
        } catch {
          /* 刷新失败不影响保存本身 */
        }
      }
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
  // autosave 回调里读当前视图(决定是否刷新未提交列表),用 ref 避免 stale 闭包
  const activeViewRef = useRef(activeView)
  useEffect(() => {
    activeViewRef.current = activeView
  }, [activeView])
  useEffect(() => {
    flushSaveRef.current = saveCurrentFile
  }, [saveCurrentFile])
  // 让一次性注册的 editor opener(跨文件跳转)始终调到最新 openFile
  useEffect(() => {
    openFileRef.current = openFile
  }, [openFile])
  // 让一次性注册的 Cmd+←/→ 始终调最新导航逻辑
  useEffect(() => {
    navBackRef.current = navigateBack
    navForwardRef.current = navigateForward
  }, [navigateBack, navigateForward])

  // 右侧面板宽度变化(展开/折叠)后,让 Monaco 重算尺寸,避免错位
  useEffect(() => {
    const id = requestAnimationFrame(() => {
      editorRef.current?.layout()
      diffEditorRef.current?.layout()
    })
    return () => cancelAnimationFrame(id)
  }, [activeRightPanel, rightPanelWidth])

  // AI 工具桥接:用现有 editorRef / openAt 拼出 EditorBridge,供 ManifestAiTools 解耦调用
  const aiBridge: EditorBridge = useMemo(
    () => ({
      contextIds: { organization_id: String(orgId) },
      manifestId,
      orgId: String(orgId),
      getActiveFilePath: () => currentFileRef.current,
      getSelectionInfo: () => {
        const ed = editorRef.current
        const sel = ed?.getSelection()
        if (!ed || !sel || sel.isEmpty()) return null
        const text = ed.getModel()?.getValueInRange(sel) ?? ''
        if (!text) return null
        return {
          text,
          filePath: currentFileRef.current ?? '(unknown)',
          startLine: sel.startLineNumber,
          endLine: sel.endLineNumber,
        }
      },
      getActiveFileContent: () => editorRef.current?.getModel()?.getValue() ?? '',
      insertText: (text: string) => {
        const ed = editorRef.current
        if (!ed) return
        const sel = ed.getSelection()
        if (!sel) return
        ed.executeEdits('manifest-ai', [{ range: sel, text, forceMoveMarkers: true }])
        ed.focus()
      },
      revealAt: (path: string, line: number) => {
        void openAt(path, line, 1, 1)
      },
      onSelectionChange: (cb) => {
        const ed = editorRef.current
        if (!ed) return () => {}
        const d = ed.onDidChangeCursorSelection((e) => cb(!e.selection.isEmpty()))
        return () => d.dispose()
      },
      // 收集 check 文件:当前文件 + 其跨文件引用所在文件(≤MAX_CROSS_FILES)
      collectCheckFiles: async () => {
        const cur = currentFileRef.current
        const curContent = editorRef.current?.getModel()?.getValue() ?? ''
        if (!cur) return []
        const out: CheckFile[] = [{ path: cur, content: curContent, startLine: 1 }]
        try {
          const entries = await collectDraftTfFilesRef.current() // 全部 .tf 的最新内容
          const index = buildBlockIndex(entries)
          const externals = findExternalRefs(cur, curContent, index)
          const MAX_CROSS_FILES = 5
          for (const path of externals.slice(0, MAX_CROSS_FILES)) {
            const e = entries.find((x) => x.path === path)
            if (e) out.push({ path, content: e.content, startLine: 1 })
          }
          if (externals.length > MAX_CROSS_FILES) {
            console.warn(`[manifest-ai] 跨文件检查关联文件 ${externals.length} 个超上限,仅带前 ${MAX_CROSS_FILES} 个`)
          }
        } catch (e) {
          console.warn('[manifest-ai] 收集跨文件失败,降级为仅当前文件:', e)
        }
        return out
      },
      // 应用修复:目标文件可能非当前文件,确保打开后按行范围替换。
      // AI 给的行号可能基于旧内容/越界,这里严格校验,越界直接拒绝(不盲目替换以免改坏草稿)。
      applyFix: async (fix) => {
        await openFile(fix.file)
        const ed = editorRef.current
        const model = ed?.getModel()
        if (!ed || !model) throw new Error('编辑器未就绪')
        const lineCount = model.getLineCount()
        if (
          fix.startLine < 1 ||
          fix.endLine < fix.startLine ||
          fix.startLine > lineCount ||
          fix.endLine > lineCount
        ) {
          throw new Error(`修复行号超出文件范围(${fix.startLine}-${fix.endLine},共 ${lineCount} 行),请重新检查`)
        }
        const touchesBlockHeader = Array.from(
          { length: fix.endLine - fix.startLine + 1 },
          (_, idx) => model.getLineContent(fix.startLine + idx),
        ).some(isHclBlockHeader)
        if (touchesBlockHeader && !isHclBlockHeader(firstNonEmptyLine(fix.newText))) {
          throw new Error('AI 修复定位不可靠:目标范围包含 Terraform 块声明行,但替换内容不是完整块。请重新检查或手动修改。')
        }
        const endLineLen = model.getLineMaxColumn(fix.endLine)
        const range = new monaco.Range(fix.startLine, 1, fix.endLine, endLineLen)
        ed.executeEdits('manifest-ai-fix', [{ range, text: fix.newText, forceMoveMarkers: true }])
        ed.revealRangeInCenter(range)
        ed.focus()
      },
    }),
    [orgId, manifestId, openAt, openFile, ctx],
  )

  // Check 面板打开时持续跟踪选区/文件变化，动态更新 context chip
  useEffect(() => {
    if (activeRightPanel !== 'check') return

    // 面板刚打开时初始化一次
    const updateCtx = () => {
      const sel = aiBridge.getSelectionInfo()
      if (sel) {
        setCheckContext({
          kind: 'selection',
          filePath: sel.filePath,
          startLine: sel.startLine,
          endLine: sel.endLine,
        })
      } else {
        const filePath = aiBridge.getActiveFilePath()
        setCheckContext(filePath ? { kind: 'file', filePath } : null)
      }
    }
    updateCtx()

    // 订阅选区变化
    const unsubscribe = aiBridge.onSelectionChange(() => {
      updateCtx()
    })

    return () => {
      unsubscribe()
    }
  }, [activeRightPanel, aiBridge])

  // 切换文件时也更新 context chip(currentFile 变化不触发 onSelectionChange)
  useEffect(() => {
    if (activeRightPanel !== 'check') return
    const sel = aiBridge.getSelectionInfo()
    if (sel) {
      setCheckContext({
        kind: 'selection',
        filePath: sel.filePath,
        startLine: sel.startLine,
        endLine: sel.endLine,
      })
    } else {
      setCheckContext(currentFile ? { kind: 'file', filePath: currentFile } : null)
    }
  }, [activeRightPanel, currentFile, aiBridge])

  // 拉所有 .tf 文件最新内容(已打开优先用 live model 含未保存改动,其余 readFile)。
  // 供「转到定义」索引和 AI 跨文件检查共用。
  const collectDraftTfFiles = useCallback(async (): Promise<{ path: string; content: string }[]> => {
    const tfFiles = files.filter((f) => f.type === 'file' && f.path.endsWith('.tf') && !f.is_binary)
    return Promise.all(
      tfFiles.map(async (f) => {
        const model = modelCache.current.get(f.path)
        if (model) return { path: f.path, content: model.getValue() }
        try {
          const r = await readFile(ctx, f.path)
          return { path: f.path, content: r.is_binary ? '' : r.content ?? '' }
        } catch {
          return { path: f.path, content: '' }
        }
      }),
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [files, orgId, manifestId])

  // 让 bridge 始终调到最新的收集逻辑(避免 useMemo 依赖 files 频繁重建 bridge)
  const collectDraftTfFilesRef = useRef(collectDraftTfFiles)
  useEffect(() => {
    collectDraftTfFilesRef.current = collectDraftTfFiles
  }, [collectDraftTfFiles])

  // ===== 检查面板(AI 检查 + 发布前检查共用右侧面板)=====

  /** 核心检查执行:收集文件 → 调 API → 更新面板状态 */
  const runCheckCore = useCallback(
    async (source: 'ai' | 'publish', instruction?: string, history?: ConversationTurn[], sessionId?: string) => {
      setCheckBusy(true)
      setCheckIssues([])
      setCheckCompletedSteps([])
      setCheckError(null)
      setCheckCurrentStep('')
      setActiveRightPanel('check')

      const controller = new AbortController()
      checkAbortRef.current = controller

      try {
        // AI 检查:有选区则只检查选区;无选区则当前文件 + 跨文件引用
        // 发布检查:始终全量(collectCheckFiles 已处理)
        let files: { path: string; content: string; start_line: number }[]
        if (source === 'ai') {
          const sel = aiBridge.getSelectionInfo()
          const filePath = aiBridge.getActiveFilePath()
          if (sel && sel.text.trim() && filePath) {
            files = [{ path: filePath, content: sel.text, start_line: sel.startLine }]
          } else {
            const collected = await aiBridge.collectCheckFiles()
            if (collected.length === 0 || !collected.some((f) => f.content.trim())) {
              setCheckBusy(false)
              setCheckError('没有可检查的文件内容')
              return []
            }
            files = collected.map((f) => ({ path: f.path, content: f.content, start_line: f.startLine }))
          }
        } else {
          const collected = await aiBridge.collectCheckFiles()
          if (collected.length === 0 || !collected.some((f) => f.content.trim())) {
            setCheckBusy(false)
            setCheckError('没有可检查的文件内容')
            return []
          }
          files = collected.map((f) => ({ path: f.path, content: f.content, start_line: f.startLine }))
        }

        const result = await checkManifestDraft(
          {
            files,
            contextIds: aiBridge.contextIds,
            userInstruction: source === 'ai' ? instruction : undefined,
            history: source === 'ai' ? history : undefined,
            sessionId: source === 'ai' ? sessionId : undefined,
          },
          (ev: ManifestProgressEvent) => {
            setCheckCurrentStep(ev.step_name || '')
            if (ev.completed_steps?.length) setCheckCompletedSteps(ev.completed_steps)
          },
          controller.signal,
        )
        const issues = result.issues || []
        setCheckIssues(issues)
        if (result.completedSteps?.length) setCheckCompletedSteps(result.completedSteps)
        return issues
      } catch (e) {
        if (e instanceof DOMException && e.name === 'AbortError') return []
        setCheckError(e instanceof Error ? e.message : '检查失败')
        return []
      } finally {
        setCheckBusy(false)
        checkAbortRef.current = null
      }
    },
    [aiBridge],
  )

  /** AI 工具栏触发的检查(接受用户指令、历史、会话ID) */
  const runAiCheck = useCallback(async (instruction?: string, history?: ConversationTurn[], sessionId?: string) => {
    await runCheckCore('ai', instruction, history, sessionId)
  }, [runCheckCore])

  /** 发布弹窗触发的检查 */
  const runPublishCheck = useCallback(async () => {
    const issues = await runCheckCore('publish')
    setPublishCheckSummary({ done: true, skipped: false, issues })
  }, [runCheckCore])

  const handleCheckApplyFix = useCallback(
    async (issue: ManifestIssue) => {
      if (!issue.fix) return
      try {
        await aiBridge.applyFix({
          file: issue.fix.file,
          startLine: issue.fix.start_line,
          endLine: issue.fix.end_line,
          newText: issue.fix.new_text,
        })
      } catch (e) {
        setCheckError(e instanceof Error ? e.message : '应用修复失败')
      }
    },
    [aiBridge],
  )

  const handleCheckPanelClose = useCallback(() => {
    checkAbortRef.current?.abort()
    setActiveRightPanel((prev) => (prev === 'check' ? null : prev))
  }, [])

  // 全量重建「转到定义」索引。文件树结构变化(新建/删除/重命名/刷新)后跑一次;
  // 编辑中的增量更新见 model.onDidChangeContent。
  const rebuildDefIndex = useCallback(async () => {
    const entries = await collectDraftTfFiles()
    defIndexRef.current = buildDefinitionIndex(entries)
    defIndexReadyRef.current = true // 全量索引就绪,之后诊断可报"未定义引用"
    // 索引重建后,对所有已打开的 .tf model 重算诊断(跨文件:A 改定义影响 B 的未定义引用)
    modelCache.current.forEach((m, p) => {
      if (p.endsWith('.tf')) runDiagnosticsRef.current(m)
    })
  }, [collectDraftTfFiles])

  useEffect(() => {
    void rebuildDefIndex()
  }, [rebuildDefIndex])

  // 让 Cmd+W 快捷键始终调最新的"关当前 tab"逻辑(diff tab 优先,否则当前文件)
  useEffect(() => {
    closeCurrentTabRef.current = () => {
      if (activeDiffKey) closeDiffTab(activeDiffKey)
      else if (currentFileRef.current) closeTab(currentFileRef.current)
    }
  })

  // Cmd+Shift+F/H:切到搜索视图(H 默认展开替换)
  useEffect(() => {
    openSearchRef.current = (withReplace: boolean) => {
      setSearchShowReplace(withReplace)
      setActiveView('search')
    }
  })

  // 窗口级 Cmd/Ctrl+W 拦截:阻止浏览器关闭标签页,改为关编辑器内当前 tab
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && (e.key === 'w' || e.key === 'W')) {
        e.preventDefault()
        closeCurrentTabRef.current()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  // 侧栏宽度拖拽:按住分隔条横向拖,限 170–600px
  const startSidebarResize = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    const startX = e.clientX
    const startW = sidebarWidth
    const onMove = (ev: MouseEvent) => {
      const w = Math.min(600, Math.max(170, startW + (ev.clientX - startX)))
      setSidebarWidth(w)
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      editorRef.current?.layout() // 拖完让 Monaco 重新布局
    }
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [sidebarWidth])

  // 右侧面板宽度拖拽:按住左缘分隔条横向拖,限 250–700px
  const startPanelResize = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    const startX = e.clientX
    const startW = rightPanelWidth
    const onMove = (ev: MouseEvent) => {
      const w = Math.min(700, Math.max(250, startW - (ev.clientX - startX)))
      setRightPanelWidth(w)
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      editorRef.current?.layout()
    }
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [rightPanelWidth])

  // 关闭编辑器(左上红灯):先 flush 当前文件,再返回 manifest 列表
  const handleClose = useCallback(async () => {
    await flushSaveRef.current()
    navigate('/admin/manifests')
  }, [navigate])

  // 全屏(左上绿灯):切换浏览器全屏(整个编辑器页面)
  const toggleFullscreen = useCallback(() => {
    const el = rootRef.current ?? document.documentElement
    if (document.fullscreenElement) {
      void document.exitFullscreen?.()
    } else {
      void el.requestFullscreen?.()
    }
  }, [])

  // 拉已发布版本列表(切到历史视图 / 发布后刷新时调)
  const loadVersions = useCallback(() => {
    setVersionsLoading(true)
    listVersions(ctx)
      .then((vs) => setVersions(vs))
      .catch(() => setVersions([]))
      .finally(() => setVersionsLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ctx])

  // 拉已部署 workspace(active deployment)+ 对应 workspace 名称映射
  const loadDeployments = useCallback(() => {
    setDeployLoading(true)
    Promise.all([
      listDeployments(ctx).catch(() => [] as ManifestDeployment[]),
      workspaceService
        .getWorkspaces()
        .then((r) => {
          const d: any = (r as any)?.data
          return Array.isArray(d?.items) ? d.items : Array.isArray(d) ? d : []
        })
        .catch(() => [] as any[]),
    ])
      .then(([deps, wss]) => {
        setDeployments(deps.filter((d) => d.status === 'active'))
        const map: Record<string, string> = {}
        ;(wss as any[]).forEach((w) => {
          const id = w.workspace_id || String(w.id)
          map[id] = w.name || id
        })
        setWsNameById(map)
      })
      .finally(() => setDeployLoading(false))
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

  // 挂载即拉版本 + 部署 + manifest 元信息(名称/描述,供顶栏就地编辑)
  useEffect(() => {
    loadVersions()
    loadDeployments()
    getManifest(String(orgId), manifestId)
      .then((m) => {
        setManifestName(m.name ?? '')
        setManifestDesc(m.description ?? '')
      })
      .catch(() => {
        setManifestName('')
        setManifestDesc('')
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [manifestId, orgId])

  // 拉取 post_init 落库的 provider 类型目录(默认根 subpath;部署 workspace 的 subpath 优先)
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      let subpath = ''
      try {
        const deps = await listDeployments(ctx).catch(() => [] as ManifestDeployment[])
        const active = deps.filter((d) => d.status === 'active')
        if (active.length > 0) {
          const wss = await workspaceService
            .getWorkspaces()
            .then((r) => {
              const d: any = (r as any)?.data
              return Array.isArray(d?.items) ? d.items : Array.isArray(d) ? d : []
            })
            .catch(() => [] as any[])
          const ws = (wss as any[]).find(
            (w) => (w.workspace_id || String(w.id)) === active[0].workspace_id,
          )
          if (ws?.manifest_subpath) subpath = String(ws.manifest_subpath)
        }
      } catch {
        /* use root */
      }
      try {
        const schema = await getProviderSchemas(ctx, subpath)
        if (cancelled) return
        setProviderTypeCatalog(schema)
        setSchemaVersionLabel(getProviderSchemaVersion())
      } catch {
        if (cancelled) return
        setProviderTypeCatalog(null)
        setSchemaVersionLabel('—')
      }
    })()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [manifestId, orgId])

  // 顶栏就地编辑:开始编辑某字段(name/desc)
  const startEditMeta = useCallback(
    (field: 'name' | 'desc') => {
      setMetaDraft(field === 'name' ? manifestName : manifestDesc)
      setEditingMeta(field)
    },
    [manifestName, manifestDesc],
  )

  // 提交元信息编辑(失焦或回车):仅在变化时调 UpdateManifest
  const commitEditMeta = useCallback(async () => {
    const field = editingMeta
    if (!field) return
    const val = metaDraft.trim()
    const cur = field === 'name' ? manifestName : manifestDesc
    setEditingMeta(null)
    if (val === cur) return
    if (field === 'name' && val === '') {
      message.warning('名称不能为空')
      return
    }
    if (field === 'name' && val.length > 255) {
      message.warning('名称不超过 255 字符')
      return
    }
    if (field === 'desc' && val.length > 1024) {
      message.warning('描述不超过 1024 字符')
      return
    }
    // 乐观更新本地,失败回滚
    const prev = cur
    if (field === 'name') setManifestName(val)
    else setManifestDesc(val)
    try {
      await updateManifest(String(orgId), manifestId, field === 'name' ? { name: val } : { description: val })
      message.success(field === 'name' ? '名称已更新' : '描述已更新')
    } catch (err) {
      if (field === 'name') setManifestName(prev)
      else setManifestDesc(prev)
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      message.error(`更新失败: ${msg ?? '未知错误'}`)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editingMeta, metaDraft, manifestName, manifestDesc, orgId, manifestId])

  // 切到历史视图时刷新版本 + 未提交更改;切到部署视图时刷新部署列表
  useEffect(() => {
    if (activeView === 'history') {
      loadVersions()
      void loadDraftDiff()
    } else if (activeView === 'deploy') {
      loadDeployments()
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
        // 内容已在服务端还原,缓存的 model/content 已过期 → dispose 让重开时重建
        const wasOpen = currentFileRef.current === f.path
        if (wasOpen) editorRef.current?.setModel(null)
        fileContentCache.current.delete(f.path)
        const m = modelCache.current.get(f.path)
        if (m) {
          m.dispose()
          modelCache.current.delete(f.path)
        }
        viewStateCache.current.delete(f.path)
        if (wasOpen) {
          if (f.state === 'added') {
            // added 被撤销 = 删除该文件 → 关 tab
            setOpenTabs((prev) => prev.filter((p) => p !== f.path))
            setCurrentFile(null)
          } else {
            setCurrentFile(null)
            void openFile(f.path) // 重建 model 显示还原后内容
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
      // 标记为新建文件
      setNewFiles((prev) => {
        if (prev.has(path)) return prev
        const next = new Set(prev)
        next.add(path)
        return next
      })
      originalContentRef.current.set(path, '')
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
    setGhostDir(null) // 根目录新建
    setCreating('')
  }, [])

  // 在指定目录下新建文件(目录行 hover 按钮 / 右键菜单):复用 ghostDir 机制
  const startCreateFileIn = useCallback((dir: string) => {
    setRenamingPath(null)
    setCreatingDir(null)
    setInlineError(null)
    setGhostDir(dir)
    setCollapsedDirs((prev) => {
      const next = new Set(prev)
      next.delete(dir)
      return next
    })
    setCreating('')
  }, [])

  // ========== 新建目录(幽灵目录)==========
  // createDirParentRef: 新建子目录时的父前缀(空=根)。在 commitCreateDir 里拼接。
  const createDirParentRef = useRef<string>('')
  const startCreateDir = useCallback(() => {
    setRenamingPath(null)
    setCreating(null)
    setInlineError(null)
    createDirParentRef.current = ''
    setCreatingDir('')
  }, [])

  // 在指定目录下新建子目录
  const startCreateDirIn = useCallback((parent: string) => {
    setRenamingPath(null)
    setCreating(null)
    setInlineError(null)
    createDirParentRef.current = parent
    setCollapsedDirs((prev) => {
      const next = new Set(prev)
      next.delete(parent)
      return next
    })
    setCreatingDir('')
  }, [])

  // 确认目录名 → 设为幽灵目录 + 立即在该目录下起新建文件输入
  const commitCreateDir = useCallback(() => {
    const raw = (creatingDir || '').trim().replace(/\/+$/, '')
    if (!raw) {
      setCreatingDir(null)
      return
    }
    const parent = createDirParentRef.current
    const dir = parent ? `${parent}/${raw}` : raw
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
      // 若当前编辑器正显示被删文件,先把 model 卸下,再 dispose(否则 dispose 已 set 的 model 会报错)
      if (currentFileRef.current && match(currentFileRef.current)) {
        editorRef.current?.setModel(null)
        setCurrentFile(null)
      }
      fileContentCache.current.forEach((_v, k) => {
        if (match(k)) fileContentCache.current.delete(k)
      })
      modelCache.current.forEach((m, k) => {
        if (match(k)) {
          m.dispose()
          modelCache.current.delete(k)
        }
      })
      viewStateCache.current.forEach((_v, k) => {
        if (match(k)) viewStateCache.current.delete(k)
      })
      setOpenTabs((prev) => prev.filter((p) => !match(p)))
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
      // 转移内容缓存;model 因语言可能随扩展名变,直接 dispose 让新 path 懒重建
      const cached = fileContentCache.current.get(fromPath)
      fileContentCache.current.delete(fromPath)
      if (cached !== undefined) fileContentCache.current.set(to, cached)
      const oldModel = modelCache.current.get(fromPath)
      const wasCurrent = currentFile === fromPath
      if (wasCurrent) editorRef.current?.setModel(null)
      if (oldModel) {
        oldModel.dispose()
        modelCache.current.delete(fromPath)
      }
      viewStateCache.current.delete(fromPath)
      setOpenTabs((prev) => prev.map((p) => (p === fromPath ? to : p)))
      const items = await listFiles(ctx)
      setFiles(items)
      setRenamingPath(null)
      setInlineError(null)
      if (wasCurrent) {
        setCurrentFile(null)
        void openFile(to) // 重建 model(新语言)并显示
      }
    } catch (err: any) {
      const msg = typeof err === 'string' ? err : err?.message
      setInlineError(msg ?? '重命名失败')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [renamingPath, renameValue, currentFile, ctx, validatePath])

  // ========== 复制 / 剪切 / 粘贴(单文件)==========
  const copyFile = useCallback((path: string) => {
    clipboardRef.current = { path, mode: 'copy' }
  }, [])
  const cutFile = useCallback((path: string) => {
    clipboardRef.current = { path, mode: 'cut' }
  }, [])

  // 在 dir 下为 base 名找一个不冲突的路径(冲突则在扩展名前加 -copy)
  const uniquePathIn = useCallback(
    (dir: string, base: string): string => {
      const dot = base.lastIndexOf('.')
      const stem = dot > 0 ? base.slice(0, dot) : base
      const ext = dot > 0 ? base.slice(dot) : ''
      let candidate = dir ? `${dir}/${base}` : base
      while (files.some((f) => f.path === candidate)) {
        const next = `${stem}-copy${ext}`
        candidate = dir ? `${dir}/${next}` : next
        base = next // 继续叠加 -copy-copy 直到不冲突
      }
      return candidate
    },
    [files],
  )

  // 把"单文件移动"在前端缓存里同步(model/viewState/openTabs/currentFile)
  const syncCachesAfterMove = useCallback(
    (from: string, to: string) => {
      const cached = fileContentCache.current.get(from)
      fileContentCache.current.delete(from)
      if (cached !== undefined) fileContentCache.current.set(to, cached)
      const oldModel = modelCache.current.get(from)
      const wasCurrent = currentFileRef.current === from
      if (wasCurrent) editorRef.current?.setModel(null)
      if (oldModel) {
        oldModel.dispose()
        modelCache.current.delete(from)
      }
      viewStateCache.current.delete(from)
      setOpenTabs((prev) => prev.map((p) => (p === from ? to : p)))
      return wasCurrent
    },
    [],
  )

  // 粘贴到目录 dir('' = 根)
  const pasteInto = useCallback(
    async (dir: string) => {
      const cb = clipboardRef.current
      if (!cb) return
      const base = cb.path.split('/').pop() || cb.path
      const to = uniquePathIn(dir, base)
      try {
        if (cb.mode === 'copy') {
          const f = await readFile(ctx, cb.path)
          if (f.is_binary) await putFileB64(ctx, to, f.content_b64 ?? '')
          else await putFile(ctx, to, f.content ?? '')
        } else {
          // cut = 移动
          await moveFile(ctx, cb.path, to)
          syncCachesAfterMove(cb.path, to)
          clipboardRef.current = null
        }
        const items = await listFiles(ctx)
        setFiles(items)
        void openFile(to)
        message.success(cb.mode === 'copy' ? '已粘贴副本' : '已移动')
      } catch (err: any) {
        const msg = typeof err === 'string' ? err : err?.message
        message.error(`粘贴失败: ${msg ?? '未知错误'}`)
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [ctx, uniquePathIn, syncCachesAfterMove, openFile],
  )

  // ========== 拖拽移动文件/文件夹到目标目录 ==========
  const moveNodeTo = useCallback(
    async (src: { path: string; isDir: boolean }, destDir: string) => {
      const base = src.path.split('/').pop() || src.path
      const srcParent = src.path.includes('/') ? src.path.slice(0, src.path.lastIndexOf('/')) : ''
      // 非法/无效目标:原位、目录拖进自身或子目录
      if (destDir === srcParent) return
      if (src.isDir && (destDir === src.path || destDir.startsWith(src.path + '/'))) return
      const to = destDir ? `${destDir}/${base}` : base
      if (to === src.path) return
      try {
        if (src.isDir) {
          await moveDir(ctx, src.path, to)
          // 目录下所有缓存项前缀迁移
          const affected: string[] = []
          modelCache.current.forEach((_m, k) => {
            if (k === src.path || k.startsWith(src.path + '/')) affected.push(k)
          })
          fileContentCache.current.forEach((_v, k) => {
            if ((k === src.path || k.startsWith(src.path + '/')) && !affected.includes(k)) affected.push(k)
          })
          for (const oldP of affected) {
            const newP = to + oldP.slice(src.path.length)
            syncCachesAfterMove(oldP, newP)
          }
        } else {
          await moveFile(ctx, src.path, to)
          syncCachesAfterMove(src.path, to)
        }
        const items = await listFiles(ctx)
        setFiles(items)
        // 展开目标目录
        if (destDir) setCollapsedDirs((prev) => {
          const next = new Set(prev)
          next.delete(destDir)
          return next
        })
        message.success('已移动')
      } catch (err: any) {
        const msg = typeof err === 'string' ? err : err?.message
        message.error(`移动失败: ${msg ?? '未知错误'}`)
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [ctx, syncCachesAfterMove],
  )

  // ========== 右键菜单项组装 ==========
  const buildMenuItems = useCallback(
    (target: { kind: 'file' | 'dir' | 'blank'; path: string }): ContextMenuItem[] => {
      const cb = clipboardRef.current
      const items: ContextMenuItem[] = []
      if (target.kind === 'file') {
        const dir = target.path.includes('/') ? target.path.slice(0, target.path.lastIndexOf('/')) : ''
        items.push({ label: '打开', icon: 'go-to-file', onClick: () => void openFile(target.path) })
        items.push({ label: '重命名', icon: 'edit', onClick: () => startRename(target.path) })
        items.push({
          label: '删除',
          icon: 'trash',
          danger: true,
          onClick: () => setPendingDelete({ path: target.path, isDir: false }),
        })
        items.push({ label: '复制', icon: 'copy', separatorBefore: true, onClick: () => copyFile(target.path) })
        items.push({ label: '剪切', icon: 'combine', onClick: () => cutFile(target.path) })
        if (cb) items.push({ label: '粘贴到所在目录', icon: 'clippy', onClick: () => void pasteInto(dir) })
        items.push({
          label: '复制路径',
          icon: 'link',
          separatorBefore: true,
          onClick: () => void navigator.clipboard?.writeText(target.path),
        })
      } else if (target.kind === 'dir') {
        items.push({ label: '新建文件', icon: 'new-file', onClick: () => startCreateFileIn(target.path) })
        items.push({ label: '新建文件夹', icon: 'new-folder', onClick: () => startCreateDirIn(target.path) })
        if (cb)
          items.push({ label: '粘贴', icon: 'clippy', onClick: () => void pasteInto(target.path) })
        items.push({
          label: '删除整个目录',
          icon: 'trash',
          danger: true,
          separatorBefore: true,
          onClick: () => setPendingDelete({ path: target.path, isDir: true }),
        })
      } else {
        items.push({ label: '新建文件', icon: 'new-file', onClick: () => startCreateFile() })
        items.push({ label: '新建文件夹', icon: 'new-folder', onClick: () => startCreateDir() })
        if (cb) items.push({ label: '粘贴', icon: 'clippy', onClick: () => void pasteInto('') })
      }
      return items
    },
    [openFile, startRename, copyFile, cutFile, pasteInto, startCreateFileIn, startCreateDirIn, startCreateFile, startCreateDir],
  )

  const onTreeContextMenu = useCallback(
    (e: React.MouseEvent, target: { kind: 'file' | 'dir' | 'blank'; path: string }) => {
      e.preventDefault()
      e.stopPropagation()
      // 右键即把焦点高亮移到该节点(否则选中框还停在上一个打开的文件上)
      setFocusedPath(target.kind === 'blank' ? null : target.path)
      setContextMenu({ x: e.clientX, y: e.clientY, target })
    },
    [],
  )

  // 编辑器 tab 右键菜单项
  const buildTabMenuItems = useCallback(
    (path: string): ContextMenuItem[] => {
      const i = openTabs.indexOf(path)
      const hasOther = openTabs.length > 1
      const hasRight = i >= 0 && i < openTabs.length - 1
      const hasLeft = i > 0
      const hasSaved = openTabs.some((p) => !dirtyFiles.has(p))
      return [
        { label: '关闭', icon: 'close', onClick: () => closeTab(path) },
        { label: '关闭其他', icon: 'close-all', disabled: !hasOther, onClick: () => closeOtherTabs(path) },
        { label: '关闭右侧', icon: 'arrow-right', disabled: !hasRight, onClick: () => closeTabsToRight(path) },
        { label: '关闭左侧', icon: 'arrow-left', disabled: !hasLeft, onClick: () => closeTabsToLeft(path) },
        {
          label: '关闭全部已保存',
          icon: 'save-all',
          separatorBefore: true,
          disabled: !hasSaved,
          onClick: () => closeSavedTabs(),
        },
        { label: '关闭全部', icon: 'close-all', onClick: () => closeAllTabs() },
      ]
    },
    [openTabs, dirtyFiles, closeTab, closeOtherTabs, closeTabsToRight, closeTabsToLeft, closeSavedTabs, closeAllTabs],
  )

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

  // 当前可见(展开态下)的扁平节点序列,用于键盘上下导航
  const visibleNodes = useMemo(() => {
    const out: { path: string; isDir: boolean }[] = []
    const walk = (nodes: TreeNode[]) => {
      for (const n of nodes) {
        out.push({ path: n.path, isDir: n.type === 'dir' })
        if (n.type === 'dir' && !collapsedDirs.has(n.path) && n.children) walk(n.children)
      }
    }
    walk(fileTree)
    return out
  }, [fileTree, collapsedDirs])

  // 文件树键盘导航(容器 onKeyDown):↑↓ 移动焦点,←→ 折叠/展开,Enter 打开,F2/Delete,Cmd+CXV
  const onTreeKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      // 内联输入(新建/重命名)进行中时,不接管按键
      if (creating !== null || creatingDir !== null || renamingPath) return
      const cur = focusedPath
      const idx = visibleNodes.findIndex((n) => n.path === cur)
      const node = idx >= 0 ? visibleNodes[idx] : null
      const mod = e.metaKey || e.ctrlKey

      if (mod && (e.key === 'c' || e.key === 'C')) {
        if (node && !node.isDir) { e.preventDefault(); copyFile(node.path) }
        return
      }
      if (mod && (e.key === 'x' || e.key === 'X')) {
        if (node && !node.isDir) { e.preventDefault(); cutFile(node.path) }
        return
      }
      if (mod && (e.key === 'v' || e.key === 'V')) {
        if (clipboardRef.current && node) {
          e.preventDefault()
          const destDir = node.isDir ? node.path : node.path.includes('/') ? node.path.slice(0, node.path.lastIndexOf('/')) : ''
          void pasteInto(destDir)
        }
        // 内部剪贴板为空时不 preventDefault,让 onPaste 事件处理系统文件粘贴
        return
      }
      switch (e.key) {
        case 'ArrowDown': {
          e.preventDefault()
          const next = visibleNodes[Math.min(visibleNodes.length - 1, idx + 1)] ?? visibleNodes[0]
          if (next) setFocusedPath(next.path)
          break
        }
        case 'ArrowUp': {
          e.preventDefault()
          const prev = visibleNodes[Math.max(0, idx - 1)] ?? visibleNodes[0]
          if (prev) setFocusedPath(prev.path)
          break
        }
        case 'ArrowRight':
          if (node?.isDir) {
            e.preventDefault()
            if (collapsedDirs.has(node.path)) toggleDir(node.path)
          }
          break
        case 'ArrowLeft':
          if (node?.isDir && !collapsedDirs.has(node.path)) {
            e.preventDefault()
            toggleDir(node.path)
          } else if (node) {
            // 文件 / 已折叠目录:焦点跳到父目录
            const parent = node.path.includes('/') ? node.path.slice(0, node.path.lastIndexOf('/')) : ''
            if (parent) { e.preventDefault(); setFocusedPath(parent) }
          }
          break
        case 'Enter':
          if (node) {
            e.preventDefault()
            if (node.isDir) toggleDir(node.path)
            else void openFile(node.path)
          }
          break
        case 'F2':
          if (node && !node.isDir) { e.preventDefault(); startRename(node.path) }
          break
        case 'Delete':
        case 'Backspace':
          if (node) { e.preventDefault(); setPendingDelete({ path: node.path, isDir: node.isDir }) }
          break
      }
    },
    [creating, creatingDir, renamingPath, focusedPath, visibleNodes, collapsedDirs, toggleDir, openFile, startRename, copyFile, cutFile, pasteInto],
  )

  // ========== 上传遍历后的文件列表(拖拽/粘贴共用)==========
  const uploadTraversedFiles = useCallback(
    async (traversed: TraversedFile[], destDir: string) => {
      const total = traversed.length
      let ok = 0
      let skipped = 0
      for (let i = 0; i < total; i++) {
        const { relativePath, file } = traversed[i]
        const fullPath = destDir ? `${destDir}/${relativePath}` : relativePath
        setPasteProgress({ current: i + 1, total, fileName: fullPath })
        if (file.size > MANIFEST_MAX_FILE_SIZE) {
          message.error(`${relativePath} 超过 ${MANIFEST_MAX_FILE_SIZE / 1024 / 1024}MB,跳过`)
          skipped++
          continue
        }
        const errMsg = validatePath(fullPath)
        if (errMsg) {
          message.error(`${fullPath}: ${errMsg}`)
          skipped++
          continue
        }
        try {
          const b64 = await fileToBase64(file)
          await putFileB64(ctx, fullPath, b64)
          ok++
        } catch (err: any) {
          message.error(`${fullPath} 上传失败: ${err?.message ?? err}`)
          skipped++
        }
      }
      setPasteProgress(null)
      if (ok > 0) {
        message.success(`已上传 ${ok} 个文件${skipped > 0 ? `, ${skipped} 个跳过` : ''}`)
        setManifestMissing(false)
        const items = await listFiles(ctx)
        setFiles(items)
        // 展开目标目录
        if (destDir) {
          setCollapsedDirs((prev) => {
            const next = new Set(prev)
            next.delete(destDir)
            return next
          })
        }
      } else if (skipped > 0) {
        message.warning(`全部 ${skipped} 个文件上传失败`)
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [ctx, validatePath],
  )

  // ========== 拖拽上传本地文件(支持文件夹,保留目录结构)==========
  const handleDropFiles = useCallback(
    async (collected: (FileSystemEntry | File)[]) => {
      setDragOver(false)
      const traversed = await traverseCollectedEntries(collected)
      if (traversed.length === 0) return
      await uploadTraversedFiles(traversed, '')
    },
    [uploadTraversedFiles],
  )

  // ========== 从系统剪贴板粘贴文件(Cmd+V 或右键粘贴,支持文件夹)==========
  const handlePasteFiles = useCallback(
    async (collected: (FileSystemEntry | File)[]) => {
      const traversed = await traverseCollectedEntries(collected)
      if (traversed.length === 0) return
      // 粘贴到当前聚焦的目录,无焦点则到根
      const destDir = (() => {
        if (!focusedPath) return ''
        const node = visibleNodes.find((n) => n.path === focusedPath)
        if (!node) return ''
        if (node.isDir) return node.path
        return node.path.includes('/') ? node.path.slice(0, node.path.lastIndexOf('/')) : ''
      })()
      await uploadTraversedFiles(traversed, destDir)
    },
    [uploadTraversedFiles, focusedPath, visibleNodes],
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
              className={`${styles.treeNode} ${deleting ? styles.deleting : ''} ${focusedPath === node.path ? styles.focused : ''} ${dragOverDir === node.path ? styles.dragOverNode : ''}`}
              style={{ paddingLeft: indent }}
              draggable
              onDragStart={(e) => {
                draggingRef.current = { path: node.path, isDir: true }
                e.dataTransfer.setData('application/x-manifest-node', node.path)
                e.dataTransfer.effectAllowed = 'move'
              }}
              onDragOver={(e) => {
                if (draggingRef.current) {
                  e.preventDefault()
                  e.stopPropagation()
                  setDragOverDir(node.path)
                }
              }}
              onDragLeave={() => setDragOverDir((d) => (d === node.path ? null : d))}
              onDrop={(e) => {
                const src = draggingRef.current
                if (src) {
                  e.preventDefault()
                  e.stopPropagation()
                  setDragOverDir(null)
                  draggingRef.current = null
                  void moveNodeTo(src, node.path)
                }
              }}
              onClick={() => {
                setFocusedPath(node.path)
                treeRef.current?.focus()
                toggleDir(node.path)
              }}
              onContextMenu={(e) => onTreeContextMenu(e, { kind: 'dir', path: node.path })}
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
                    className="codicon codicon-new-file"
                    title="在此目录下新建文件"
                    onClick={(e) => {
                      e.stopPropagation()
                      startCreateFileIn(node.path)
                    }}
                  />
                  <i
                    className="codicon codicon-new-folder"
                    title="在此目录下新建文件夹"
                    onClick={(e) => {
                      e.stopPropagation()
                      startCreateDirIn(node.path)
                    }}
                  />
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
          className={`${styles.treeNode} ${currentFile === node.path ? styles.selected : ''} ${focusedPath === node.path ? styles.focused : ''} ${deleting ? styles.deleting : ''}`}
          style={{ paddingLeft: indent }}
          draggable={renamingPath !== node.path}
          onDragStart={(e) => {
            draggingRef.current = { path: node.path, isDir: false }
            e.dataTransfer.setData('application/x-manifest-node', node.path)
            e.dataTransfer.effectAllowed = 'move'
          }}
          onClick={() => {
            if (renamingPath === node.path) return
            setFocusedPath(node.path)
            void openFile(node.path)
          }}
          onContextMenu={(e) => onTreeContextMenu(e, { kind: 'file', path: node.path })}
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
            <>
              <span className={styles.name}>{node.name}</span>
              {newFiles.has(node.path) ? (
                <span className={`${styles.statusBadge} ${styles.new}`} title="新建文件">U</span>
              ) : modifiedFiles.has(node.path) ? (
                <span className={`${styles.statusBadge} ${styles.modified}`} title="已修改">M</span>
              ) : null}
            </>
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
    <div
      ref={rootRef}
      className={styles.root}
      style={{ gridTemplateColumns: `48px ${sidebarWidth}px 1fr` }}
    >
      <div className={styles.titleBar}>
        <div className={styles.traffic}>
          <span
            className={styles.red}
            title="关闭编辑器(返回 Manifest 列表)"
            role="button"
            onClick={() => void handleClose()}
          />
          <span className={styles.yellow} />
          <span
            className={styles.green}
            title="全屏 / 退出全屏"
            role="button"
            onClick={toggleFullscreen}
          />
        </div>
        {/* 导航后退/前进(中间偏左),复用 Cmd+←/→ 逻辑 */}
        <div className={styles.navArrows}>
          <i
            className="codicon codicon-arrow-left"
            title="后退(Alt+←)"
            role="button"
            onClick={() => navBackRef.current()}
          />
          <i
            className="codicon codicon-arrow-right"
            title="前进(Alt+→)"
            role="button"
            onClick={() => navForwardRef.current()}
          />
        </div>
        <div className={styles.breadcrumb}>
          <span className={styles.muted}>Terranova</span>
          <span className={styles.muted}> › </span>
          <span className={styles.muted}>manifests</span>
          <span className={styles.muted}> › </span>
          {/* 名称就地编辑:点击变输入框(无名称时回退显示 manifestId) */}
          {editingMeta === 'name' ? (
            <input
              className={styles.metaInput}
              value={metaDraft}
              autoFocus
              maxLength={255}
              onChange={(e) => setMetaDraft(e.target.value)}
              onBlur={() => void commitEditMeta()}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void commitEditMeta()
                else if (e.key === 'Escape') setEditingMeta(null)
              }}
            />
          ) : (
            <span className={styles.metaName} title="点击编辑名称" onClick={() => startEditMeta('name')}>
              {manifestName || manifestId}
            </span>
          )}
          {/* 徽标反映真实状态:有已发布版本 → 显示最新版本号;否则 DRAFT(从未发布) */}
          <span className={styles.badge} title={versions.length > 0 ? '最新已发布版本' : '尚未发布任何版本'}>
            {versions.length > 0 ? versions[0].version : 'DRAFT'}
          </span>
          {/* 描述就地编辑:点击变输入框;空描述显示"添加描述"占位 */}
          {editingMeta === 'desc' ? (
            <input
              className={`${styles.metaInput} ${styles.metaInputDesc}`}
              value={metaDraft}
              autoFocus
              maxLength={1024}
              placeholder="描述(≤1024)"
              onChange={(e) => setMetaDraft(e.target.value)}
              onBlur={() => void commitEditMeta()}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void commitEditMeta()
                else if (e.key === 'Escape') setEditingMeta(null)
              }}
            />
          ) : (
            <span
              className={styles.metaDesc}
              title={manifestDesc || '点击添加描述'}
              onClick={() => startEditMeta('desc')}
            >
              {manifestDesc || '添加描述'}
            </span>
          )}
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
          <ManifestAiTools
            bridge={aiBridge}
            disabled={manifestMissing}
            open={activeRightPanel === 'ai'}
            onOpen={() => setActiveRightPanel('ai')}
            onClose={() => setActiveRightPanel((prev) => (prev === 'ai' ? null : prev))}
            panelWidth={rightPanelWidth}
            onRequestCheck={() => {
              // 打开检查面板时快照当前选区/文件上下文
              const sel = aiBridge.getSelectionInfo()
              if (sel) {
                setCheckContext({
                  kind: 'selection',
                  filePath: sel.filePath,
                  startLine: sel.startLine,
                  endLine: sel.endLine,
                })
              } else {
                const filePath = aiBridge.getActiveFilePath()
                setCheckContext(filePath ? { kind: 'file', filePath } : null)
              }
              // 清空上次检查的残留状态，避免跨 session 显示旧进度
              setCheckIssues([])
              setCheckCompletedSteps([])
              setCheckError(null)
              setCheckCurrentStep('')
              setActiveRightPanel('check')
            }}
          />
          <button
            title="对当前草稿在已部署 workspace 跑 plan-only 检测"
            disabled={manifestMissing}
            onClick={() => {
              setRunViewLast(false)
              setActiveRightPanel('run')
            }}
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
            onClick={() => setActiveRightPanel('deploy')}
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
        <div
          className={`${styles.item} ${activeView === 'search' ? styles.active : ''}`}
          title="跨文件搜索 (Cmd/Ctrl+Shift+F)"
          role="button"
          onClick={() => {
            setSearchShowReplace(false)
            setActiveView('search')
          }}
        >
          <i className="codicon codicon-search" />
        </div>
        <div
          className={`${styles.item} ${activeView === 'problems' ? styles.active : ''}`}
          title="问题"
          role="button"
          onClick={() => {
            refreshProblemsRef.current()
            setActiveView('problems')
          }}
        >
          <i className="codicon codicon-warning" />
        </div>
        <div
          className={`${styles.item} ${activeView === 'deploy' ? styles.active : ''}`}
          title="已部署的 Workspace"
          role="button"
          onClick={() => setActiveView('deploy')}
        >
          <i className="codicon codicon-rocket" />
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
        {activeView === 'search' && (
          <SearchPanel
            ctx={ctx}
            showReplace={searchShowReplace}
            onOpenAt={(p, line, col, endCol) => void openAt(p, line, col, endCol)}
            onAfterReplace={(changed) => refreshAfterReplace(changed)}
          />
        )}
        {activeView === 'problems' && (
          <ProblemsPanel
            problems={problems}
            onOpenAt={(p, line, col, endCol) => void openAt(p, line, col, endCol)}
          />
        )}
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
          ref={treeRef}
          className={`${styles.tree} ${dragOver ? styles.dragOver : ''}`}
          tabIndex={0}
          onKeyDown={onTreeKeyDown}
          onContextMenu={(e) => {
            // 空白区右键(事件未被节点 stopPropagation 拦下)
            onTreeContextMenu(e, { kind: 'blank', path: '' })
          }}
          onDragOver={(e) => {
            e.preventDefault()
            // 外部文件拖入才高亮整树(内部节点移动由目录行自己高亮)
            if (!draggingRef.current && !dragOver) setDragOver(true)
          }}
          onDragLeave={(e) => {
            // 仅当离开整个 tree 容器(而非内部子元素)时才取消高亮
            if (e.currentTarget === e.target) setDragOver(false)
          }}
          onDrop={(e) => {
            e.preventDefault()
            // 内部节点拖到树空白处 = 移到根目录
            const src = draggingRef.current
            if (src) {
              draggingRef.current = null
              setDragOverDir(null)
              void moveNodeTo(src, '')
              return
            }
            if (e.dataTransfer.items && e.dataTransfer.items.length > 0) {
              // 同步提取 FileSystemEntry(事件返回后 items 失效)
              const collected = collectFileEntries(e.dataTransfer.items)
              void handleDropFiles(collected)
            } else {
              setDragOver(false)
            }
          }}
          onPaste={(e) => {
            // 系统剪贴板粘贴文件(从 Finder/Explorer 复制的文件,或右键粘贴)
            const items = e.clipboardData?.items
            if (!items) return
            const hasFileItem = Array.from(items).some((it) => it.kind === 'file')
            if (!hasFileItem) return // 非文件粘贴,放行给内部剪贴板逻辑
            e.preventDefault()
            const collected = collectFileEntries(items)
            void handlePasteFiles(collected)
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

        {activeView === 'deploy' && (
          <>
            <div className={styles.header}>
              <span>已部署的 Workspace</span>
              <span className={styles.actions}>
                <i className="codicon codicon-refresh" title="刷新" onClick={() => loadDeployments()} />
              </span>
            </div>
            <div className={styles.tree}>
              {deployLoading && (
                <div style={{ padding: '8px 12px', color: '#858585', fontSize: 12 }}>加载中...</div>
              )}
              {!deployLoading && deployments.length === 0 && (
                <div style={{ padding: '8px 12px', color: '#858585', fontSize: 12, lineHeight: 1.5 }}>
                  本 manifest 还没部署到任何 workspace。
                  <br />
                  点顶栏「部署到 Workspace」进行安装。
                </div>
              )}
              {!deployLoading &&
                deployments.map((d) => {
                  const wsName = wsNameById[d.workspace_id] || d.workspace_id
                  const ver = versions.find((v) => v.id === d.version_id)?.version
                  return (
                    <div
                      key={d.id}
                      className={styles.deployRow}
                      title={`跳转到 ${wsName}`}
                      onClick={() => navigate(`/workspaces/${d.workspace_id}`)}
                    >
                      <i className={`codicon codicon-vm ${styles.deployIcon}`} />
                      <span className={styles.deployName}>{wsName}</span>
                      {ver && <span className={styles.deployVersion}>{ver}</span>}
                      <i className={`codicon codicon-arrow-right ${styles.deployGo}`} />
                    </div>
                  )
                })}
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
        {/* 侧栏右缘拖拽条:调整 sidebar 宽度 */}
        <div className={styles.sidebarResizer} onMouseDown={startSidebarResize} />
      </div>

      <div className={styles.editorArea} style={{ marginRight: activeRightPanel ? rightPanelWidth : 0 }}>
        <div className={styles.tabs}>
          {openTabs.map((path) => (
            <div
              key={path}
              className={`${styles.tab} ${currentFile === path && !activeDiffKey ? styles.active : ''} ${dirtyFiles.has(path) ? styles.dirty : ''}`}
              onClick={() => void openFile(path)}
              onContextMenu={(e) => {
                e.preventDefault()
                setTabMenu({ x: e.clientX, y: e.clientY, path })
              }}
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
        {currentFile && !activeDiffKey && !binaryView ? (
          <div className={styles.editorBreadcrumb} title={currentFile}>
            {currentFile.split('/').map((seg, i, arr) => (
              <span key={`${i}-${seg}`}>
                {i > 0 ? <span className={styles.editorBreadcrumbSep}>/</span> : null}
                <span className={`${styles.editorBreadcrumbSeg}${i === arr.length - 1 ? ' ' + styles.editorBreadcrumbCurrent : ''}`}>
                  {seg}
                </span>
              </span>
            ))}
          </div>
        ) : null}
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
                activeDiffKey || (binaryView && currentFile === binaryView.path)
                  ? 'hidden'
                  : 'visible',
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
        <span
          className={`${styles.item} ${styles.statusBarClickable}`}
          title="打开问题面板"
          onClick={() => {
            refreshProblemsRef.current()
            setActiveView('problems')
          }}
        >
          <i className="codicon codicon-error" /> {problems.filter((p) => p.severity === monaco.MarkerSeverity.Error).length}
          <i className="codicon codicon-warning" style={{ marginLeft: 6 }} /> {problems.filter((p) => p.severity === monaco.MarkerSeverity.Warning).length}
        </span>
        <div className={styles.spacer} />
        {pasteProgress && (
          <span className={styles.item} style={{ background: '#005a9e', padding: '0 8px' }}>
            <i className="codicon codicon-cloud-upload" style={{ marginRight: 4 }} />
            上传 {pasteProgress.current}/{pasteProgress.total} — {pasteProgress.fileName}
          </span>
        )}
        {lastRunTask && (
          <span
            className={styles.item}
            style={{ cursor: 'pointer' }}
            title={`查看上次运行: Task #${lastRunTask.taskId}`}
            onClick={() => {
              setRunViewLast(true)
              setActiveRightPanel('run')
            }}
          >
            <i className="codicon codicon-output" />
            Task #{lastRunTask.taskId}
          </span>
        )}
        <span className={styles.item}>{`行 ${cursor.line}, 列 ${cursor.col}`}</span>
        <span className={styles.item}>UTF-8</span>
        <span className={styles.item}>LF</span>
        <span className={styles.item}>{currentFile ? languageDisplay(currentFile) : ''}</span>
        <span
          className={styles.item}
          title="Provider 类型补全目录(execute post_init 落库;按 manifest+subpath;provider 版本未变不更新)"
        >
          schema {schemaVersionLabel}
        </span>
      </div>

      {/* 右侧停靠面板:检查 / Run / 部署(与 AI 生成面板共享布局,四选一互斥) */}
      {activeRightPanel && (
        <div
          className={styles.panelResizer}
          style={{ top: 65, bottom: 22, right: rightPanelWidth - 2 }}
          onMouseDown={startPanelResize}
        />
      )}

      {activeRightPanel === 'check' && (
        <CheckPanel
          busy={checkBusy}
          issues={checkIssues}
          completedSteps={checkCompletedSteps}
          checkError={checkError}
          currentStepName={checkCurrentStep}
          manifestId={manifestId}
          orgId={orgId}
          checkContext={checkContext}
          onRevealAt={aiBridge.revealAt}
          onApplyFix={handleCheckApplyFix}
          onStartCheck={(instruction, history, sessionId) => void runAiCheck(instruction || undefined, history, sessionId)}
          onRecheck={() => void runAiCheck()}
          onSessionChange={() => {
            setCheckIssues([])
            setCheckCompletedSteps([])
            setCheckError(null)
            setCheckCurrentStep('')
          }}
          onClose={handleCheckPanelClose}
          panelWidth={rightPanelWidth}
        />
      )}

      {activeRightPanel === 'run' && (
        <RunDialog
          open
          ctx={ctx}
          lastRunTask={lastRunTask}
          viewLast={runViewLast}
          onRunTaskCreated={(taskId, workspaceId) => setLastRunTask({ taskId, workspaceId })}
          onClose={() => {
            setActiveRightPanel((prev) => (prev === 'run' ? null : prev))
            setRunViewLast(false)
          }}
          panelWidth={rightPanelWidth}
        />
      )}

      {activeRightPanel === 'deploy' && (
        <DeployPanel
          ctx={ctx}
          onClose={() => setActiveRightPanel((prev) => (prev === 'deploy' ? null : prev))}
          panelWidth={rightPanelWidth}
        />
      )}

      {/* === 弹窗 === */}
      <PublishVersionDialog
        open={publishOpen}
        ctx={ctx}
        checkSummary={publishCheckSummary}
        onStartCheck={() => {
          setPublishCheckSummary(null)
          void runPublishCheck()
        }}
        onSkipCheck={() => setPublishCheckSummary({ done: false, skipped: true, issues: [] })}
        onClose={() => setPublishOpen(false)}
        onPublished={() => {
          // 发布成功后始终刷新版本(顶栏徽标依赖 versions);历史视图下再刷未提交更改
          loadVersions()
          if (activeView === 'history') {
            loadDraftDiff()
          }
        }}
      />

      {/* 文件树右键菜单(自建轻量菜单)*/}
      {contextMenu && (
        <TreeContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          items={buildMenuItems(contextMenu.target)}
          onClose={() => setContextMenu(null)}
        />
      )}
      {/* 编辑器 tab 右键菜单 */}
      {tabMenu && (
        <TreeContextMenu
          x={tabMenu.x}
          y={tabMenu.y}
          items={buildTabMenuItems(tabMenu.path)}
          onClose={() => setTabMenu(null)}
        />
      )}

      <QuickOpen
        open={quickOpen}
        files={files.filter((f) => f.type === 'file').map((f) => f.path)}
        onSelect={(path) => void openFile(path)}
        onClose={() => setQuickOpen(false)}
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
