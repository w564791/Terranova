/**
 * Manifest 编辑器 AI 工具 — 资源生成/修复 + 草稿检查
 *
 * 自包含组件:渲染两个工具栏按钮 + 生成弹窗。
 * 与编辑器通过 EditorBridge 解耦,父组件用现有 editorRef / openAt 拼出 bridge。
 *
 * 检查结果由父组件通过右侧面板(CheckPanel)展示,本组件仅负责触发。
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  generateManifestResource,
  listAISessions,
  createAISession,
  getAISessionMessages,
  deleteAISession,
  type ManifestProgressEvent,
  type ManifestIssue,
  type ManifestAISession,
  type ManifestAIMessage,
  type ManifestCompletedStep,
  type ConversationTurn,
} from '../../../services/manifestAi'
import {
  chatPanelStyle,
  chatHeaderStyle,
  chatHeaderUnderline,
  chatHeaderIcon,
  chatBodyStyle,
  chatEmptyStyle,
  sessionListStyle,
  sessionItemStyle,
  historyMsgStyle,
  historyTextStyle,
  historyCodeStyle,
  historyIssueStyle,
  chatInputWrapStyle,
  contextChipRowStyle,
  contextChipStyle,
  chatInputStyle,
  chatInputFooterStyle,
  errorStyle,
  genWarnStyle,
  pipelineStyle,
  pipelineStepStyle,
  pipelineSkillStyle,
  pipelineSkillTagStyle,
} from './manifestAiStyles'

const GEN_STEP_DESC: Record<string, string> = {
  初始化: '获取 AI 配置',
  意图断言: '安全守卫',
  Module候选: '列出 Module 库',
  Skill选择: 'AI 语义选择 Domain + 精确匹配 Module Skill',
  AI生成: '组装 Skill 并调用 AI',
}

/** 选区快照:用于在面板上展示"已附带的上下文" */
export interface SelectionInfo {
  text: string
  filePath: string
  startLine: number
  endLine: number
}

/** 附带给 AI 的上下文:选区(selection)优先,否则整个文件(file) */
export interface AiContext {
  kind: 'selection' | 'file'
  text: string
  filePath: string
  startLine?: number
  endLine?: number
}

/** 待检查文件(跨文件检查时多于一个) */
export interface CheckFile {
  path: string
  content: string
  startLine: number // content 在文件中的起始行(整文件=1)
}

/** AI 给出的结构化修复:按行范围替换目标文件内容 */
export interface ManifestFix {
  file: string
  startLine: number
  endLine: number
  newText: string
}

/** 编辑器桥接:父组件用现有 editorRef/openAt 实现 */
export interface EditorBridge {
  contextIds: { workspace_id?: string; organization_id?: string }
  /** 当前 manifest / org(用于 AI 会话按 manifest+用户隔离)*/
  manifestId: string
  orgId: string
  /** 当前激活文件路径(无则 null) */
  getActiveFilePath: () => string | null
  /** 当前选区信息(无选区返回 null) */
  getSelectionInfo: () => SelectionInfo | null
  /** 当前文件全文 */
  getActiveFileContent: () => string
  /** 把文本插入光标处/替换选区 */
  insertText: (text: string) => void
  /** 打开文件并定位到行 */
  revealAt: (path: string, line: number) => void
  /** 订阅选区变化(用于按钮文案实时切换),返回取消订阅函数 */
  onSelectionChange: (cb: (hasSelection: boolean) => void) => () => void
  /** 收集 check 待检查文件:当前文件 + 跨文件引用到的关联文件(无选区时) */
  collectCheckFiles: () => Promise<CheckFile[]>
  /** 应用一条修复(按行范围替换目标文件,可能是非当前文件) */
  applyFix: (fix: ManifestFix) => Promise<void>
}

interface Props {
  bridge: EditorBridge
  disabled?: boolean
  /** 生成面板是否展开。由父组件统一控制,用于和检查/Run/部署互斥。 */
  open?: boolean
  onOpen?: () => void
  onClose?: () => void
  /** 当前面板宽度(拖拽时由父组件传入,用于动态覆盖 chatPanelStyle 的固定宽度) */
  panelWidth?: number
  /** 检查按钮点击:由父组件执行检查并在右侧面板展示结果 */
  onRequestCheck?: () => void
}

const LEVEL_COLOR: Record<string, string> = {
  error: 'var(--red)',
  warning: '#cca700',
  info: '#3794ff',
}
const LEVEL_ICON: Record<string, string> = {
  error: 'codicon-error',
  warning: 'codicon-warning',
  info: 'codicon-info',
}

