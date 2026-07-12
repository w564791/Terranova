/**
 * Run 面板 — VS Code 右侧停靠面板(与 AI 生成面板布局一致)
 *
 * 行为:
 *  1. 列出已装本 manifest 的 workspaces (deployments status='active')
 *  2. 用户选一个,前端把当前草稿全量打包成 external_files
 *  3. 调 POST /workspaces/:id/tasks/plan 带 external_files
 *  4. 提交后不跳转,面板内通过 WebSocket 实时展示任务输出日志
 *  5. 任务完成/WebSocket 关闭后,自动 HTTP 轮询兜底获取 plan_output
 *  6. 关闭面板后保留上次任务,重新打开可查看历史日志
 *
 * 灰禁条件:本 manifest 没有任何 active deployment (toolbar 直接 disabled)
 */
import { useEffect, useState, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { message } from 'antd'
import {
  listFiles,
  readFile,
  listDeployments,
  runPlanWithDraft,
  type ManifestEditorContext,
  type ManifestDeployment,
} from './manifestApi'
import { workspaceService, type Workspace } from '../../../services/workspaces'
import api from '../../../services/api'
import { useTerraformOutput } from '../../../hooks/useTerraformOutput'
import {
  chatPanelStyle,
  chatHeaderStyle,
  chatHeaderUnderline,
  chatHeaderIcon,
  chatBodyStyle,
  errorStyle,
} from './manifestAiStyles'

interface Props {
  open: boolean
  ctx: ManifestEditorContext
  lastRunTask: { taskId: number; workspaceId: string } | null
  viewLast: boolean
  onRunTaskCreated: (taskId: number, workspaceId: string) => void
  onClose: () => void
  panelWidth?: number
}

interface RunTarget {
  deployment: ManifestDeployment
  workspace: Workspace
}

// ===== 内联样式(VS Code 暗色主题)=====

const formGroupStyle: React.CSSProperties = {
  marginBottom: 12,
}

const labelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: 12,
  color: '#999',
  marginBottom: 4,
}

const selectStyle: React.CSSProperties = {
  width: '100%',
  padding: '4px 8px',
  background: '#3c3c3c',
  color: '#cccccc',
  border: '1px solid #454545',
  borderRadius: 2,
  fontSize: 13,
  outline: 'none',
  boxSizing: 'border-box',
  fontFamily: 'inherit',
}

const btnPrimaryStyle: React.CSSProperties = {
  padding: '4px 14px',
  background: '#0e639c',
  color: '#fff',
  border: 'none',
  borderRadius: 2,
  cursor: 'pointer',
  fontSize: 13,
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
}

const btnPrimaryDisabledStyle: React.CSSProperties = {
  ...btnPrimaryStyle,
  background: '#3a3a3a',
  color: '#666',
  cursor: 'default',
}

const btnSecondaryStyle: React.CSSProperties = {
  padding: '4px 14px',
  background: '#3a3a3a',
  color: '#cccccc',
  border: 'none',
  borderRadius: 2,
  cursor: 'pointer',
  fontSize: 13,
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
}

const hintStyle: React.CSSProperties = {
  padding: '8px 12px',
  background: 'rgba(55,148,255,0.1)',
  border: '1px solid rgba(55,148,255,0.3)',
  borderRadius: 4,
  color: '#9cdcfe',
  fontSize: 12,
  marginBottom: 12,
  display: 'flex',
  alignItems: 'center',
  gap: 6,
}

const warnBoxStyle: React.CSSProperties = {
  padding: '8px 12px',
  background: 'rgba(204,167,0,0.12)',
  border: '1px solid rgba(204,167,0,0.3)',
  borderRadius: 4,
  color: '#cca700',
  fontSize: 12,
  marginBottom: 12,
  display: 'flex',
  alignItems: 'center',
  gap: 6,
}

const logContainerStyle: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
  background: '#1b1b1b',
  border: '1px solid #2d2d2d',
  borderRadius: 4,
  padding: '8px 12px',
  fontFamily: '"Cascadia Code", "Fira Code", Menlo, Monaco, monospace',
  fontSize: 12,
  lineHeight: 1.6,
  color: '#d4d4d4',
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-all',
}

const statusBadgeStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 6,
  padding: '4px 10px',
  borderRadius: 4,
  fontSize: 12,
  marginBottom: 8,
}

const taskLinkStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  color: '#9cdcfe',
  fontSize: 12,
  cursor: 'pointer',
  marginTop: 8,
}

const lastRunBannerStyle: React.CSSProperties = {
  padding: '8px 12px',
  background: 'rgba(78,201,176,0.1)',
  border: '1px solid rgba(78,201,176,0.3)',
  borderRadius: 4,
  color: '#4ec9b0',
  fontSize: 12,
  marginBottom: 12,
  display: 'flex',
  alignItems: 'center',
  gap: 8,
}

