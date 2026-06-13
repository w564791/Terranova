/**
 * 部署到 Workspace — 右侧停靠面板(VS Code 暗色主题,与 AI 生成面板布局一致)
 *
 * 三态合一:
 *  1. 选定 workspace 未装 manifest → install(可选 workdir 执行子目录)
 *  2. 选定 workspace 已装本 manifest (active) → upgrade / uninstall 二选一
 *  3. 选定 workspace 装了别的 manifest → 后端校验拒绝
 *
 * 接后端:
 *   POST /manifests/:id/v2/deployments/install
 *   POST /manifests/:id/v2/deployments/:id/upgrade
 *   POST /manifests/:id/v2/deployments/:id/uninstall
 *   GET  /manifests/:id/v2/versions/:id/workdirs   (列可用 workdir 目录)
 */
import { useEffect, useMemo, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { message } from 'antd'
import {
  listVersions,
  listDeployments,
  listVersionWorkdirs,
  getDeploymentVarsets,
  installDeployment,
  upgradeDeployment,
  uninstallDeployment,
  triggerWorkspacePlanApply,
  type ManifestEditorContext,
  type ManifestVersion,
  type ManifestDeployment,
  type DeploymentVarsetEntry,
} from './manifestApi'
import { workspaceService, type Workspace } from '../../../services/workspaces'
import { variableSetService, type VariableSet } from '../../../services/variableSets'
import {
  chatPanelStyle,
  chatHeaderStyle,
  chatHeaderUnderline,
  chatHeaderIcon,
  chatBodyStyle,
  errorStyle,
} from './manifestAiStyles'

interface Props {
  ctx: ManifestEditorContext
  onClose: () => void
  onDeployed?: () => void
  panelWidth?: number
}

// ===== 内联样式(VS Code 暗色主题)=====

const formGroupStyle: React.CSSProperties = {
  marginBottom: 14,
}

const labelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: 12,
  color: '#999',
  marginBottom: 4,
}

const labelHintStyle: React.CSSProperties = {
  color: '#666',
  fontWeight: 400,
  fontSize: 11,
  marginLeft: 4,
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

const btnBaseStyle: React.CSSProperties = {
  padding: '4px 14px',
  border: '1px solid transparent',
  borderRadius: 2,
  cursor: 'pointer',
  fontSize: 13,
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  boxSizing: 'border-box',
  lineHeight: '20px',
  fontFamily: 'inherit',
}

const btnPrimaryStyle: React.CSSProperties = {
  ...btnBaseStyle,
  background: '#0e639c',
  color: '#fff',
}

const btnSecondaryStyle: React.CSSProperties = {
  ...btnBaseStyle,
  background: '#3a3a3a',
  color: '#cccccc',
}

const btnDangerStyle: React.CSSProperties = {
  ...btnBaseStyle,
  background: '#5a1d1d',
  color: '#f14c4c',
  borderColor: '#f14c4c',
}

const footerStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 8,
  paddingTop: 12,
  borderTop: '1px solid #2d2d2d',
  marginTop: 'auto',
}

const tagStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  padding: '1px 8px',
  borderRadius: 3,
  background: '#2d2d2d',
  border: '1px solid #3c3c3c',
  fontSize: 11,
  color: '#cccccc',
  margin: '0 4px 4px 0',
}

const tagRequiredStyle: React.CSSProperties = {
  ...tagStyle,
  borderColor: '#f14c4c',
  color: '#f14c4c',
}

const tagBlueStyle: React.CSSProperties = {
  ...tagStyle,
  borderColor: '#3794ff',
  color: '#3794ff',
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

const varBoxStyle: React.CSSProperties = {
  marginTop: 6,
  padding: '8px 10px',
  background: '#1b1b1b',
  border: '1px solid #2d2d2d',
  borderRadius: 4,
}

const varLabelStyle: React.CSSProperties = {
  fontSize: 11,
  color: '#666',
  marginBottom: 6,
}

const multiSelectWrapStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 4,
  padding: '6px 8px',
  background: '#3c3c3c',
  border: '1px solid #454545',
  borderRadius: 2,
  minHeight: 30,
  boxSizing: 'border-box',
}

const multiSelectChipStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  padding: '1px 8px',
  background: '#2d2d2d',
  border: '1px solid #3c3c3c',
  borderRadius: 3,
  fontSize: 12,
  color: '#cccccc',
}

const multiSelectDropdownStyle: React.CSSProperties = {
  marginTop: 2,
  background: '#252526',
  border: '1px solid #454545',
  borderRadius: 2,
  maxHeight: 160,
  overflow: 'auto',
  zIndex: 70,
}

const multiSelectItemStyle: React.CSSProperties = {
  padding: '4px 10px',
  fontSize: 12,
  color: '#cccccc',
  cursor: 'pointer',
  display: 'flex',
  alignItems: 'center',
  gap: 6,
}

const uninstallConfirmStyle: React.CSSProperties = {
  padding: '10px 12px',
  background: 'rgba(241,76,76,0.1)',
  border: '1px solid rgba(241,76,76,0.3)',
  borderRadius: 4,
  color: '#f14c4c',
  fontSize: 12,
  marginBottom: 12,
}

// ===== 组件 =====

export default function DeployPanel({ ctx, onClose, onDeployed, panelWidth }: Props) {
  const navigate = useNavigate()
  const [versions, setVersions] = useState<ManifestVersion[]>([])
  const [deployments, setDeployments] = useState<ManifestDeployment[]>([])
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [varsets, setVarsets] = useState<VariableSet[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const [versionId, setVersionId] = useState<string | undefined>()
  const [workspaceId, setWorkspaceId] = useState<string | undefined>()
  const [varsetIds, setVarsetIds] = useState<string[]>([])
  const [currentVarsetIds, setCurrentVarsetIds] = useState<string[]>([])
  const [workdir, setWorkdir] = useState<string>('')
  const [workdirs, setWorkdirs] = useState<string[]>([''])
  const [workdirsLoading, setWorkdirsLoading] = useState(false)
  const [varsetDropdownOpen, setVarsetDropdownOpen] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [confirmingUninstall, setConfirmingUninstall] = useState(false)

  // 加载数据
  useEffect(() => {
    setLoading(true)
    Promise.all([
      listVersions(ctx).catch(() => []),
      listDeployments(ctx).catch(() => []),
      workspaceService
        .getWorkspaces()
        .then((r) => {
          const d: any = (r as any)?.data
          return Array.isArray(d?.items) ? d.items : Array.isArray(d) ? d : []
        })
        .catch(() => []),
      variableSetService.list().then((r) => r.items ?? []).catch(() => []),
    ])
      .then(([v, d, w, vs]) => {
        setVersions(v)
        setDeployments(d)
        setWorkspaces(w)
        setVarsets(vs)
        if (v.length > 0) setVersionId(v[0].id)
      })
      .finally(() => setLoading(false))
  }, [ctx])

  const activeDeploymentForWs = useMemo<ManifestDeployment | undefined>(() => {
    if (!workspaceId) return undefined
    return deployments.find((d) => d.workspace_id === workspaceId && d.status === 'active')
  }, [workspaceId, deployments])

  const targetMode: 'install' | 'upgrade' = useMemo(() => {
    if (!workspaceId) return 'install'
    return activeDeploymentForWs ? 'upgrade' : 'install'
  }, [workspaceId, activeDeploymentForWs])

  // 选中已装 workspace 时:拉它当前关联的 varset,预填表单
  useEffect(() => {
    if (!activeDeploymentForWs) {
      setCurrentVarsetIds([])
      return
    }
    let cancelled = false
    getDeploymentVarsets(ctx, activeDeploymentForWs.id)
      .then((ids) => {
        if (cancelled) return
        setCurrentVarsetIds(ids)
        setVarsetIds(ids)
      })
      .catch(() => {
        if (!cancelled) setCurrentVarsetIds([])
      })
    return () => {
      cancelled = true
    }
  }, [ctx, activeDeploymentForWs])

  const sameVersion = useMemo(
    () => !!activeDeploymentForWs && activeDeploymentForWs.version_id === versionId,
    [activeDeploymentForWs, versionId],
  )
  const sameVarsets = useMemo(() => {
    if (varsetIds.length !== currentVarsetIds.length) return false
    return varsetIds.every((id, i) => id === currentVarsetIds[i])
  }, [varsetIds, currentVarsetIds])
  const noChange = sameVersion && sameVarsets

  // install 模式下:版本变更时拉该版本可用 workdir 目录
  useEffect(() => {
    if (!versionId || targetMode !== 'install') return
    let cancelled = false
    setWorkdirsLoading(true)
    listVersionWorkdirs(ctx, versionId)
      .then((dirs) => {
        if (cancelled) return
        setWorkdirs(dirs)
        const existing = workspaces.find(
          (w) => (w.workspace_id || String(w.id)) === workspaceId,
        )?.manifest_subpath
        setWorkdir(existing && dirs.includes(existing) ? existing : '')
      })
      .catch(() => {
        if (cancelled) return
        setWorkdirs([''])
        setWorkdir('')
      })
      .finally(() => {
        if (!cancelled) setWorkdirsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [ctx, versionId, targetMode, workspaceId, workspaces])

  const selectedVersionVars = useMemo(() => {
    const v = versions.find((x) => x.id === versionId)
    return Array.isArray(v?.variables) ? v!.variables : []
  }, [versions, versionId])

  const handleClose = useCallback(() => {
    setSubmitError(null)
    setConfirmingUninstall(false)
    onClose()
  }, [onClose])

  const doInstall = async (andRun: boolean): Promise<boolean> => {
    if (!versionId || !workspaceId) {
      setSubmitError('请选择版本与 workspace')
      return false
    }
    const vs: DeploymentVarsetEntry[] = varsetIds.map((id, i) => ({ varset_id: id, priority: i }))
    setSubmitting(true)
    setSubmitError(null)
    try {
      await installDeployment(ctx, {
        version_id: versionId,
        workspace_id: workspaceId,
        varsets: vs,
        workdir,
      })
      if (andRun) {
        const taskId = await triggerWorkspacePlanApply(workspaceId)
        message.success('已部署并触发 Plan+Apply,跳转任务页查看')
        onDeployed?.()
        handleClose()
        navigate(taskId ? `/workspaces/${workspaceId}/tasks/${taskId}` : `/workspaces/${workspaceId}`)
      } else {
        message.success('已 install,请到 workspace 跑 Plan+Apply 落地云端')
        onDeployed?.()
        handleClose()
      }
      return true
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      setSubmitError(`${andRun ? '部署并运行' : 'Install'} 失败: ${msg ?? '未知错误'}`)
      return false
    } finally {
      setSubmitting(false)
    }
  }

  const doUpgrade = async (andRun: boolean) => {
    if (!versionId || !activeDeploymentForWs) return
    const wsId = activeDeploymentForWs.workspace_id
    const vs: DeploymentVarsetEntry[] = varsetIds.map((id, i) => ({ varset_id: id, priority: i }))
    setSubmitting(true)
    setSubmitError(null)
    try {
      await upgradeDeployment(ctx, activeDeploymentForWs.id, {
        target_version_id: versionId,
        varsets: vs,
      })
      if (andRun) {
        const taskId = await triggerWorkspacePlanApply(wsId)
        message.success('已更新并触发 Plan+Apply,跳转任务页查看')
        onDeployed?.()
        handleClose()
        navigate(taskId ? `/workspaces/${wsId}/tasks/${taskId}` : `/workspaces/${wsId}`)
      } else {
        message.success('已更新,请到 workspace 跑 Plan+Apply')
        onDeployed?.()
        handleClose()
      }
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      setSubmitError(`${andRun ? '更新并运行' : '更新'} 失败: ${msg ?? '未知错误'}`)
    } finally {
      setSubmitting(false)
    }
  }

  const handleRunOnly = async () => {
    if (!activeDeploymentForWs) return
    const wsId = activeDeploymentForWs.workspace_id
    setSubmitting(true)
    setSubmitError(null)
    try {
      const taskId = await triggerWorkspacePlanApply(wsId)
      message.success('已触发 Plan+Apply,跳转任务页查看')
      handleClose()
      navigate(taskId ? `/workspaces/${wsId}/tasks/${taskId}` : `/workspaces/${wsId}`)
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      setSubmitError(`运行失败: ${msg ?? '未知错误'}`)
    } finally {
      setSubmitting(false)
    }
  }

  const handleUninstall = async () => {
    if (!activeDeploymentForWs) return
    setSubmitting(true)
    setSubmitError(null)
    try {
      await uninstallDeployment(ctx, activeDeploymentForWs.id)
      message.success('已 uninstall,请到 workspace 跑 Plan+Apply 销毁残留资源')
      setConfirmingUninstall(false)
      onDeployed?.()
      handleClose()
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      setSubmitError(`Uninstall 失败: ${msg ?? '未知错误'}`)
    } finally {
      setSubmitting(false)
    }
  }

  // varset 多选切换
  const toggleVarset = (id: string) => {
    setVarsetIds((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      return [...prev, id]
    })
  }

  // 底栏按钮
  const renderFooter = () => {
    if (loading) return null
    if (versions.length === 0) {
      return (
        <button style={btnSecondaryStyle} onClick={handleClose}>
          关闭 — 请先发布至少一个版本
        </button>
      )
    }
    if (targetMode === 'upgrade' && activeDeploymentForWs) {
      if (confirmingUninstall) {
        return (
          <>
            <div style={uninstallConfirmStyle}>
              确认解除关联?残留云端资源需到 workspace 跑 Plan+Apply 销毁。
            </div>
            <div style={footerStyle}>
              <button style={btnSecondaryStyle} onClick={() => setConfirmingUninstall(false)} disabled={submitting}>
                取消
              </button>
              <button style={btnDangerStyle} onClick={() => void handleUninstall()} disabled={submitting}>
                {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
                确认 Uninstall
              </button>
            </div>
          </>
        )
      }
      return (
        <div style={footerStyle}>
          <button style={btnSecondaryStyle} onClick={handleClose}>取消</button>
          <button style={btnDangerStyle} onClick={() => setConfirmingUninstall(true)} disabled={submitting}>
            卸载
          </button>
          {noChange ? (
            <button style={btnPrimaryStyle} onClick={() => void handleRunOnly()} disabled={submitting}>
              {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
              <i className="codicon codicon-play" /> 运行 (Plan+Apply)
            </button>
          ) : (
            <>
              <button style={btnSecondaryStyle} onClick={() => void doUpgrade(false)} disabled={submitting}>
                {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
                更新
              </button>
              <button style={btnPrimaryStyle} onClick={() => void doUpgrade(true)} disabled={submitting}>
                {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
                更新并运行
              </button>
            </>
          )}
        </div>
      )
    }
    return (
      <div style={footerStyle}>
        <button style={btnSecondaryStyle} onClick={handleClose}>取消</button>
        <button style={btnSecondaryStyle} onClick={() => void doInstall(false)} disabled={submitting}>
          {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
          Install
        </button>
        <button style={btnPrimaryStyle} onClick={() => void doInstall(true)} disabled={submitting}>
          {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
          部署并运行 (Plan+Apply)
        </button>
      </div>
    )
  }

  return (
    <div style={panelWidth ? { ...chatPanelStyle, width: panelWidth } : chatPanelStyle}>
      {/* 顶栏 */}
      <div style={chatHeaderStyle}>
        <span style={{ color: '#cccccc', fontWeight: 600 }}>部署到 Workspace</span>
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
        {versions.length === 0 && !loading && (
          <div style={warnBoxStyle}>
            <i className="codicon codicon-warning" />
            <span>还没有任何已发布版本,请先点击顶栏「发布版本」</span>
          </div>
        )}

        {/* 版本选择 */}
        <div style={formGroupStyle}>
          <label style={labelStyle}>选择版本</label>
          <select
            style={selectStyle}
            value={versionId ?? ''}
            onChange={(e) => setVersionId(e.target.value || undefined)}
            disabled={loading || versions.length === 0}
          >
            {versions.map((v) => (
              <option key={v.id} value={v.id}>{v.version}</option>
            ))}
          </select>
          {versionId && selectedVersionVars.length > 0 && (
            <div style={varBoxStyle}>
              <div style={varLabelStyle}>
                该版本声明的输入变量{' '}
                <span style={{ color: '#f14c4c' }}>(red = 必填)</span>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap' }}>
                {selectedVersionVars.map((v) => (
                  <span
                    key={v.name}
                    style={v.required ? tagRequiredStyle : tagStyle}
                    title={
                      (v.type_raw ? `type: ${v.type_raw}\n` : '') +
                      (v.default_raw ? `default: ${v.default_raw}\n` : '') +
                      (v.description ? `\n${v.description}` : '')
                    }
                  >
                    {v.name}
                    {v.sensitive && <span style={{ color: '#fa8c16', marginLeft: 4 }}>·敏感</span>}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Workspace 选择 */}
        <div style={formGroupStyle}>
          <label style={labelStyle}>目标 Workspace</label>
          <select
            style={selectStyle}
            value={workspaceId ?? ''}
            onChange={(e) => setWorkspaceId(e.target.value || undefined)}
            disabled={loading}
          >
            <option value="">选择 workspace</option>
            {workspaces.map((w) => {
              const wsId = w.workspace_id || String(w.id)
              return (
                <option key={wsId} value={wsId}>
                  {w.name} ({wsId})
                </option>
              )
            })}
          </select>
          {targetMode === 'upgrade' && activeDeploymentForWs && (
            <div style={{ marginTop: 6, display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={tagBlueStyle}>当前已装</span>
              <span style={{ color: '#888', fontSize: 12 }}>
                version:{' '}
                {versions.find((v) => v.id === activeDeploymentForWs.version_id)?.version ??
                  activeDeploymentForWs.version_id}
              </span>
            </div>
          )}
        </div>

        {/* 工作目录:仅 install 模式 */}
        {targetMode === 'install' && (
          <div style={formGroupStyle}>
            <label style={labelStyle}>
              工作目录 (workdir)
              <span style={labelHintStyle}>terraform 执行子目录,默认根 /</span>
            </label>
            <select
              style={selectStyle}
              value={workdir}
              onChange={(e) => setWorkdir(e.target.value)}
              disabled={loading || !versionId || workdirsLoading}
            >
              {workdirs.map((d) => (
                <option key={d} value={d}>{d === '' ? '/ (根目录)' : d}</option>
              ))}
            </select>
          </div>
        )}

        {/* Variable Sets 多选 */}
        <div style={formGroupStyle}>
          <label style={labelStyle}>
            关联 Variable Sets
            <span style={labelHintStyle}>(顺序即优先级,后选的优先级高)</span>
          </label>
          <div
            style={multiSelectWrapStyle}
            onClick={() => setVarsetDropdownOpen((v) => !v)}
          >
            {varsetIds.length === 0 ? (
              <span style={{ opacity: 0.5, fontSize: 12 }}>可不选 — 仅用 workspace 自有变量</span>
            ) : (
              varsetIds.map((id) => {
                const vs = varsets.find((v) => v.varset_id === id)
                return (
                  <span key={id} style={multiSelectChipStyle}>
                    {vs?.name ?? id}
                    <i
                      className="codicon codicon-close"
                      style={{ fontSize: 11, cursor: 'pointer', opacity: 0.7 }}
                      onClick={(e) => {
                        e.stopPropagation()
                        toggleVarset(id)
                      }}
                    />
                  </span>
                )
              })
            )}
          </div>
          {varsetDropdownOpen && (
            <div style={multiSelectDropdownStyle}>
              {varsets.length === 0 && (
                <div style={{ padding: '6px 10px', fontSize: 12, opacity: 0.5 }}>无可用 Variable Sets</div>
              )}
              {varsets.map((vs) => (
                <div
                  key={vs.varset_id}
                  style={{
                    ...multiSelectItemStyle,
                    background: varsetIds.includes(vs.varset_id) ? '#2a2d2e' : 'transparent',
                  }}
                  onClick={() => toggleVarset(vs.varset_id)}
                >
                  <i
                    className={`codicon ${varsetIds.includes(vs.varset_id) ? 'codicon-check' : 'codicon-blank'}`}
                    style={{ fontSize: 12, color: '#4ec9b0' }}
                  />
                  <span>{vs.name}</span>
                  <span style={{ opacity: 0.5, marginLeft: 'auto', fontSize: 11 }}>{vs.scope}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* 错误提示 */}
        {submitError && <div style={errorStyle}>{submitError}</div>}

        {/* 底栏按钮 */}
        {renderFooter()}
      </div>
    </div>
  )
}