export default function ManifestAiTools({ bridge, disabled, open = false, onOpen, onClose, panelWidth, onRequestCheck }: Props) {
  const [description, setDescription] = useState('')
  const [genBusy, setGenBusy] = useState(false)
  const [genStep, setGenStep] = useState<ManifestProgressEvent | null>(null)
  const [genError, setGenError] = useState<string | null>(null)
  const [genWarnings, setGenWarnings] = useState<string[]>([]) // schema 校验警告
  const [genCompletedSteps, setGenCompletedSteps] = useState<ManifestCompletedStep[]>([])
  const [genCurrentStep, setGenCurrentStep] = useState('')
  const [stepsExpanded, setStepsExpanded] = useState(true)
  // 打开面板时快照的上下文(选区优先,否则当前文件),作为 chip 展示,用户可移除
  const [aiContext, setAiContext] = useState<AiContext | null>(null)

  // 当前编辑器是否有选区(订阅 selection 变化),决定 Check 按钮文案
  const [hasSelection, setHasSelection] = useState(false)
  useEffect(() => bridge.onSelectionChange(setHasSelection), [bridge])

  // ===== 会话(按 manifest+用户隔离,后端持久化)=====
  const [sessions, setSessions] = useState<ManifestAISession[]>([])
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null)
  const [history, setHistory] = useState<ManifestAIMessage[]>([])
  const [sessionListOpen, setSessionListOpen] = useState(false)
  const currentSessionRef = useRef<string | null>(null)
  useEffect(() => {
    currentSessionRef.current = currentSessionId
  }, [currentSessionId])

  // 拉某会话的历史消息
  const loadHistory = useCallback(async (sid: string) => {
    try {
      setHistory(await getAISessionMessages(sid))
    } catch {
      setHistory([])
    }
  }, [])

  // 加载会话列表;首次打开时默认续最近一条。只自动选一次:之后用户主动"新建会话"
  // (置空 currentSessionId)的意图不被重开面板覆盖。
  const didAutoSelectRef = useRef(false)
  const refreshSessions = useCallback(
    async (selectLatest: boolean) => {
      try {
        const list = await listAISessions(bridge.manifestId, bridge.orgId)
        setSessions(list)
        if (selectLatest && !didAutoSelectRef.current && list.length > 0 && !currentSessionRef.current) {
          didAutoSelectRef.current = true
          setCurrentSessionId(list[0].id)
          void loadHistory(list[0].id)
        }
      } catch {
        setSessions([])
      }
    },
    [bridge, loadHistory],
  )

  // 确保有当前会话:无则新建,返回 session_id(供发送时携带)。
  // 用 creatingRef 防重入:ref 要到下一帧才被 effect 更新,快速双触发会各看到 null 而双建;
  // 这里把"创建中的 Promise"挂在 ref 上,并发调用复用同一个,避免建出多个会话。
  const creatingSessionRef = useRef<Promise<string | undefined> | null>(null)
  const ensureSession = useCallback(async (): Promise<string | undefined> => {
    if (currentSessionRef.current) return currentSessionRef.current
    if (creatingSessionRef.current) return creatingSessionRef.current
    const p = (async () => {
      try {
        const sess = await createAISession(bridge.manifestId, bridge.orgId, '新会话')
        currentSessionRef.current = sess.id // 立即同步占位,后续调用直接命中
        setCurrentSessionId(sess.id)
        setSessions((prev) => [sess, ...prev])
        return sess.id
      } catch {
        return undefined // 建会话失败不阻断生成/检查(只是不持久化)
      } finally {
        creatingSessionRef.current = null
      }
    })()
    creatingSessionRef.current = p
    return p
  }, [bridge])

  const switchSession = useCallback(
    (sid: string) => {
      setCurrentSessionId(sid)
      setSessionListOpen(false)
      void loadHistory(sid)
    },
    [loadHistory],
  )

  const startNewSession = useCallback(() => {
    setCurrentSessionId(null)
    setHistory([])
    setSessionListOpen(false)
  }, [])

  const removeSession = useCallback(
    async (sid: string) => {
      try {
        await deleteAISession(sid)
      } catch {
        /* 忽略,下方仍刷新 */
      }
      setSessions((prev) => prev.filter((s) => s.id !== sid))
      if (currentSessionRef.current === sid) {
        setCurrentSessionId(null)
        setHistory([])
      }
    },
    [],
  )

  // 打开生成面板:重置上次残留状态 + 快照上下文 + 加载会话列表。选区优先,无选区回退到当前文件。
  const openGenerate = useCallback(() => {
    setGenStep(null)
    setGenError(null)
    setGenWarnings([])
    setGenCompletedSteps([])
    setGenCurrentStep('')
    const sel = bridge.getSelectionInfo()
    if (sel) {
      setAiContext({
        kind: 'selection',
        text: sel.text,
        filePath: sel.filePath,
        startLine: sel.startLine,
        endLine: sel.endLine,
      })
    } else {
      const filePath = bridge.getActiveFilePath()
      const text = bridge.getActiveFileContent()
      setAiContext(filePath && text ? { kind: 'file', text, filePath } : null)
    }
    onOpen?.()
    void refreshSessions(true) // 打开面板时加载会话列表,默认续最近一条
  }, [bridge, refreshSessions, onOpen])

  // 生成的 abort 控制器
  const genAbortRef = useRef<AbortController | null>(null)

  // ===== 生成/修复 =====
  // 从本地历史消息构建对话上下文(发送给 AI 作为上下文参考)
  const buildConversationHistory = useCallback((): ConversationTurn[] => {
    if (history.length === 0) return []
    const turns: ConversationTurn[] = []
    for (const msg of history) {
      let parsed: Record<string, unknown> = {}
      try { parsed = JSON.parse(msg.content) } catch { /* ignore */ }
      if (msg.role === 'user') {
        const desc = String(parsed.description ?? '')
        const truncated = desc.length > 500 ? desc.slice(0, 500) + '...(已截断)' : desc
        if (truncated) turns.push({ role: 'user', content: truncated })
      } else if (msg.role === 'assistant') {
        // 提取 HCL 摘要或检查结论,避免传太多 token
        const hcl = String(parsed.hcl ?? '')
        const message = String(parsed.message ?? '')
        const content = hcl ? `生成了 HCL 代码:\n\`\`\`hcl\n${hcl.slice(0, 500)}${hcl.length > 500 ? '\n...(已截断)' : ''}\n\`\`\`` : (message || '完成')
        turns.push({ role: 'assistant', content })
      }
    }
    // 限制历史轮数,避免 prompt 过大(最多保留最近 6 轮)
    const maxTurns = 12 // 6 轮 = 12 条
    return turns.length > maxTurns ? turns.slice(turns.length - maxTurns) : turns
  }, [history])

  const runGenerate = useCallback(async () => {
    if (!description.trim() || genBusy) return
    setGenBusy(true)
    setGenError(null)
    setGenStep(null)
    setGenWarnings([])
    setGenCompletedSteps([])
    setGenCurrentStep('')
    setDescription('')
    const controller = new AbortController()
    genAbortRef.current = controller

    const sid = await ensureSession() // 无当前会话则新建,把交互落入会话
    const convHistory = buildConversationHistory()
    try {
      const result = await generateManifestResource(
        {
          description: description.trim(),
          currentContent: aiContext?.text || undefined,
          context: aiContext
            ? {
                kind: aiContext.kind,
                file_path: aiContext.filePath,
                start_line: aiContext.startLine,
                end_line: aiContext.endLine,
              }
            : undefined,
          sessionId: sid,
          history: convHistory.length > 0 ? convHistory : undefined,
          contextIds: bridge.contextIds,
        },
        (ev) => {
          setGenStep(ev)
          setGenCurrentStep(ev.step_name || '')
          if (ev.completed_steps?.length) setGenCompletedSteps(ev.completed_steps)
        },
        controller.signal,
      )

      if (result.completedSteps?.length) setGenCompletedSteps(result.completedSteps)
      setGenCurrentStep('')

      if (result.status === 'blocked') {
        setGenError(result.message || '请求被安全策略拦截')
        return
      }
      if (result.hcl) {
        bridge.insertText(result.hcl)
        setGenWarnings(result.warnings ?? []) // schema 校验警告(若有)
        if (sid) {
          void loadHistory(sid) // 刷新会话历史
          void refreshSessions(false)
        }
        // 面板保持打开(VS Code 风格),用关闭按钮收起
      } else {
        setGenError('AI 未返回有效内容')
      }
    } catch (e) {
      // 用户主动取消(abort)不当作错误展示
      if (!isAbortError(e)) {
        setGenError(e instanceof Error ? e.message : '生成失败')
      }
    } finally {
      setGenBusy(false)
      genAbortRef.current = null
    }
  }, [description, genBusy, bridge, aiContext, ensureSession, loadHistory, refreshSessions, buildConversationHistory])

  return (
    <>
      <button
        title="用 AI 生成/修复资源(选中代码则基于选区修复)"
        disabled={disabled || genBusy}
        onClick={openGenerate}
      >
        <i className="codicon codicon-sparkle" /> AI 生成
      </button>
      <button
        title={hasSelection ? '用 AI 检查选中的内容' : '用 AI 检查当前文件(含跨文件引用)'}
        disabled={disabled}
        onClick={onRequestCheck}
      >
        <i className="codicon codicon-checklist" />{' '}
        {hasSelection ? '检查选中' : '检查文件'}
      </button>

      {/* ===== 生成面板(VS Code 风格右侧停靠聊天面板)===== */}
      {open && (
        <div style={panelWidth ? { ...chatPanelStyle, width: panelWidth } : chatPanelStyle}>
          {/* 顶栏:标题 + 右上角操作(含关闭按钮)*/}
          <div style={chatHeaderStyle}>
            <span style={{ color: '#cccccc', fontWeight: 600 }}>AI 生成</span>
            <span style={chatHeaderUnderline} />
            {genBusy ? (
              <span style={{ opacity: 0.7, fontSize: 12, marginLeft: 8 }}>
                {genCurrentStep}
              </span>
            ) : genCompletedSteps.length > 0 ? (
              <span style={{ opacity: 0.6, fontSize: 12, marginLeft: 8 }}>
                完成 · {genCompletedSteps.length} 步 ·{' '}
                {Math.round(genCompletedSteps.reduce((s, st) => s + (st.elapsed_ms || 0), 0))}ms
              </span>
            ) : null}
            <div style={{ flex: 1 }} />
            {(genCompletedSteps.length > 0 || (genBusy && genCurrentStep)) && (
              <i
                className={`codicon ${stepsExpanded ? 'codicon-chevron-up' : 'codicon-chevron-down'}`}
                title={stepsExpanded ? '收起步骤' : '展开步骤'}
                style={chatHeaderIcon}
                onClick={() => setStepsExpanded((v) => !v)}
              />
            )}
            <i
              className="codicon codicon-history"
              title="历史会话"
              style={chatHeaderIcon}
              onClick={() => setSessionListOpen((v) => !v)}
            />
            <i
              className="codicon codicon-add"
              title="新建会话"
              style={chatHeaderIcon}
              onClick={startNewSession}
            />
            {genBusy ? (
              <i
                className="codicon codicon-stop-circle"
                title="停止生成"
                style={chatHeaderIcon}
                onClick={() => genAbortRef.current?.abort()}
              />
            ) : null}
            <i
              className="codicon codicon-close"
              title="关闭智能体"
              style={chatHeaderIcon}
              onClick={() => {
                if (!genBusy) {
                  onClose?.()
                }
              }}
            />
          </div>

          {/* 历史会话下拉 */}
          {sessionListOpen && (
            <div style={sessionListStyle}>
              {sessions.length === 0 && (
                <div style={{ padding: 10, fontSize: 12, opacity: 0.6 }}>暂无历史会话</div>
              )}
              {sessions.map((s) => (
                <div
                  key={s.id}
                  style={{
                    ...sessionItemStyle,
                    background: s.id === currentSessionId ? '#2a2d2e' : 'transparent',
                  }}
                  onClick={() => switchSession(s.id)}
                >
                  <i className="codicon codicon-comment-discussion" style={{ opacity: 0.7 }} />
                  <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {s.title || '会话'}
                  </span>
                  <span style={{ opacity: 0.4, fontSize: 11 }}>
                    {new Date(s.updated_at).toLocaleDateString()}
                  </span>
                  <i
                    className="codicon codicon-trash"
                    title="删除会话"
                    style={{ opacity: 0.6, cursor: 'pointer' }}
                    onClick={(e) => {
                      e.stopPropagation()
                      void removeSession(s.id)
                    }}
                  />
                </div>
              ))}
            </div>
          )}

          {/* pipeline 步骤:固定在侧边栏顶部,不随历史/结果滚动 */}
          {(() => {
            const hasSteps = genCompletedSteps.length > 0 || (genBusy && genCurrentStep)
            const showPipeline = genBusy ? stepsExpanded : (stepsExpanded && genCompletedSteps.length > 0)
            if (!hasSteps || !showPipeline) return null
            return (
              <div style={pipelineStyle}>
                {genCompletedSteps.map((st, i) => (
                  <div key={i} style={pipelineStepStyle}>
                    <i className="codicon codicon-pass-filled" style={{ color: '#4ec9b0' }} />
                    <span style={{ fontWeight: 500 }}>{st.name}</span>
                    <span style={{ opacity: 0.6 }}>· {GEN_STEP_DESC[st.name] || ''}</span>
                    <span style={{ marginLeft: 'auto', opacity: 0.5 }}>{Math.round(st.elapsed_ms)}ms</span>
                    {st.used_skills && st.used_skills.length > 0 && (
                      <div style={pipelineSkillStyle}>
                        {st.used_skills.map((sk) => (
                          <span key={sk} style={pipelineSkillTagStyle}>{sk}</span>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
                {genBusy && genCurrentStep && !genCompletedSteps.some((s) => s.name === genCurrentStep) && (
                  <div style={{ ...pipelineStepStyle, opacity: 0.6 }}>
                    <i className="codicon codicon-loading codicon-modifier-spin" style={{ color: '#3794ff' }} />
                    <span>{genCurrentStep}</span>
                    <span style={{ opacity: 0.6 }}>· {GEN_STEP_DESC[genCurrentStep] || ''}</span>
                  </div>
                )}
              </div>
            )
          })()}

          {/* 内容区:历史对话流 / 空态引导 / 错误 */}
          <div style={chatBodyStyle}>
            {/* 历史消息回放 */}
            {history.filter((m) => m.kind === 'generate').map((m) => (
              <ManifestHistoryMessage key={m.id} msg={m} onJump={bridge.revealAt} />
            ))}
            {!genBusy && !genStep && !genError && !genCompletedSteps.length && history.filter((m) => m.kind === 'generate').length === 0 && (
              <div style={chatEmptyStyle}>
                <i className="codicon codicon-sparkle" style={{ fontSize: 40, opacity: 0.5 }} />
                <div style={{ fontSize: 15, fontWeight: 600 }}>使用智能体构建</div>
                <div style={{ fontSize: 12, opacity: 0.6 }}>AI 答复可能不准确</div>
              </div>
            )}
            {genError && <div style={errorStyle}>{genError}</div>}
            {genWarnings.length > 0 && (
              <div style={genWarnStyle}>
                <div style={{ fontWeight: 600, marginBottom: 4 }}>
                  <i className="codicon codicon-warning" /> Schema 校验({genWarnings.length})
                </div>
                {genWarnings.map((w, i) => (
                  <div key={i} style={{ fontSize: 12, lineHeight: 1.5 }}>
                    · {w}
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* 底部输入框 */}
          <div style={chatInputWrapStyle}>
            {/* 上下文 chip(VS Code 风格):选区优先显示行范围,否则整个文件 */}
            {aiContext && (
              <div style={contextChipRowStyle}>
                <span style={contextChipStyle} title={fileNameOf(aiContext.filePath)}>
                  <i
                    className={`codicon ${aiContext.kind === 'selection' ? 'codicon-list-selection' : 'codicon-file'}`}
                    style={{ fontSize: 13, opacity: 0.8 }}
                  />
                  <span style={{ fontStyle: 'italic' }}>{fileNameOf(aiContext.filePath)}</span>
                  {aiContext.kind === 'selection' && (
                    <span style={{ color: '#6796e6' }}>
                      :{aiContext.startLine}-{aiContext.endLine}
                    </span>
                  )}
                  <i
                    className="codicon codicon-close"
                    style={{ fontSize: 12, cursor: 'pointer', opacity: 0.7 }}
                    title="移除上下文"
                    onClick={() => setAiContext(null)}
                  />
                </span>
              </div>
            )}
            <textarea
              style={chatInputStyle}
              placeholder="描述要构建的内容(选中代码则基于选区修复)"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault()
                  void runGenerate()
                }
              }}
              disabled={genBusy}
              rows={3}
              autoFocus
            />
            <div style={chatInputFooterStyle}>
              <span style={{ fontSize: 11, opacity: 0.5 }}>Cmd/Ctrl + Enter 发送</span>
              <div style={{ flex: 1 }} />
              <i
                className={`codicon ${genBusy ? 'codicon-loading codicon-modifier-spin' : 'codicon-send'}`}
                title="生成并插入"
                style={{
                  cursor: genBusy || !description.trim() ? 'default' : 'pointer',
                  opacity: genBusy || !description.trim() ? 0.4 : 1,
                  fontSize: 16,
                }}
                onClick={() => !genBusy && description.trim() && void runGenerate()}
              />
            </div>
          </div>
        </div>
      )}

    </>
  )
}

// fileNameOf 取路径末段文件名
function fileNameOf(path: string): string {
  return path.split('/').pop() || path
}

function formatHistoryContext(value: unknown): string {
  if (!value || typeof value !== 'object') return ''
  const ctx = value as Record<string, unknown>
  const path = String(ctx.file_path ?? ctx.filePath ?? '')
  if (!path) return ''
  const kind = String(ctx.kind ?? '')
  const start = Number(ctx.start_line ?? ctx.startLine ?? 0)
  const end = Number(ctx.end_line ?? ctx.endLine ?? 0)
  const range = start > 0 ? `:${start}${end > start ? `-${end}` : ''}` : ''
  return `${kind === 'selection' ? '选区 ' : '文件 '}${path}${range}`
}

function formatHistoryFileItem(value: unknown): string {
  if (typeof value === 'string') return value
  if (!value || typeof value !== 'object') return ''
  const item = value as Record<string, unknown>
  const path = String(item.path ?? item.file ?? '')
  if (!path) return ''
  const start = Number(item.start_line ?? item.startLine ?? 0)
  const end = Number(item.end_line ?? item.endLine ?? 0)
  return start > 0 ? `${path}:${start}${end > start ? `-${end}` : ''}` : path
}

function formatHistoryFiles(value: unknown): string {
  if (!Array.isArray(value)) return ''
  return value.map(formatHistoryFileItem).filter(Boolean).join(', ')
}

// ManifestHistoryMessage 渲染一条历史消息(用户输入 / AI 产出,按 kind 区分)
function ManifestHistoryMessage({
  msg,
  onJump,
}: {
  msg: ManifestAIMessage
  onJump: (path: string, line: number) => void
}) {
  let parsed: Record<string, unknown> = {}
  try {
    parsed = JSON.parse(msg.content)
  } catch {
    /* 容错:内容非 JSON 时按空对象处理 */
  }
  const isUser = msg.role === 'user'
  const label = isUser ? '我' : 'AI'
  const labelColor = isUser ? '#9cdcfe' : '#4ec9b0'

  return (
    <div style={historyMsgStyle}>
      <div style={{ fontSize: 11, color: labelColor, marginBottom: 4 }}>
        {label} · {msg.kind === 'generate' ? '生成' : '检查'}
      </div>
      {/* 用户消息 */}
      {isUser && msg.kind === 'generate' && (
        <div style={historyTextStyle}>
          <div>{String(parsed.description ?? '')}</div>
          {formatHistoryContext(parsed.context) && (
            <div style={{ marginTop: 4, color: '#9cdcfe', fontStyle: 'italic' }}>
              上下文：{formatHistoryContext(parsed.context)}
            </div>
          )}
        </div>
      )}
      {isUser && msg.kind === 'check' && (
        <div style={historyTextStyle}>
          检查:{formatHistoryFiles(parsed.file_contexts ?? parsed.files)}
        </div>
      )}
      {/* AI 产出:生成 → HCL;检查 → 问题列表 */}
      {!isUser && msg.kind === 'generate' && (
        <pre style={historyCodeStyle}>{String(parsed.hcl ?? '')}</pre>
      )}
      {!isUser && msg.kind === 'check' && (
        <div>
          {Array.isArray(parsed.issues) && (parsed.issues as ManifestIssue[]).length > 0 ? (
            (parsed.issues as ManifestIssue[]).map((it, i) => (
              <div
                key={i}
                style={historyIssueStyle}
                onClick={() => onJump(it.file, it.line || 1)}
                title="点击跳转"
              >
                <i
                  className={`codicon ${LEVEL_ICON[it.level] || 'codicon-info'}`}
                  style={{ color: LEVEL_COLOR[it.level] || '#3794ff' }}
                />
                <span style={{ flex: 1 }}>{it.message}</span>
                <span style={{ opacity: 0.5, fontSize: 11 }}>
                  {fileNameOf(it.file)}
                  {it.line ? `:${it.line}` : ''}
                </span>
              </div>
            ))
          ) : (
            <div style={{ fontSize: 12, color: '#4ec9b0' }}>{String(parsed.message ?? '未发现问题')}</div>
          )}
        </div>
      )}
    </div>
  )
}

// isAbortError 判断是否为用户主动取消(fetch abort)产生的异常
function isAbortError(e: unknown): boolean {
  return e instanceof DOMException && e.name === 'AbortError'
}
