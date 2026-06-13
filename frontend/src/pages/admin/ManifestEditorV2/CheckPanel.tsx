/**
 * AI 检查右侧面板 — VS Code 风格对话框
 *
 * 与 AI 生成面板一致的对话体验:
 * - 会话管理(按 manifest+用户隔离)
 * - 历史消息流(生成 + 检查混合展示)
 * - 用户输入检查意见 + 开始检查
 * - 检查结果(issue 列表 + 修复按钮)
 *
 * 供「AI 检查」(工具栏按钮)和「发布前检查」(发布弹窗)共用。
 */
import { useState, useCallback, useRef, useEffect } from 'react'
import type { ManifestIssue, ManifestCompletedStep, ManifestAISession, ManifestAIMessage, ConversationTurn } from '../../../services/manifestAi'
import {
  listAISessions,
  createAISession,
  getAISessionMessages,
  deleteAISession,
} from '../../../services/manifestAi'
import {
  chatPanelStyle,
  chatHeaderStyle,
  chatHeaderUnderline,
  chatHeaderIcon,
  chatBodyStyle,
  chatEmptyStyle,
  pipelineStyle,
  pipelineStepStyle,
  pipelineSkillStyle,
  pipelineSkillTagStyle,
  historyMsgStyle,
  historyTextStyle,
  historyCodeStyle,
  historyIssueStyle,
  sessionListStyle,
  sessionItemStyle,
  chatInputWrapStyle,
  chatInputStyle,
  chatInputFooterStyle,
  contextChipRowStyle,
  contextChipStyle,
  errorStyle,
  issueRowStyle,
  fixBtnStyle,
  fixBtnDisabledStyle,
  fixAppliedBannerStyle,
} from './manifestAiStyles'

const LEVEL_ICON: Record<string, string> = {
  error: 'codicon-error',
  warning: 'codicon-warning',
  info: 'codicon-info',
}
const LEVEL_COLOR: Record<string, string> = {
  error: '#f14c4c',
  warning: '#cca700',
  info: '#3794ff',
}

const CHECK_STEP_DESC: Record<string, string> = {
  初始化: '获取 AI 配置',
  意图断言: '安全守卫:检测内容是否含注入',
  打包内容: '当前文件 + 跨文件引用',
  Skill选择: 'AI 语义选择 Domain + 精确匹配 Module Skill',
  AI检查: '组装 Skill 并调用 AI 检查',
}

// fileNameOf 取路径末段文件名
function fileNameOf(path: string): string {
  return path.split('/').pop() || path
}

function formatHistoryFileItem(value: unknown): string {
  if (typeof value === 'string') return value
  if (!value || typeof value !== 'object') return ''
  const item = value as Record<string, unknown>
  const path = String(item.path ?? item.file ?? '')
  if (!path) return ''
  const start = Number(item.start_line ?? item.startLine ?? 0)
  const end = Number(item.end_line ?? item.endLine ?? 0)
  return start > 0
    ? `${path}:${start}${end > start ? `-${end}` : ''}`
    : path
}

