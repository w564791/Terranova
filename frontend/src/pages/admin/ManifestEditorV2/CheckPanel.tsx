/**
 * 通用检查结果右侧面板 — VS Code 风格
 *
 * 嵌入编辑器右侧(和 AI 生成面板布局一致)。
 * 点 issue 行 → 跳转代码(不移除);点修复按钮 → 应用修复并移除该项。
 *
 * 供「AI 检查」(工具栏按钮)和「发布前检查」(发布弹窗)共用。
 */
import { useState, useCallback, useMemo, useRef } from 'react'
import type { ManifestIssue, ManifestCompletedStep } from '../../../services/manifestAi'
import {
  chatPanelStyle,
  chatHeaderStyle,
  chatHeaderUnderline,
  chatHeaderIcon,
  chatBodyStyle,
  pipelineStyle,
  pipelineStepStyle,
  pipelineSkillStyle,
  pipelineSkillTagStyle,
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
  AI检查: '组装 Skill 并调用 AI 检查',
}

export interface CheckPanelProps {
  busy: boolean
  issues: ManifestIssue[]
  completedSteps: ManifestCompletedStep[]
  checkError: string | null
  currentStepName: string
  onRevealAt: (path: string, line: number) => void
  onApplyFix: (issue: ManifestIssue, idx: number) => Promise<void>
  onRecheck: () => void
  onClose: () => void
}

export default function CheckPanel({
  busy,
  issues,
  completedSteps,
  checkError,
  currentStepName,
  onRevealAt,
  onApplyFix,
  onRecheck,
  onClose,
}: CheckPanelProps) {
  const [stepsExpanded, setStepsExpanded] = useState(true)
  const [copied, setCopied] = useState(false)

  // 逐条追踪已修复的 issue,避免单一 boolean 导致全部修复按钮被禁用
  const prevIssuesRef = useRef<ManifestIssue[]>(issues)
  const [fixedIndices, setFixedIndices] = useState<Set<number>>(new Set())
  if (prevIssuesRef.current !== issues) {
    prevIssuesRef.current = issues
    setFixedIndices(new Set())
  }
  const anyFixApplied = fixedIndices.size > 0

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

  const errCount = issues.filter((i) => i.level === 'error').length
  const warnCount = issues.filter((i) => i.level === 'warning').length

  return (
    <div style={chatPanelStyle}>
      {/* 顶栏 */}
      <div style={chatHeaderStyle}>
        <span style={{ color: '#cccccc', fontWeight: 600 }}>检查结果</span>
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
        <i className="codicon codicon-close" title="关闭" style={chatHeaderIcon} onClick={onClose} />
      </div>

      {/* 内容 */}
      <div style={chatBodyStyle}>
        {/* pipeline 步骤:检查中默认展示(可通过 toggle 收起),完成后需用户展开 */}
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

        {/* 检查中 */}
        {busy && (
          <div style={{ padding: 12, opacity: 0.7 }}>正在检查...</div>
        )}

        {/* 错误 */}
        {checkError && (
          <div style={{ padding: '8px 12px', color: '#f14c4c', fontSize: 13 }}>
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
        {!busy && !checkError && (
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

        {/* 重新检查按钮 */}
        {!busy && (
          <div style={{ padding: '8px 12px' }}>
            <button
              style={{
                padding: '4px 12px',
                background: '#0e639c',
                color: '#fff',
                border: 'none',
                borderRadius: 2,
                cursor: 'pointer',
                fontSize: 13,
              }}
              onClick={handleRecheck}
            >
              <i className="codicon codicon-refresh" style={{ marginRight: 4 }} />
              重新检查
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