// ===== HTTP 兜底:从 task API 获取 plan_output =====

interface TaskDetail {
  status: string
  plan_output?: string
  apply_output?: string
  error_message?: string
  stage?: string
}

function useTaskPolling(
  workspaceId: string | undefined,
  taskId: number | null,
  enabled: boolean,
) {
  const [taskDetail, setTaskDetail] = useState<TaskDetail | null>(null)
  const [polling, setPolling] = useState(false)
  const [pollError, setPollError] = useState<string | null>(null)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchTask = useCallback(async () => {
    if (!workspaceId || !taskId) return
    try {
      const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}`)
      const task = data.task || data
      setTaskDetail({
        status: task.status,
        plan_output: task.plan_output,
        apply_output: task.apply_output,
        error_message: task.error_message,
        stage: task.stage,
      })
      setPollError(null)
      // 终态停止轮询
      if (['success', 'failed', 'cancelled', 'error'].includes(task.status)) {
        setPolling(false)
      }
    } catch (err: any) {
      setPollError(err?.message || '获取任务状态失败')
    }
  }, [workspaceId, taskId])

  useEffect(() => {
    if (!enabled || !workspaceId || !taskId) return
    setPolling(true)
    void fetchTask()
    intervalRef.current = setInterval(() => void fetchTask(), 3000)
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
      intervalRef.current = null
    }
  }, [enabled, workspaceId, taskId, fetchTask])

  return { taskDetail, polling, pollError, refetch: fetchTask }
}

// ===== 输出日志渲染 =====

function TaskOutputLog({
  taskId,
  workspaceId,
}: {
  taskId: number
  workspaceId: string
}) {
  const { lines, isConnected, isCompleted, error } = useTerraformOutput(taskId)
  const containerRef = useRef<HTMLDivElement>(null)

  // WebSocket 关闭后启用 HTTP 兜底
  const httpEnabled = isCompleted || (!isConnected && lines.length === 0)
  const { taskDetail, polling, pollError, refetch } = useTaskPolling(
    workspaceId,
    taskId,
    httpEnabled,
  )

  // 自动滚动到底部
  useEffect(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight
    }
  }, [lines, taskDetail?.plan_output])

  // WebSocket 有实时日志
  const hasWsLines = lines.length > 0
  // HTTP 兜底有 plan_output
  const hasHttpOutput = !!taskDetail?.plan_output
  // 任务状态
  const taskStatus = taskDetail?.status
  const isTerminal = taskStatus && ['success', 'failed', 'cancelled', 'error'].includes(taskStatus)

  // 状态徽章
  let statusColor: string
  let statusBg: string
  let statusIcon: string
  let statusText: string

  if (isCompleted && hasWsLines) {
    statusColor = '#4ec9b0'
    statusBg = 'rgba(78,201,176,0.12)'
    statusIcon = 'codicon-check'
    statusText = '任务完成 (实时日志)'
  } else if (isConnected) {
    statusColor = '#3794ff'
    statusBg = 'rgba(55,148,255,0.12)'
    statusIcon = 'codicon-loading codicon-modifier-spin'
    statusText = '实时输出中'
  } else if (polling) {
    statusColor = '#cca700'
    statusBg = 'rgba(204,167,0,0.12)'
    statusIcon = 'codicon-loading codicon-modifier-spin'
    statusText = taskStatus
      ? `状态: ${taskStatus}`
      : '获取日志中...'
  } else if (isTerminal) {
    statusColor = taskStatus === 'success' ? '#4ec9b0' : 'var(--red)'
    statusBg = taskStatus === 'success' ? 'rgba(78,201,176,0.12)' : 'rgba(241,76,76,0.12)'
    statusIcon = taskStatus === 'success' ? 'codicon-check' : 'codicon-error'
    statusText = `任务${taskStatus === 'success' ? '完成' : '失败'}`
  } else {
    statusColor = '#cca700'
    statusBg = 'rgba(204,167,0,0.12)'
    statusIcon = 'codicon-warning'
    statusText = '连接断开'
  }

  // 显示内容
  const displayOutput = taskDetail?.plan_output || taskDetail?.apply_output || taskDetail?.error_message

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div
        style={{
          ...statusBadgeStyle,
          background: statusBg,
          color: statusColor,
        }}
      >
        <i className={`codicon ${statusIcon}`} />
        {statusText}
        {isTerminal && !hasHttpOutput && !hasWsLines && (
          <span
            style={{ ...taskLinkStyle, marginTop: 0, marginLeft: 'auto' }}
            onClick={() => void refetch()}
          >
            <i className="codicon codicon-refresh" /> 刷新
          </span>
        )}
      </div>

      {error && <div style={errorStyle}>{error}</div>}
      {pollError && <div style={errorStyle}>{pollError}</div>}

      <div ref={containerRef} style={logContainerStyle}>
        {/* WebSocket 实时日志 */}
        {hasWsLines &&
          lines.map((line, i) => (
            <div
              key={i}
              style={{
                color:
                  line.type === 'error'
                    ? 'var(--red)'
                    : line.type === 'stage_marker'
                      ? '#4ec9b0'
                      : '#d4d4d4',
                fontWeight: line.type === 'stage_marker' ? 600 : 400,
              }}
            >
              {line.line ?? ''}
            </div>
          ))}

        {/* HTTP 兜底:plan_output */}
        {!hasWsLines && hasHttpOutput && (
          <div style={{ whiteSpace: 'pre-wrap' }}>{displayOutput}</div>
        )}

        {/* 等待中 */}
        {!hasWsLines && !hasHttpOutput && isConnected && (
          <span style={{ opacity: 0.5 }}>等待输出...</span>
        )}

        {/* 无数据 */}
        {!hasWsLines && !hasHttpOutput && !isConnected && !polling && (
          <span style={{ opacity: 0.5 }}>
            {isTerminal ? '暂无日志输出' : '连接中...'}
          </span>
        )}
      </div>
    </div>
  )
}

// ===== 组件 =====

export default function RunDialog({
  open,
  ctx,
  lastRunTask,
  viewLast,
  onRunTaskCreated,
  onClose,
  panelWidth,
}: Props) {
  const navigate = useNavigate()
  const [targets, setTargets] = useState<RunTarget[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [selected, setSelected] = useState<string | undefined>()
  // 当前正在查看的任务(null = 选择 workspace 界面)
  const [viewingTaskId, setViewingTaskId] = useState<number | null>(null)
  const [viewingWorkspaceId, setViewingWorkspaceId] = useState<string | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // viewLast 变化时自动跳到上次任务
  useEffect(() => {
    if (viewLast && lastRunTask) {
      setViewingTaskId(lastRunTask.taskId)
      setViewingWorkspaceId(lastRunTask.workspaceId)
    }
  }, [viewLast, lastRunTask])

  // 加载 targets
  useEffect(() => {
    if (!open) return
    setLoading(true)
    Promise.all([
      listDeployments(ctx).catch(() => []),
      workspaceService
        .getWorkspaces()
        .then((r) => {
          const d: any = (r as any)?.data
          return Array.isArray(d?.items) ? d.items : Array.isArray(d) ? d : []
        })
        .catch(() => []),
    ])
      .then(([deployments, workspaces]) => {
        const wsByID = new Map<string, Workspace>()
        ;(workspaces as Workspace[]).forEach((w) => {
          wsByID.set(w.workspace_id || String(w.id), w)
        })
        const list: RunTarget[] = deployments
          .filter((d) => d.status === 'active')
          .map((d) => {
            const ws = wsByID.get(d.workspace_id)
            return ws ? { deployment: d, workspace: ws } : null
          })
          .filter((x): x is RunTarget => x !== null)
        setTargets(list)
        if (list.length > 0) setSelected(list[0].workspace.workspace_id || String(list[0].workspace.id))
      })
      .finally(() => setLoading(false))
  }, [open, ctx])

  // 关闭面板
  const handleClose = useCallback(() => {
    setViewingTaskId(null)
    setViewingWorkspaceId(null)
    setSubmitError(null)
    setSubmitting(false)
    onClose()
  }, [onClose])

  // 查看上次运行结果
  const handleViewLast = useCallback(() => {
    if (lastRunTask) {
      setViewingTaskId(lastRunTask.taskId)
      setViewingWorkspaceId(lastRunTask.workspaceId)
    }
  }, [lastRunTask])

  // 返回选择界面
  const handleBackToSelect = useCallback(() => {
    setViewingTaskId(null)
    setViewingWorkspaceId(null)
  }, [])

  const handleRun = useCallback(async () => {
    if (!selected) return
    setSubmitting(true)
    setSubmitError(null)
    try {
      const fileList = await listFiles(ctx)
      const fileContents = await Promise.all(
        fileList.map(async (f) => {
          const file = await readFile(ctx, f.path)
          let b64: string
          if (file.content_b64) {
            b64 = file.content_b64
          } else {
            b64 = btoa(unescape(encodeURIComponent(file.content ?? '')))
          }
          return { path: file.path, content_b64: b64 }
        }),
      )
      if (fileContents.length === 0) {
        setSubmitError('草稿为空,无文件可 run')
        setSubmitting(false)
        return
      }
      const resp = (await runPlanWithDraft({
        workspace_id: selected,
        external_files: fileContents,
      })) as { task?: { id?: number | string }; task_id?: number | string; id?: number | string }
      const tid = Number(resp.task?.id ?? resp.task_id ?? resp.id)
      if (tid) {
        setViewingTaskId(tid)
        setViewingWorkspaceId(selected)
        onRunTaskCreated(tid, selected)
        message.success(`已提交 plan-only 任务 #${tid}`)
      } else {
        setSubmitError('提交成功但未返回 task_id,请前往 workspace 查看')
      }
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      setSubmitError(`Run 失败: ${msg ?? '未知错误'}`)
    } finally {
      setSubmitting(false)
    }
  }, [selected, ctx, onRunTaskCreated])

  if (!open) return null

  return (
    <div style={panelWidth ? { ...chatPanelStyle, width: panelWidth } : chatPanelStyle}>
      {/* 顶栏 */}
      <div style={chatHeaderStyle}>
        <span style={{ color: '#cccccc', fontWeight: 600 }}>Run</span>
        <span style={chatHeaderUnderline} />
        <div style={{ flex: 1 }} />
        <i
          className="codicon codicon-close"
          title="关闭"
          style={chatHeaderIcon}
          onClick={handleClose}
        />
      </div>

      {/* 内容区 */}
      <div style={{ ...chatBodyStyle, display: 'flex', flexDirection: 'column' }}>
        {/* 阶段 1: 选择 workspace + 提交 */}
        {!viewingTaskId && (
          <>
            <div style={hintStyle}>
              <i className="codicon codicon-info" />
              <span>Plan-only 模式,仅检测草稿不会变更云端</span>
            </div>

            {/* 上次运行结果入口 */}
            {lastRunTask && (
              <div style={lastRunBannerStyle}>
                <i className="codicon codicon-history" />
                <span style={{ flex: 1 }}>上次运行: Task #{lastRunTask.taskId}</span>
                <span
                  style={{ ...taskLinkStyle, marginTop: 0 }}
                  onClick={handleViewLast}
                >
                  查看日志 →
                </span>
              </div>
            )}

            {!loading && targets.length === 0 ? (
              <div style={warnBoxStyle}>
                <i className="codicon codicon-warning" />
                <span>本 manifest 没有任何 active deployment,请先发布版本并部署</span>
              </div>
            ) : (
              <div style={formGroupStyle}>
                <label style={labelStyle}>目标 Workspace</label>
                <select
                  style={selectStyle}
                  value={selected ?? ''}
                  onChange={(e) => setSelected(e.target.value)}
                  disabled={loading || submitting}
                >
                  {targets.map((t) => {
                    const wsId = t.workspace.workspace_id || String(t.workspace.id)
                    return (
                      <option key={wsId} value={wsId}>
                        {t.workspace.name} ({wsId})
                      </option>
                    )
                  })}
                </select>
              </div>
            )}

            {submitError && <div style={errorStyle}>{submitError}</div>}

            <div style={{ marginTop: 'auto', paddingTop: 12 }}>
              <button
                style={!selected || loading || submitting || targets.length === 0 ? btnPrimaryDisabledStyle : btnPrimaryStyle}
                disabled={!selected || loading || submitting || targets.length === 0}
                onClick={() => void handleRun()}
              >
                {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
                <i className="codicon codicon-play" /> Run Plan-only
              </button>
            </div>
          </>
        )}

        {/* 阶段 2: 查看任务日志(WebSocket + HTTP 兜底) */}
        {viewingTaskId && viewingWorkspaceId && (
          <>
            <div style={{ marginBottom: 8, display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ fontSize: 12, color: '#999' }}>Task #{viewingTaskId}</span>
              <div style={{ flex: 1 }} />
              <span
                style={{ ...taskLinkStyle, marginTop: 0 }}
                onClick={handleBackToSelect}
                title="返回新建运行"
              >
                <i className="codicon codicon-arrow-left" /> 新建
              </span>
              <span
                style={{ ...taskLinkStyle, marginTop: 0 }}
                onClick={() => navigate(`/workspaces/${viewingWorkspaceId}/tasks/${viewingTaskId}`)}
                title="跳转到 workspace 任务详情"
              >
                <i className="codicon codicon-external-link" /> 详情
              </span>
            </div>
            <TaskOutputLog taskId={viewingTaskId} workspaceId={viewingWorkspaceId} />
            <div style={{ marginTop: 8, display: 'flex', gap: 8 }}>
              <button style={btnSecondaryStyle} onClick={handleBackToSelect}>
                <i className="codicon codicon-arrow-left" /> 返回
              </button>
              <div style={{ flex: 1 }} />
              <button style={btnPrimaryStyle} onClick={handleClose}>
                关闭
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