function formatHistoryFiles(value: unknown): string {
  if (!Array.isArray(value)) return ''
  return value.map(formatHistoryFileItem).filter(Boolean).join(', ')
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

/** 检查上下文:当前文件/选区信息(在输入框上方显示为 chip) */
export interface CheckContext {
  kind: 'selection' | 'file'
  filePath: string
  startLine?: number
  endLine?: number
}

export interface CheckPanelProps {
  busy: boolean
  issues: ManifestIssue[]
  completedSteps: ManifestCompletedStep[]
  checkError: string | null
  currentStepName: string
  manifestId: string
  orgId: string
  /** 当前检查的文件/选区上下文(在输入框上方显示) */
  checkContext: CheckContext | null
  onRevealAt: (path: string, line: number) => void
  onApplyFix: (issue: ManifestIssue, idx: number) => Promise<void>
  onStartCheck: (instruction: string, history: ConversationTurn[], sessionId?: string) => void
  onRecheck: () => void
  onSessionChange: () => void
  onClose: () => void
  panelWidth?: number
}

export default function CheckPanel({
  busy,
  issues,
  completedSteps,
  checkError,
  currentStepName,
  manifestId,
  orgId,
  checkContext,
  onRevealAt,
  onApplyFix,
  onStartCheck,
  onRecheck,
  onSessionChange,
  onClose,
  panelWidth,
}: CheckPanelProps) {
  const [stepsExpanded, setStepsExpanded] = useState(true)
  const [copied, setCopied] = useState(false)
  const [instruction, setInstruction] = useState('')

  // 逐条追踪已修复的 issue
  const prevIssuesRef = useRef<ManifestIssue[]>(issues)
  const [fixedIndices, setFixedIndices] = useState<Set<number>>(new Set())
  if (prevIssuesRef.current !== issues) {
    prevIssuesRef.current = issues
    setFixedIndices(new Set())
  }
  const anyFixApplied = fixedIndices.size > 0
  // 是否有过至少一次检查(有 completedSteps 或 checkError 或 issues)
  const hasCheckResult = completedSteps.length > 0 || !!checkError || issues.length > 0

  const handleApplyFix = useCallback(async (issue: ManifestIssue, idx: number) => {
    await onApplyFix(issue, idx)
    setFixedIndices((prev) => new Set([...prev, idx]))
  }, [onApplyFix])

  const handleRecheck = useCallback(() => {
    setFixedIndices(new Set())
    onRecheck()
  }, [onRecheck])

  const copyIssues = useCallback(async () => {
    if (issues.length === 0) return
    const text = issues
      .map((it) => `[${it.level}] ${it.file}:${it.line} ${it.message}`)
      .join('\n')
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }, [issues])

  // ===== 会话管理(与 AI 生成共享同一套 session) =====
  const [sessions, setSessions] = useState<ManifestAISession[]>([])
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null)
  const [history, setHistory] = useState<ManifestAIMessage[]>([])
  const [sessionListOpen, setSessionListOpen] = useState(false)
  const currentSessionRef = useRef<string | null>(null)
  useEffect(() => {
    currentSessionRef.current = currentSessionId
  }, [currentSessionId])

  const loadHistory = useCallback(async (sid: string) => {
    try {
      setHistory(await getAISessionMessages(sid))
    } catch {
      setHistory([])
    }
  }, [])

  const didAutoSelectRef = useRef(false)
  const refreshSessions = useCallback(
    async (selectLatest: boolean) => {
      try {
        const list = await listAISessions(manifestId, orgId)
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
    [manifestId, orgId, loadHistory],
  )

  // 面板打开时加载会话列表
  const didInitRef = useRef(false)
  useEffect(() => {
    if (!didInitRef.current) {
      didInitRef.current = true
      void refreshSessions(true)
    }
  }, [refreshSessions])

  // 检查完成后刷新历史
  const prevBusyRef = useRef(busy)
  useEffect(() => {
    if (prevBusyRef.current && !busy && currentSessionRef.current) {
      void loadHistory(currentSessionRef.current)
      void refreshSessions(false)
    }
    prevBusyRef.current = busy
  }, [busy, loadHistory, refreshSessions])

  const switchSession = useCallback(
    (sid: string) => {
      setCurrentSessionId(sid)
      setSessionListOpen(false)
      onSessionChange()
      void loadHistory(sid)
    },
    [loadHistory, onSessionChange],
  )

  const startNewSession = useCallback(() => {
    setCurrentSessionId(null)
    setHistory([])
    setSessionListOpen(false)
    onSessionChange()
  }, [onSessionChange])

  // 确保有当前会话:无则新建,返回 session_id
  const creatingSessionRef = useRef<Promise<string | undefined> | null>(null)
  const ensureSession = useCallback(async (): Promise<string | undefined> => {
    if (currentSessionRef.current) return currentSessionRef.current
    if (creatingSessionRef.current) return creatingSessionRef.current
    const p = (async () => {
      try {
        const sess = await createAISession(manifestId, orgId, '新会话')
        currentSessionRef.current = sess.id
        setCurrentSessionId(sess.id)
        setSessions((prev) => [sess, ...prev])
        return sess.id
      } catch {
        return undefined
      } finally {
        creatingSessionRef.current = null
      }
    })()
    creatingSessionRef.current = p
    return p
  }, [manifestId, orgId])

  // 从本地历史构建检查对话上下文。检查也需要带上同一会话里的生成历史,
  // 否则"先生成再检查"时模型看不到前一轮生成意图。
  const buildCheckHistory = useCallback((): ConversationTurn[] => {
    if (history.length === 0) return []
    const turns: ConversationTurn[] = []
    for (const msg of history) {
      let parsed: Record<string, unknown> = {}
      try { parsed = JSON.parse(msg.content) } catch { /* ignore */ }
      if (msg.role === 'user') {
        let content = ''
        if (msg.kind === 'generate') {
          const desc = String(parsed.description ?? '')
          const context = formatHistoryContext(parsed.context)
          content = context ? `${desc}\n上下文: ${context}` : desc
        } else {
          const files = formatHistoryFiles(parsed.file_contexts ?? parsed.files)
          const instruction = String(parsed.user_instruction ?? '')
          content = instruction ? `检查 ${files}，关注点：${instruction}` : `检查 ${files}`
        }
        if (!content) continue
        turns.push({ role: 'user', content })
      } else if (msg.role === 'assistant') {
        let content = ''
        if (msg.kind === 'generate') {
          const hcl = String(parsed.hcl ?? '')
          const message = String(parsed.message ?? '')
          content = hcl ? `生成了 HCL 代码:\n\`\`\`hcl\n${hcl.slice(0, 500)}${hcl.length > 500 ? '\n...(已截断)' : ''}\n\`\`\`` : (message || '完成')
        } else {
          const issuesArr = Array.isArray(parsed.issues) ? parsed.issues as ManifestIssue[] : []
          const message = String(parsed.message ?? '')
          content = issuesArr.length > 0
            ? `发现 ${issuesArr.length} 个问题：${issuesArr.slice(0, 5).map((it) => it.message).join('；')}${issuesArr.length > 5 ? '...(已截断)' : ''}`
            : (message || '未发现问题')
        }
        turns.push({ role: 'assistant', content })
      }
    }
    const maxTurns = 12
    return turns.length > maxTurns ? turns.slice(turns.length - maxTurns) : turns
  }, [history])

  const handleStartCheck = useCallback(async () => {
    if (busy) return
    const sid = await ensureSession()
    const convHistory = buildCheckHistory()
    onStartCheck(instruction, convHistory.length > 0 ? convHistory : [], sid)
    setInstruction('')
  }, [busy, instruction, ensureSession, buildCheckHistory, onStartCheck])

  const removeSession = useCallback(
    async (sid: string) => {
      try {
        await deleteAISession(sid)
      } catch {
        /* ignore */
      }
      setSessions((prev) => prev.filter((s) => s.id !== sid))
      if (currentSessionRef.current === sid) {
        setCurrentSessionId(null)
        setHistory([])
      }
    },
    [],
  )

  const errCount = issues.filter((i) => i.level === 'error').length
  const warnCount = issues.filter((i) => i.level === 'warning').length

  return (
    <div style={panelWidth ? { ...chatPanelStyle, width: panelWidth } : chatPanelStyle}>
      {/* 顶栏 */}
      <div style={chatHeaderStyle}>
        <span style={{ color: '#cccccc', fontWeight: 600 }}>AI 检查</span>
        <span style={chatHeaderUnderline} />
        {busy ? (
          <span style={{ opacity: 0.7, fontSize: 12, marginLeft: 8 }}>
            {currentStepName}
          </span>
        ) : completedSteps.length > 0 ? (
          <span style={{ opacity: 0.6, fontSize: 12, marginLeft: 8 }}>
            完成 · {completedSteps.length} 步 ·{' '}
            {Math.round(completedSteps.reduce((s, st) => s + (st.elapsed_ms || 0), 0))}ms
          </span>
        ) : null}
        <div style={{ flex: 1 }} />
        {(completedSteps.length > 0 || (busy && currentStepName)) && (
          <i
            className={`codicon ${stepsExpanded ? 'codicon-chevron-up' : 'codicon-chevron-down'}`}
            title={stepsExpanded ? '收起步骤' : '展开步骤'}
            style={chatHeaderIcon}
            onClick={() => setStepsExpanded((v) => !v)}
          />
        )}
        {issues.length > 0 && (
          <i
            className={`codicon ${copied ? 'codicon-check' : 'codicon-copy'}`}
            title="复制全部检查结果"
            style={chatHeaderIcon}
            onClick={() => void copyIssues()}
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
        <i className="codicon codicon-close" title="关闭" style={chatHeaderIcon} onClick={onClose} />
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
        const hasSteps = completedSteps.length > 0 || (busy && currentStepName)
        const showPipeline = busy ? stepsExpanded : (stepsExpanded && completedSteps.length > 0)
        if (!hasSteps || !showPipeline) return null
        return (
          <div style={pipelineStyle}>
            {completedSteps.map((st, i) => (
              <div key={i} style={pipelineStepStyle}>
                <i className="codicon codicon-pass-filled" style={{ color: '#4ec9b0' }} />
                <span style={{ fontWeight: 500 }}>{st.name}</span>
                <span style={{ opacity: 0.6 }}>· {CHECK_STEP_DESC[st.name] || ''}</span>
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
            {busy && currentStepName && !completedSteps.some((s) => s.name === currentStepName) && (
              <div style={{ ...pipelineStepStyle, opacity: 0.6 }}>
                <i className="codicon codicon-loading codicon-modifier-spin" style={{ color: '#3794ff' }} />
                <span>{currentStepName}</span>
                <span style={{ opacity: 0.6 }}>· {CHECK_STEP_DESC[currentStepName] || ''}</span>
              </div>
            )}
          </div>
        )
      })()}

      {/* 内容区:历史对话流 + 当前检查结果 */}
      <div style={chatBodyStyle}>
        {/* 历史消息回放(显示同一会话里的生成 + 检查消息,供检查上下文可视化) */}
        {history.map((m) => (
          <CheckHistoryMessage key={m.id} msg={m} onJump={onRevealAt} />
        ))}

        {/* 空态 */}
        {!busy && !hasCheckResult && history.length === 0 && (
          <div style={chatEmptyStyle}>
            <i className="codicon codicon-checklist" style={{ fontSize: 40, opacity: 0.5 }} />
            <div style={{ fontSize: 15, fontWeight: 600 }}>AI 检查</div>
            <div style={{ fontSize: 12, opacity: 0.6 }}>输入关注点或直接开始检查</div>
          </div>
        )}

        {/* 检查中 */}
        {busy && (
          <div style={{ padding: 12, opacity: 0.7 }}>正在检查...</div>
        )}

        {/* 错误 */}
        {checkError && (
          <div style={errorStyle}>
            <i className="codicon codicon-error" style={{ marginRight: 6 }} />
            {checkError}
            <span
              style={{ color: '#9cdcfe', cursor: 'pointer', marginLeft: 8 }}
              onClick={handleRecheck}
            >
              重试
            </span>
          </div>
        )}

        {/* 修复提示 */}
        {anyFixApplied && (
          <div style={fixAppliedBannerStyle}>
            <i className="codicon codicon-info" /> 已应用修复,内容已变更,建议
            <span
              style={{ color: '#9cdcfe', cursor: 'pointer', marginLeft: 4 }}
              onClick={handleRecheck}
            >
              重新检查
            </span>
          </div>
        )}

        {/* 结果摘要 */}
        {!busy && !checkError && hasCheckResult && (
          <div style={{ padding: '8px 12px', fontSize: 13 }}>
            {issues.length === 0 ? (
              <span style={{ color: '#4ec9b0' }}>
                <i className="codicon codicon-pass-filled" style={{ marginRight: 4 }} />
                未发现问题
              </span>
            ) : (
              <span>
                发现 {issues.length} 个问题
                {errCount > 0 && <span style={{ color: LEVEL_COLOR.error, marginLeft: 8 }}>{errCount} error</span>}
                {warnCount > 0 && <span style={{ color: LEVEL_COLOR.warning, marginLeft: 8 }}>{warnCount} warning</span>}
              </span>
            )}
          </div>
        )}

        {/* issue 列表 */}
        {issues.map((it, i) => (
          <div key={i} style={issueRowStyle}>
            <i
              className={`codicon ${LEVEL_ICON[it.level] || 'codicon-info'}`}
              style={{ color: LEVEL_COLOR[it.level] || '#3794ff' }}
            />
            <span
              style={{ flex: 1, cursor: 'pointer', fontSize: 13 }}
              onClick={() => onRevealAt(it.file, it.line || 1)}
              title="点击跳转到该位置"
            >
              {it.message}
            </span>
            {it.fix && (
              <button
                style={fixedIndices.has(i) ? fixBtnDisabledStyle : fixBtnStyle}
                disabled={fixedIndices.has(i)}
                onClick={(e) => {
                  e.stopPropagation()
                  void handleApplyFix(it, i)
                }}
                title={fixedIndices.has(i) ? '该修复已应用' : '应用该修复'}
              >
                <i className="codicon codicon-wand" /> 修复
              </button>
            )}
            <span
              style={{ opacity: 0.6, fontSize: 12, cursor: 'pointer' }}
              onClick={() => onRevealAt(it.file, it.line || 1)}
            >
              {it.file}{it.line ? `:${it.line}` : ''}
            </span>
          </div>
        ))}
      </div>

      {/* 底部输入框 */}
      <div style={chatInputWrapStyle}>
        {/* 上下文 chip:显示当前检查的文件/选区 */}
        {checkContext && (
          <div style={contextChipRowStyle}>
            <span style={contextChipStyle} title={fileNameOf(checkContext.filePath)}>
              <i
                className={`codicon ${checkContext.kind === 'selection' ? 'codicon-list-selection' : 'codicon-file'}`}
                style={{ fontSize: 13, opacity: 0.8 }}
              />
              <span style={{ fontStyle: 'italic' }}>{fileNameOf(checkContext.filePath)}</span>
              {checkContext.kind === 'selection' && checkContext.startLine && (
                <span style={{ color: '#6796e6' }}>
                  :{checkContext.startLine}{checkContext.endLine ? `-${checkContext.endLine}` : ''}
                </span>
              )}
            </span>
          </div>
        )}
        <textarea
          style={chatInputStyle}
          placeholder={'输入检查关注点（可选），如「重点检查安全组」'}
          value={instruction}
          onChange={(e) => setInstruction(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              handleStartCheck()
            }
          }}
          disabled={busy}
          rows={2}
          autoFocus
        />
        <div style={chatInputFooterStyle}>
          <span style={{ fontSize: 11, opacity: 0.5 }}>Cmd/Ctrl + Enter 发送</span>
          <div style={{ flex: 1 }} />
          <i
            className={`codicon ${busy ? 'codicon-loading codicon-modifier-spin' : 'codicon-play'}`}
            title="开始检查"
            style={{
              cursor: busy ? 'default' : 'pointer',
              opacity: busy ? 0.4 : 1,
              fontSize: 16,
            }}
            onClick={() => !busy && handleStartCheck()}
          />
        </div>
      </div>
    </div>
  )
}

// CheckHistoryMessage 渲染一条历史消息(用户输入 / AI 产出,按 kind 区分)
function CheckHistoryMessage({
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
    /* 容错 */
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
          <div>检查:{formatHistoryFiles(parsed.file_contexts ?? parsed.files)}</div>
          {Boolean(parsed.user_instruction) && (
            <div style={{ marginTop: 4, color: '#e2c08d', fontStyle: 'italic' }}>
              关注点：{String(parsed.user_instruction)}
            </div>
          )}
        </div>
      )}
      {/* AI 产出 */}
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
