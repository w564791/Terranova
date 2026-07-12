/**
 * 发布版本对话框 — VS Code 暗色主题风格
 *
 * 仅负责版本发布表单。检查流程由父组件管理,通过右侧面板展示。
 * 对话框打开时:若未检查 → 提示"请先检查";已检查 → 直接显示发布表单。
 *
 * 发布时可多选已部署的 workspace:选中的会自动 upgrade 到新版本并触发 Plan+Apply;
 * 未选中的不会更新 / trigger。
 *
 * 接 POST /manifests/:id/v2/versions
 */
import { useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  publishVersion,
  listVersions,
  listDeployments,
  getDeploymentUpgradeContext,
  upgradeDeployment,
  triggerWorkspacePlanApply,
  type ManifestEditorContext,
  type ManifestVersion,
  type ManifestDeployment,
} from './manifestApi'
import { workspaceService, type Workspace } from '../../../services/workspaces'
import type { ManifestIssue } from '../../../services/manifestAi'

export interface PublishCheckSummary {
  done: boolean
  skipped: boolean
  issues: ManifestIssue[]
}

interface Props {
  open: boolean
  ctx: ManifestEditorContext
  checkSummary: PublishCheckSummary | null
  onStartCheck: () => void
  onSkipCheck: () => void
  onClose: () => void
  /** 仅在发布 API 成功后调用;autoUpdateCount 为本次勾选并后台更新的 workspace 数 */
  onPublished?: (version: ManifestVersion, meta?: { autoUpdateCount: number }) => void
}

const SEMVER_RE = /^v\d+\.\d+\.\d+$/

// ===== VS Code 风格样式 =====

const overlayStyle: React.CSSProperties = {
  position: 'fixed',
  inset: 0,
  background: 'rgba(0,0,0,0.5)',
  zIndex: 1000,
  display: 'flex',
  alignItems: 'flex-start',
  justifyContent: 'center',
  paddingTop: '10vh',
}

const dialogStyle: React.CSSProperties = {
  width: 520,
  maxHeight: '80vh',
  display: 'flex',
  flexDirection: 'column',
  background: '#252526',
  border: '1px solid #454545',
  borderRadius: 0,
  boxShadow: '0 8px 30px rgba(0,0,0,0.5)',
  color: '#cccccc',
  fontSize: 13,
  fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
  userSelect: 'text',
}

const titleBarStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  padding: '10px 14px',
  borderBottom: '1px solid #454545',
  background: '#2d2d2d',
  fontSize: 14,
  fontWeight: 600,
  color: '#cccccc',
  flexShrink: 0,
}

const bodyStyle: React.CSSProperties = {
  padding: '12px 14px',
  overflowY: 'auto',
  flex: 1,
}

const footerStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: 8,
  padding: '8px 14px',
  borderTop: '1px solid #454545',
  background: '#2d2d2d',
  flexShrink: 0,
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
}

const btnLinkStyle: React.CSSProperties = {
  ...btnSecondaryStyle,
  padding: '2px 8px',
  fontSize: 12,
  background: 'transparent',
  color: '#3794ff',
}

const labelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: 12,
  color: '#999',
  marginBottom: 4,
}

const inputStyle: React.CSSProperties = {
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

const textAreaStyle: React.CSSProperties = {
  ...inputStyle,
  resize: 'vertical',
  minHeight: 60,
}

const sectionBoxStyle: React.CSSProperties = {
  padding: '10px 12px',
  background: 'rgba(255,255,255,0.04)',
  border: '1px solid rgba(255,255,255,0.1)',
  borderRadius: 2,
  marginBottom: 12,
}

const wsListStyle: React.CSSProperties = {
  maxHeight: 180,
  overflowY: 'auto',
  border: '1px solid #454545',
  borderRadius: 2,
  background: '#1e1e1e',
}

const wsRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '6px 10px',
  borderBottom: '1px solid #333',
  cursor: 'pointer',
  fontSize: 12,
}

const checkboxStyle: React.CSSProperties = {
  width: 14,
  height: 14,
  accentColor: '#0e639c',
  cursor: 'pointer',
  flexShrink: 0,
}

// ===== 辅助 =====

