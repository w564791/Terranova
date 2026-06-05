/**
 * Manifest 编辑器 AI 工具 — 资源生成/修复 + 草稿检查
 *
 * 自包含组件:渲染两个工具栏按钮 + 生成弹窗 + 底部问题面板。
 * 与编辑器通过 EditorBridge 解耦,父组件用现有 editorRef / openAt 拼出 bridge。
 *
 * 两个能力都走 SSE,进度展示与 form_generation 一致(步骤名 + 已耗时)。
 */
import { useCallback, useRef, useState } from 'react'
import {
  generateManifestResource,
  checkManifestDraft,
  type ManifestProgressEvent,
  type ManifestIssue,
} from '../../../services/manifestAi'

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

/** 编辑器桥接:父组件用现有 editorRef/openAt 实现 */
export interface EditorBridge {
  contextIds: { workspace_id?: string; organization_id?: string }
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
}

interface Props {
  bridge: EditorBridge
  disabled?: boolean
}

const LEVEL_COLOR: Record<string, string> = {
  error: '#f14c4c',
  warning: '#cca700',
  info: '#3794ff',
}
const LEVEL_ICON: Record<string, string> = {
  error: 'codicon-error',
  warning: 'codicon-warning',
  info: 'codicon-info',
}

export default function ManifestAiTools({ bridge, disabled }: Props) {
  // 生成弹窗
  const [genOpen, setGenOpen] = useState(false)
  const [description, setDescription] = useState('')
  const [genBusy, setGenBusy] = useState(false)
  const [genStep, setGenStep] = useState<ManifestProgressEvent | null>(null)
  const [genError, setGenError] = useState<string | null>(null)
  // 打开面板时快照的上下文(选区优先,否则当前文件),作为 chip 展示,用户可移除
  const [aiContext, setAiContext] = useState<AiContext | null>(null)

  // 打开生成面板:重置上次残留状态 + 快照上下文。选区优先,无选区回退到当前文件。
  const openGenerate = useCallback(() => {
    setGenStep(null)
    setGenError(null)
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
    setGenOpen(true)
  }, [bridge])

  // 检查面板
  const [checkBusy, setCheckBusy] = useState(false)
  const [checkStep, setCheckStep] = useState<ManifestProgressEvent | null>(null)
  const [issues, setIssues] = useState<ManifestIssue[] | null>(null)
  const [checkError, setCheckError] = useState<string | null>(null)

  // 生成与 Check 各自独立的 abort 控制器,互不覆盖
  const genAbortRef = useRef<AbortController | null>(null)
  const checkAbortRef = useRef<AbortController | null>(null)

  // ===== 生成/修复 =====
  const runGenerate = useCallback(async () => {
    if (!description.trim() || genBusy) return
    setGenBusy(true)
    setGenError(null)
    setGenStep(null)
    const controller = new AbortController()
    genAbortRef.current = controller

    try {
      const result = await generateManifestResource(
        {
          description: description.trim(),
          currentContent: aiContext?.text || undefined,
          contextIds: bridge.contextIds,
        },
        (ev) => setGenStep(ev),
        controller.signal,
      )

      if (result.status === 'blocked') {
        setGenError(result.message || '请求被安全策略拦截')
        return
      }
      if (result.hcl) {
        bridge.insertText(result.hcl)
        setDescription('')
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
  }, [description, genBusy, bridge, aiContext])

  // ===== 检查 =====
  const runCheck = useCallback(async () => {
    if (checkBusy) return
    const filePath = bridge.getActiveFilePath()
    if (!filePath) return
    const sel = bridge.getSelectionInfo()
    const content = sel?.text || bridge.getActiveFileContent()
    if (!content.trim()) return

    // 选区检查时把选区在文件中的起始行号传给后端,后端按绝对行号给每行加前缀,
    // AI 直接引用前缀里的行号,返回的就是文件绝对行号,前端无需再做偏移。
    const startLine = sel ? sel.startLine : 1

    setCheckBusy(true)
    setCheckError(null)
    setCheckStep(null)
    setIssues(null)
    const controller = new AbortController()
    checkAbortRef.current = controller

    try {
      const result = await checkManifestDraft(
        { filePath, content, startLine, contextIds: bridge.contextIds },
        (ev) => setCheckStep(ev),
        controller.signal,
      )
      setIssues(result.issues)
    } catch (e) {
      if (!isAbortError(e)) {
        setCheckError(e instanceof Error ? e.message : '检查失败')
      }
    } finally {
      setCheckBusy(false)
      checkAbortRef.current = null
    }
  }, [checkBusy, bridge])

  const closeIssues = () => {
    setIssues(null)
    setCheckError(null)
    setCheckStep(null)
  }

  const showIssuePanel = checkBusy || issues !== null || checkError !== null

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
        title="用 AI 检查当前文件/选区的基本问题"
        disabled={disabled || checkBusy}
        onClick={() => void runCheck()}
      >
        <i className={`codicon ${checkBusy ? 'codicon-loading codicon-modifier-spin' : 'codicon-checklist'}`} /> Check
      </button>

      {/* ===== 生成面板(VS Code 风格右侧停靠聊天面板)===== */}
      {genOpen && (
        <div style={chatPanelStyle}>
          {/* 顶栏:标题 + 右上角操作(含关闭按钮)*/}
          <div style={chatHeaderStyle}>
            <span style={{ color: '#cccccc', fontWeight: 600 }}>AI 生成</span>
            <span style={chatHeaderUnderline} />
            <div style={{ flex: 1 }} />
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
              onClick={() => !genBusy && setGenOpen(false)}
            />
          </div>

          {/* 内容区:空态引导 / 进度 / 错误 */}
          <div style={chatBodyStyle}>
            {!genBusy && !genStep && !genError && (
              <div style={chatEmptyStyle}>
                <i className="codicon codicon-sparkle" style={{ fontSize: 40, opacity: 0.5 }} />
                <div style={{ fontSize: 15, fontWeight: 600 }}>使用智能体构建</div>
                <div style={{ fontSize: 12, opacity: 0.6 }}>AI 答复可能不准确</div>
              </div>
            )}
            {genStep && (
              <div style={chatProgressStyle}>
                <i
                  className={`codicon ${
                    genStep.type === 'complete' ? 'codicon-check' : 'codicon-loading codicon-modifier-spin'
                  }`}
                />
                <span>
                  {genStep.step_name}
                  {genStep.total_steps ? ` (${genStep.step}/${genStep.total_steps})` : ''}
                  {genStep.message ? ` — ${genStep.message}` : ''}
                </span>
                {genStep.elapsed_ms ? (
                  <span style={{ opacity: 0.5, marginLeft: 'auto' }}>{Math.round(genStep.elapsed_ms)}ms</span>
                ) : null}
              </div>
            )}
            {genError && <div style={errorStyle}>{genError}</div>}
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

      {/* ===== 底部问题面板 ===== */}
      {showIssuePanel && (
        <div style={issuePanelStyle}>
          <div style={issueHeaderStyle}>
            <i className="codicon codicon-checklist" />
            <span>
              问题
              {issues !== null ? ` (${issues.length})` : ''}
            </span>
            {checkBusy && checkStep && (
              <span style={{ opacity: 0.7, fontSize: 12 }}>
                {checkStep.step_name}
                {checkStep.total_steps ? ` (${checkStep.step}/${checkStep.total_steps})` : ''}
              </span>
            )}
            <div style={{ flex: 1 }} />
            <i className="codicon codicon-close" style={{ cursor: 'pointer' }} onClick={closeIssues} />
          </div>
          <div style={issueBodyStyle}>
            {checkBusy && <div style={{ padding: 12, opacity: 0.7 }}>正在检查...</div>}
            {checkError && <div style={{ ...errorStyle, margin: 12 }}>{checkError}</div>}
            {issues !== null && issues.length === 0 && !checkBusy && (
              <div style={{ padding: 12, color: '#4ec9b0' }}>未发现问题。</div>
            )}
            {issues?.map((it, i) => (
              <div
                key={i}
                style={issueRowStyle}
                onClick={() => bridge.revealAt(it.file, it.line || 1)}
                title="点击跳转到该位置"
              >
                <i
                  className={`codicon ${LEVEL_ICON[it.level] || 'codicon-info'}`}
                  style={{ color: LEVEL_COLOR[it.level] || '#3794ff' }}
                />
                <span style={{ flex: 1 }}>{it.message}</span>
                <span style={{ opacity: 0.6, fontSize: 12 }}>
                  {it.file}
                  {it.line ? `:${it.line}` : ''}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </>
  )
}

// ===== 内联样式(贴合 VS Code 暗色主题)=====
// 右侧停靠聊天面板
const chatPanelStyle: React.CSSProperties = {
  position: 'absolute',
  top: 65, // 让出 titleBar(30) + toolbar(35)
  right: 0,
  bottom: 22, // 让出 statusBar
  width: 360,
  maxWidth: '50vw',
  background: '#1e1e1e',
  borderLeft: '1px solid #2d2d2d',
  display: 'flex',
  flexDirection: 'column',
  zIndex: 60,
}
const chatHeaderStyle: React.CSSProperties = {
  position: 'relative',
  display: 'flex',
  alignItems: 'center',
  gap: 6,
  padding: '8px 12px',
  borderBottom: '1px solid #2d2d2d',
  fontSize: 13,
}
const chatHeaderUnderline: React.CSSProperties = {
  position: 'absolute',
  left: 12,
  bottom: -1,
  width: 28,
  height: 2,
  background: '#e8843c',
}
const chatHeaderIcon: React.CSSProperties = {
  cursor: 'pointer',
  fontSize: 15,
  opacity: 0.8,
  padding: '0 4px',
  color: '#cccccc',
}
const chatBodyStyle: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
  padding: 12,
  color: '#cccccc',
}
const chatEmptyStyle: React.CSSProperties = {
  height: '100%',
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 8,
  color: '#cccccc',
}
const chatProgressStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  fontSize: 13,
  color: '#9cdcfe',
  padding: '6px 0',
}
const chatInputWrapStyle: React.CSSProperties = {
  margin: 12,
  border: '1px solid #3c3c3c',
  borderRadius: 6,
  background: '#252526',
}
const contextChipRowStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 6,
  padding: '8px 8px 0',
}
const contextChipStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  padding: '2px 8px',
  border: '1px solid #3c3c3c',
  borderRadius: 4,
  background: '#2d2d2d',
  fontSize: 12,
  color: '#cccccc',
  maxWidth: '100%',
}
const chatInputStyle: React.CSSProperties = {
  width: '100%',
  background: 'transparent',
  color: '#cccccc',
  border: 'none',
  outline: 'none',
  padding: 10,
  fontFamily: 'inherit',
  fontSize: 13,
  resize: 'none',
  boxSizing: 'border-box',
}
const chatInputFooterStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '4px 10px 8px',
  color: '#cccccc',
}
const errorStyle: React.CSSProperties = {
  marginTop: 10,
  padding: '6px 10px',
  background: 'rgba(241,76,76,0.12)',
  border: '1px solid rgba(241,76,76,0.4)',
  borderRadius: 4,
  color: '#f14c4c',
  fontSize: 13,
}
const issuePanelStyle: React.CSSProperties = {
  position: 'absolute',
  left: 48, // 让出 activityBar
  right: 0,
  bottom: 22, // 让出底部 statusBar
  height: 200,
  background: '#1e1e1e',
  borderTop: '1px solid #454545',
  display: 'flex',
  flexDirection: 'column',
  zIndex: 50,
}
const issueHeaderStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '6px 12px',
  borderBottom: '1px solid #2d2d2d',
  background: '#252526',
  color: '#cccccc',
  fontSize: 13,
  textTransform: 'uppercase',
}
const issueBodyStyle: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
}
const issueRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '4px 12px',
  cursor: 'pointer',
  fontSize: 13,
  color: '#cccccc',
  borderBottom: '1px solid #2a2a2a',
}

// fileNameOf 取路径末段文件名
function fileNameOf(path: string): string {
  return path.split('/').pop() || path
}

// isAbortError 判断是否为用户主动取消(fetch abort)产生的异常
function isAbortError(e: unknown): boolean {
  return e instanceof DOMException && e.name === 'AbortError'
}