function suggestNextVersion(versions: ManifestVersion[]): string {
  const semvers = versions
    .map((v) => v.version)
    .filter((v) => SEMVER_RE.test(v))
    .map((v) => v.slice(1).split('.').map(Number) as [number, number, number])
  if (semvers.length === 0) return 'v1.0.0'
  semvers.sort((a, b) => b[0] - a[0] || b[1] - a[1] || b[2] - a[2])
  const [maj, min, patch] = semvers[0]
  return `v${maj}.${min}.${patch + 1}`
}

interface DeployedTarget {
  deployment: ManifestDeployment
  workspace: Workspace
  versionLabel: string
}

function workspaceKey(w: Workspace): string {
  return w.workspace_id || String(w.id)
}

// ===== 组件 =====

export default function PublishVersionDialog({
  open,
  ctx,
  checkSummary,
  onStartCheck,
  onSkipCheck,
  onClose,
  onPublished,
}: Props) {
  const { orgId, manifestId } = ctx
  const [version, setVersion] = useState('v1.0.0')
  const [changelog, setChangelog] = useState('')
  const [versionError, setVersionError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [progress, setProgress] = useState<string | null>(null)

  // 已部署 workspace 多选
  const [targets, setTargets] = useState<DeployedTarget[]>([])
  const [targetsLoading, setTargetsLoading] = useState(false)
  // 选中的 deployment id 集合(未选中的不会 update / trigger)
  const [selectedDepIds, setSelectedDepIds] = useState<Set<string>>(() => new Set())

  const publishReady = !!checkSummary?.done || !!checkSummary?.skipped

  useEffect(() => {
    if (!open) return
    setVersionError(null)
    setSubmitError(null)
    setProgress(null)
    setChangelog('')
    setSelectedDepIds(new Set())
    listVersions({ orgId, manifestId })
      .then((vs) => setVersion(suggestNextVersion(vs)))
      .catch(() => setVersion('v1.0.0'))
  }, [open, orgId, manifestId])

  // 加载 active deployments + workspace 名称 + 当前版本标签
  useEffect(() => {
    if (!open) return
    let cancelled = false
    setTargetsLoading(true)
    Promise.all([
      listDeployments(ctx).catch(() => [] as ManifestDeployment[]),
      listVersions(ctx).catch(() => [] as ManifestVersion[]),
      workspaceService
        .getWorkspaces()
        .then((r) => {
          const d: unknown = (r as { data?: unknown })?.data
          if (Array.isArray(d)) return d as Workspace[]
          if (d && typeof d === 'object' && Array.isArray((d as { items?: unknown }).items)) {
            return (d as { items: Workspace[] }).items
          }
          return [] as Workspace[]
        })
        .catch(() => [] as Workspace[]),
    ])
      .then(([deps, versions, workspaces]) => {
        if (cancelled) return
        const wsByID = new Map<string, Workspace>()
        for (const w of workspaces) {
          wsByID.set(workspaceKey(w), w)
        }
        const verByID = new Map(versions.map((v) => [v.id, v.version]))
        const list: DeployedTarget[] = deps
          .filter((d) => d.status === 'active')
          .map((d) => {
            const ws = wsByID.get(d.workspace_id)
            if (!ws) return null
            return {
              deployment: d,
              workspace: ws,
              versionLabel: verByID.get(d.version_id) ?? d.version_id,
            }
          })
          .filter((x): x is DeployedTarget => x !== null)
          .sort((a, b) => a.workspace.name.localeCompare(b.workspace.name))
        setTargets(list)
      })
      .finally(() => {
        if (!cancelled) setTargetsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, ctx])

  const allSelected = useMemo(
    () => targets.length > 0 && targets.every((t) => selectedDepIds.has(t.deployment.id)),
    [targets, selectedDepIds],
  )
  const selectedCount = selectedDepIds.size

  const toggleDep = (depId: string) => {
    setSelectedDepIds((prev) => {
      const next = new Set(prev)
      if (next.has(depId)) next.delete(depId)
      else next.add(depId)
      return next
    })
  }

  const selectAll = () => {
    setSelectedDepIds(new Set(targets.map((t) => t.deployment.id)))
  }

  const selectNone = () => {
    setSelectedDepIds(new Set())
  }

  const handleOk = async () => {
    if (!publishReady) return
    if (!SEMVER_RE.test(version)) {
      setVersionError('格式必须为 vMAJOR.MINOR.PATCH (如 v1.2.0)')
      return
    }
    setVersionError(null)
    setSubmitError(null)
    setProgress(null)
    try {
      setSubmitting(true)
      setProgress('正在发布版本…')
      const v = await publishVersion(ctx, { version, changelog })

      // 发布成功即收工:立刻关对话框并通知父组件展示提示;
      // workspace 更新/trigger 后台 best-effort,结果不影响发布
      const selected = targets.filter((t) => selectedDepIds.has(t.deployment.id))
      const publishedVersionId = v.id
      const publishedLabel = v.version || version

      if (selected.length > 0) {
        // 不阻塞关闭;失败静默,用户去任务页看实际结果
        void Promise.all(
          selected.map(async (t) => {
            const wsId = t.workspace.workspace_id || String(t.workspace.id)
            try {
              const upgradeCtx = await getDeploymentUpgradeContext(ctx, t.deployment.id)
              await upgradeDeployment(ctx, t.deployment.id, {
                target_version_id: publishedVersionId,
                varsets: upgradeCtx.varsets,
                variable_overrides: upgradeCtx.variable_overrides,
              })
              await triggerWorkspacePlanApply(
                wsId,
                `Manifest 发布 ${publishedLabel}: 自动更新并 Plan+Apply`,
              )
            } catch {
              // ignore
            }
          }),
        )
      }

      onPublished?.(v, { autoUpdateCount: selected.length })
      onClose()
    } catch (err) {
      // 仅发布本身失败才展示错误
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      if (msg) setSubmitError(`发布失败: ${msg}`)
    } finally {
      setSubmitting(false)
      setProgress(null)
    }
  }

  if (!open) return null

  return createPortal(
    <div style={overlayStyle}>
      <div style={dialogStyle}>
        {/* 标题栏 */}
        <div style={titleBarStyle}>
          <i className="codicon codicon-tag" style={{ marginRight: 8, opacity: 0.8 }} />
          发布新版本
          <div style={{ flex: 1 }} />
          <i
            className="codicon codicon-close"
            style={{
              cursor: submitting ? 'default' : 'pointer',
              opacity: submitting ? 0.4 : 0.7,
              padding: '0 4px',
            }}
            onClick={() => {
              if (!submitting) onClose()
            }}
          />
        </div>

        {/* 内容区 */}
        <div style={bodyStyle}>
          {/* 未检查:提示 */}
          {!publishReady && (
            <div style={sectionBoxStyle}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <i className="codicon codicon-info" style={{ color: '#3794ff' }} />
                <span style={{ flex: 1 }}>发布前建议对草稿执行一次检查</span>
                <button
                  style={btnSecondaryStyle}
                  onClick={onSkipCheck}
                >
                  跳过检查
                </button>
                <button
                  style={btnPrimaryStyle}
                  onClick={() => {
                    onClose()
                    onStartCheck()
                  }}
                >
                  <i className="codicon codicon-checklist" /> 开始检查
                </button>
              </div>
            </div>
          )}

          {/* 已检查摘要 */}
          {publishReady && checkSummary && (
            <div style={sectionBoxStyle}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
                {checkSummary.issues.length === 0 ? (
                  <>
                    <i className="codicon codicon-pass-filled" style={{ color: '#4ec9b0' }} />
                    <span style={{ color: '#4ec9b0' }}>检查通过,未发现问题</span>
                  </>
                ) : (
                  <>
                    <i className="codicon codicon-checklist" />
                    <span>检查完成,发现 {checkSummary.issues.length} 个问题</span>
                  </>
                )}
              </div>
            </div>
          )}

          {/* 发布表单 */}
          <div style={{ opacity: publishReady ? 1 : 0.4, pointerEvents: publishReady ? 'auto' : 'none' }}>
            <p style={{ color: '#999', margin: '0 0 10px', fontSize: 12 }}>
              把当前草稿快照成不可变版本,后续可被部署到 Workspace。
            </p>
            <div style={{ marginBottom: 10 }}>
              <label style={labelStyle}>版本号</label>
              <input
                style={{
                  ...inputStyle,
                  borderColor: versionError ? 'var(--red)' : '#454545',
                }}
                value={version}
                onChange={(e) => {
                  setVersion(e.target.value)
                  if (versionError) setVersionError(null)
                }}
                placeholder="v1.2.0"
                autoFocus
                disabled={submitting}
              />
              {versionError && <div style={{ color: 'var(--red)', fontSize: 12, marginTop: 2 }}>{versionError}</div>}
            </div>
            <div style={{ marginBottom: 14 }}>
              <label style={labelStyle}>发布说明 (可选)</label>
              <textarea
                style={textAreaStyle}
                value={changelog}
                onChange={(e) => setChangelog(e.target.value)}
                rows={3}
                placeholder="例如: 增加 NAT Gateway 配置"
                disabled={submitting}
              />
            </div>

            {/* 已部署 workspace 多选:自动更新并 trigger */}
            <div style={{ marginBottom: 4 }}>
              <div style={{ display: 'flex', alignItems: 'center', marginBottom: 4 }}>
                <label style={{ ...labelStyle, marginBottom: 0, flex: 1 }}>
                  自动更新并触发已部署 Workspace
                  {selectedCount > 0 && (
                    <span style={{ color: '#3794ff', marginLeft: 6 }}>
                      (已选 {selectedCount})
                    </span>
                  )}
                </label>
                {targets.length > 0 && (
                  <div style={{ display: 'flex', gap: 4 }}>
                    <button
                      type="button"
                      style={btnLinkStyle}
                      onClick={allSelected ? selectNone : selectAll}
                      disabled={submitting}
                    >
                      {allSelected ? '全不选' : '全选'}
                    </button>
                  </div>
                )}
              </div>
              <p style={{ color: '#666', margin: '0 0 6px', fontSize: 11, lineHeight: 1.4 }}>
                勾选的 workspace 会升级到新版本并触发 Plan+Apply;未勾选的不会更新或触发。
              </p>
              {targetsLoading ? (
                <div style={{ ...sectionBoxStyle, marginBottom: 0, color: '#999', fontSize: 12 }}>
                  <i className="codicon codicon-loading codicon-modifier-spin" style={{ marginRight: 6 }} />
                  加载已部署 workspace…
                </div>
              ) : targets.length === 0 ? (
                <div style={{ ...sectionBoxStyle, marginBottom: 0, color: '#666', fontSize: 12 }}>
                  当前没有已部署的 workspace
                </div>
              ) : (
                <div style={wsListStyle}>
                  {targets.map((t) => {
                    const depId = t.deployment.id
                    const wsId = t.workspace.workspace_id || String(t.workspace.id)
                    const checked = selectedDepIds.has(depId)
                    return (
                      <label
                        key={depId}
                        style={{
                          ...wsRowStyle,
                          background: checked ? 'rgba(14,99,156,0.18)' : 'transparent',
                        }}
                      >
                        <input
                          type="checkbox"
                          style={checkboxStyle}
                          checked={checked}
                          disabled={submitting}
                          onChange={() => toggleDep(depId)}
                        />
                        <span style={{ flex: 1, minWidth: 0 }}>
                          <span style={{ color: '#cccccc', fontWeight: 500 }}>{t.workspace.name}</span>
                          <span style={{ color: '#666', marginLeft: 6 }}>{wsId}</span>
                        </span>
                        <span style={{ color: '#999', flexShrink: 0, fontFamily: 'monospace' }}>
                          {t.versionLabel}
                        </span>
                      </label>
                    )
                  })}
                </div>
              )}
            </div>

            {progress && (
              <div style={{ marginTop: 10, color: '#3794ff', fontSize: 12, display: 'flex', alignItems: 'center', gap: 6 }}>
                <i className="codicon codicon-loading codicon-modifier-spin" />
                {progress}
              </div>
            )}
            {submitError && (
              <div style={{ marginTop: 8, color: 'var(--red)', fontSize: 12, whiteSpace: 'pre-wrap' }}>
                {submitError}
              </div>
            )}
          </div>
        </div>

        {/* 底栏按钮 */}
        <div style={footerStyle}>
          <button style={btnSecondaryStyle} onClick={onClose} disabled={submitting}>取消</button>
          <button
            style={!publishReady || submitting ? btnPrimaryDisabledStyle : btnPrimaryStyle}
            disabled={!publishReady || submitting}
            onClick={() => void handleOk()}
          >
            {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
            <i className="codicon codicon-tag" />
            {selectedCount > 0
              ? `发布并更新 ${selectedCount} 个 Workspace`
              : '发布'}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
