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
 *   POST /manifests/:id/v2/deployments/:id/variable-preview (upgrade 时预览合并变量,敏感值不回显)
 *   POST /manifests/:id/v2/deployments/variable-preview     (首次 install 前预览,同形响应)
 *
 * 变量覆盖:预览每行可填覆盖值,只提交用户改过的 key(敏感行未输入新值则不提交);
 * upgrade 时已有覆盖可"移除覆盖"(进 unset_keys,提交前可撤销),未动的 key 一律不发送。
 * 已存覆盖来自 deployment.overrides 脱敏视图:有 value 才预填;无 value 显示"已设置"、输入框为空。
 * 响应里的 sensitive 只用于展示(锁定为敏感),绝不回传;只有用户输入新值并勾选"敏感"才发
 * {value, sensitive: true}。
 *
 * 变量写权限:workspace 列表项 can_write_variables === false(缺省视为 true)时只能更换版本:
 * varset 不可改(install 发空列表;upgrade / upgrade 预览省略 varsets 字段 = 后端保持已存 varset;已存 varset 只读展示),
 * 覆盖输入 / 敏感 / 移除覆盖全部禁用,不发 variable_overrides / unset_keys。
 * upgrade 预览 403 => 显示"无变量查看权限",仍可只换版本。
 *
 * 不可变 bundle:bundle_hash 为空(或 hash_mismatch)的版本在版本下拉中禁用并标注"需重新发布";
 * install / 预览 / upgrade 返回 409 bundle_republish_required 时按 reason 提示
 * (hash_mismatch => "完整性校验失败，请重新发布",否则"该版本需要重新发布")并重拉版本
 * (列表只返回已存值,看似正常的版本也可能 409;该版本随后在本面板内禁用)。
 * 从此类版本升级走、卸载不受影响。
 *
 * 加载分两步(避免一打开就拉全量):
 *   1. 打开时只拉版本、部署记录、可写 workspace(GET /workspaces?capability=WORKSPACE_RESOURCES:WRITE)
 *   2. 选定 workspace 后才拉该 workspace 可挂载的 varset(GET /variable-sets?workspace_id=)与变量预览
 */
import { useEffect, useMemo, useState, useCallback } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Alert, Tag, message } from 'antd'
import {
  listVersions,
  listDeployments,
  listVersionWorkdirs,
  getDeploymentUpgradeContext,
  previewDeploymentVariables,
  previewInstallVariables,
  isManifestTargetNotFound,
  MANIFEST_TARGET_NOT_FOUND_MESSAGE,
  installDeployment,
  upgradeDeployment,
  uninstallDeployment,
  triggerWorkspacePlanApply,
  type ManifestEditorContext,
  type ManifestVersion,
  type ManifestDeployment,
  type DeploymentVarsetEntry,
  type DeploymentPreviewVariable,
  type DeploymentOverrideView,
  type OverrideInputs,
} from './manifestApi'
import BundleStatusTag from './BundleStatusTag'
import {
  REPUBLISH_REQUIRED_MESSAGE,
  republishRequiredMessage,
  bundleStatusLabel,
  isBundleRepublishRequired,
  versionNeedsRepublish,
  deploymentVersionStale,
  STALE_DEPLOYMENT_MESSAGE,
} from './bundleStatus'
import { workspaceService, type Workspace } from '../../../services/workspaces'
import { getHttpStatus } from '../../../services/api'
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
  /** 打开时预选的 workspace(如从部署列表"升级"失效部署进入) */
  initialWorkspaceId?: string
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
  color: 'var(--red)',
  borderColor: 'var(--red)',
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
  borderColor: 'var(--red)',
  color: 'var(--red)',
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
  color: 'var(--red)',
  fontSize: 12,
  marginBottom: 12,
}

const overrideInputStyle: React.CSSProperties = {
  ...selectStyle,
  padding: '2px 6px',
  fontSize: 12,
}

const rowActionStyle: React.CSSProperties = {
  marginLeft: 'auto',
  fontSize: 11,
  color: '#3794ff',
  cursor: 'pointer',
  flexShrink: 0,
}

const SENSITIVE_PLACEHOLDER = '敏感值，不回显'
const VALUE_SET_PLACEHOLDER = '已设置'

function errorText(err: unknown): string {
  if (isBundleRepublishRequired(err)) return republishRequiredMessage(err)
  if (isManifestTargetNotFound(err)) return MANIFEST_TARGET_NOT_FOUND_MESSAGE
  const msg = typeof err === 'string' ? err : (err as Error)?.message
  return msg ?? '未知错误'
}

// ===== 变量预览(install / upgrade 共用)=====

/** 预览行的展示状态(install / upgrade 共用,DeployPanel 计算) */
interface PreviewRow {
  key: string
  description?: string
  /** 输入框的初始值(未编辑时显示);值不可见时为空串 */
  baseline: string
  /** 值不回显:敏感,或已存覆盖未带 value */
  valueHidden: boolean
  /** 已是敏感(预览或已存覆盖标记),UI 锁定,不能取消 */
  lockedSensitive: boolean
  /** deployment 已存覆盖(仅 upgrade) */
  overridden: boolean
  /** 已存覆盖有值但未回显 */
  hasHiddenValue: boolean
}

interface VariablePreviewProps {
  rows: PreviewRow[]
  loading: boolean
  error: string | null
  /** 预览 403:无变量查看权限 */
  forbidden?: boolean
  /** 用户在本面板里输入的覆盖值(key -> 新值) */
  edits: Record<string, string>
  onEdit: (key: string, value: string) => void
  /** 本次被视为改动、将提交的 key */
  changedKeys: Set<string>
  /** 用户勾选"敏感"的 key(仅对改动且未锁定敏感的行生效) */
  sensitiveMarks: Set<string>
  onToggleSensitive: (key: string) => void
  /** 已标记"移除覆盖"的 key(仅 upgrade) */
  unsetKeys?: Set<string>
  onToggleUnset?: (key: string) => void
  disabled?: boolean
}

// 预览列表:敏感值后端恒为空串,只显示占位不回显;每行可输入覆盖值。
function VariablePreview({
  rows,
  loading,
  error,
  forbidden,
  edits,
  onEdit,
  changedKeys,
  sensitiveMarks,
  onToggleSensitive,
  unsetKeys,
  onToggleUnset,
  disabled,
}: VariablePreviewProps) {
  return (
    <div style={{ ...varBoxStyle, marginTop: 0 }}>
      {loading && <div style={varLabelStyle}>加载中...</div>}
      {!loading && forbidden && <div style={varLabelStyle}>无变量查看权限</div>}
      {!loading && !forbidden && error && (
        <div style={{ ...varLabelStyle, color: 'var(--red)' }}>预览失败: {error}</div>
      )}
      {!loading && !forbidden && !error && rows.length === 0 && <div style={varLabelStyle}>无变量</div>}
      {!loading &&
        !forbidden &&
        !error &&
        rows.map((v) => {
          const unset = !!unsetKeys?.has(v.key)
          const edited = Object.prototype.hasOwnProperty.call(edits, v.key)
          const changed = changedKeys.has(v.key)
          const markedSensitive = !v.lockedSensitive && changed && sensitiveMarks.has(v.key)
          const masked = v.lockedSensitive || markedSensitive
          const placeholder = v.lockedSensitive
            ? SENSITIVE_PLACEHOLDER
            : v.hasHiddenValue
              ? VALUE_SET_PLACEHOLDER
              : undefined
          return (
            <div key={v.key} style={{ fontSize: 12, padding: '3px 0' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                <span style={{ color: '#cccccc' }} title={v.description || undefined}>
                  {v.key}
                </span>
                {v.lockedSensitive && <span style={{ color: 'var(--amber)' }}>·敏感</span>}
                {v.overridden && <span style={{ color: '#3794ff' }}>·已覆盖</span>}
                {v.hasHiddenValue && !v.lockedSensitive && (
                  <span style={{ color: '#888' }}>·{VALUE_SET_PLACEHOLDER}</span>
                )}
                {!v.lockedSensitive && !unset && (
                  <label
                    style={{
                      marginLeft: v.overridden && onToggleUnset ? 8 : 'auto',
                      fontSize: 11,
                      color: changed ? '#cccccc' : '#666',
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 2,
                      cursor: changed && !disabled ? 'pointer' : 'not-allowed',
                    }}
                    title="仅在输入新值时生效;标记后不可改回普通"
                  >
                    <input
                      type="checkbox"
                      checked={markedSensitive}
                      disabled={disabled || !changed}
                      onChange={() => onToggleSensitive(v.key)}
                    />
                    敏感
                  </label>
                )}
                {v.overridden && onToggleUnset && (
                  <span
                    style={disabled ? { ...rowActionStyle, color: '#666', cursor: 'not-allowed' } : rowActionStyle}
                    onClick={() => {
                      if (!disabled) onToggleUnset(v.key)
                    }}
                  >
                    {unset ? '撤销' : '移除覆盖'}
                  </span>
                )}
              </div>
              {unset ? (
                <div style={{ color: 'var(--red)', fontStyle: 'italic', marginTop: 2 }}>
                  提交后移除覆盖,恢复为 workspace / Variable Set 中的值
                </div>
              ) : (
                <input
                  style={{ ...overrideInputStyle, marginTop: 2 }}
                  type={masked ? 'password' : 'text'}
                  autoComplete="new-password"
                  value={edited ? edits[v.key] : v.baseline}
                  placeholder={placeholder}
                  title={v.valueHidden ? placeholder : v.baseline}
                  disabled={disabled}
                  onChange={(e) => onEdit(v.key, e.target.value)}
                />
              )}
            </div>
          )
        })}
    </div>
  )
}

// ===== 组件 =====

export default function DeployPanel({ ctx, onClose, onDeployed, panelWidth, initialWorkspaceId }: Props) {
  const navigate = useNavigate()
  const [versions, setVersions] = useState<ManifestVersion[]>([])
  const [deployments, setDeployments] = useState<ManifestDeployment[]>([])
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [varsets, setVarsets] = useState<VariableSet[]>([])
  const [varsetsLoading, setVarsetsLoading] = useState(false)
  const [previewVars, setPreviewVars] = useState<DeploymentPreviewVariable[]>([])
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [previewForbidden, setPreviewForbidden] = useState(false)
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const [versionId, setVersionId] = useState<string | undefined>()
  const [workspaceId, setWorkspaceId] = useState<string | undefined>(initialWorkspaceId)
  const [varsetIds, setVarsetIds] = useState<string[]>([])
  const [currentVarsetIds, setCurrentVarsetIds] = useState<string[]>([])
  // deployment 已存 varset(含 priority,按加载顺序);无变量写权限时 upgrade 原样回传
  const [workdir, setWorkdir] = useState<string>('')
  const [workdirs, setWorkdirs] = useState<string[]>([''])
  const [workdirsLoading, setWorkdirsLoading] = useState(false)
  const [varsetDropdownOpen, setVarsetDropdownOpen] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [confirmingUninstall, setConfirmingUninstall] = useState(false)
  // 变量覆盖:existingOverrides=deployment 已存覆盖脱敏视图(仅展示,不回传;install 时为 [])
  const [existingOverrides, setExistingOverrides] = useState<DeploymentOverrideView[]>([])
  const [overrideEdits, setOverrideEdits] = useState<Record<string, string>>({})
  const [sensitiveMarks, setSensitiveMarks] = useState<Set<string>>(() => new Set())
  const [unsetKeys, setUnsetKeys] = useState<Set<string>>(() => new Set())
  // 部署路径上返回过 409 bundle_republish_required 的版本(列表里可能仍显示为正常)
  const [republishIds, setRepublishIds] = useState<Set<string>>(() => new Set())

  const isDeployable = useCallback(
    (v: ManifestVersion) => !versionNeedsRepublish(v) && !republishIds.has(v.id),
    [republishIds],
  )

  // 写入版本列表并修正选中项:选中的版本不存在或需重新发布时,改选第一个可部署的版本
  const applyVersions = useCallback((v: ManifestVersion[]) => {
    setVersions(v)
    setVersionId((cur) => {
      const keep = cur ? v.find((x) => x.id === cur) : undefined
      if (keep && !versionNeedsRepublish(keep)) return cur
      return v.find((x) => !versionNeedsRepublish(x))?.id
    })
  }, [])

  // 409 bundle_republish_required:记下该版本(本面板内禁用)并重拉版本列表
  const handleRepublishRequired = useCallback(
    (err: unknown, fallbackVersionId?: string) => {
      const data = (err as { data?: { version_id?: unknown } } | null)?.data
      const vid = typeof data?.version_id === 'string' && data.version_id ? data.version_id : fallbackVersionId
      if (vid) setRepublishIds((prev) => (prev.has(vid) ? prev : new Set(prev).add(vid)))
      listVersions(ctx)
        .then(applyVersions)
        .catch(() => {})
    },
    [ctx, applyVersions],
  )

  // 第一步:只拉版本 / 部署记录 / 可写 workspace(部署目标只列有 WORKSPACE_RESOURCES WRITE 的)
  useEffect(() => {
    setLoading(true)
    Promise.all([
      listVersions(ctx).catch(() => []),
      listDeployments(ctx).catch(() => []),
      workspaceService
        .getWorkspaces({ capability: 'WORKSPACE_RESOURCES:WRITE' })
        .then((r) => {
          const d: any = (r as any)?.data
          return Array.isArray(d?.items) ? d.items : Array.isArray(d) ? d : []
        })
        .catch(() => []),
    ])
      .then(([v, d, w]) => {
        applyVersions(v)
        setDeployments(d)
        setWorkspaces(w)
      })
      .finally(() => setLoading(false))
  }, [ctx, applyVersions])

  // 第二步:选定 workspace 后才拉它可挂载的 varset;切换 workspace 时重拉
  useEffect(() => {
    if (!workspaceId) {
      setVarsets([])
      return
    }
    let cancelled = false
    setVarsetsLoading(true)
    variableSetService
      .list(undefined, workspaceId)
      .then((r) => {
        if (!cancelled) setVarsets(r.items ?? [])
      })
      .catch(() => {
        if (!cancelled) setVarsets([])
      })
      .finally(() => {
        if (!cancelled) setVarsetsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [workspaceId])

  // 面板已打开时再次从部署列表点"升级":切到对应 workspace
  useEffect(() => {
    if (initialWorkspaceId) setWorkspaceId(initialWorkspaceId)
  }, [initialWorkspaceId])

  const activeDeploymentForWs = useMemo<ManifestDeployment | undefined>(() => {
    if (!workspaceId) return undefined
    return deployments.find((d) => d.workspace_id === workspaceId && d.status === 'active')
  }, [workspaceId, deployments])

  // 已装版本失效(bundle_hash 为空 / hash_mismatch):plan/apply/drift 都会失败,须升级到有效版本
  const installedStale = useMemo(
    () => deploymentVersionStale(activeDeploymentForWs, versions),
    [activeDeploymentForWs, versions],
  )

  // 无变量写权限(can_write_variables === false;字段缺省 = 旧后端,视为可写)
  const canWriteVariables = useMemo(() => {
    if (!workspaceId) return true
    const ws = workspaces.find((w) => (w.workspace_id || String(w.id)) === workspaceId)
    return ws?.can_write_variables !== false
  }, [workspaceId, workspaces])

  const targetMode: 'install' | 'upgrade' = useMemo(() => {
    if (!workspaceId) return 'install'
    return activeDeploymentForWs ? 'upgrade' : 'install'
  }, [workspaceId, activeDeploymentForWs])

  // 切换 workspace / 已装 deployment 时:清空本面板里的覆盖输入与移除标记
  useEffect(() => {
    setOverrideEdits({})
    setSensitiveMarks(new Set())
    setUnsetKeys(new Set())
  }, [workspaceId, activeDeploymentForWs])

  // 选中已装 workspace 时:拉它当前关联的 varset 与已存覆盖,预填表单
  useEffect(() => {
    if (!activeDeploymentForWs) {
      setCurrentVarsetIds([])
      setExistingOverrides([])
      return
    }
    let cancelled = false
    setExistingOverrides([])
    getDeploymentUpgradeContext(ctx, activeDeploymentForWs.id)
      .then((uc) => {
        if (cancelled) return
        setCurrentVarsetIds(uc.varsetIds)
        setVarsetIds(uc.varsetIds)
        setExistingOverrides(uc.overrides)
      })
      .catch(() => {
        if (cancelled) return
        setCurrentVarsetIds([])
        setExistingOverrides([])
      })
    return () => {
      cancelled = true
    }
  }, [ctx, activeDeploymentForWs])

  // 变量预览(install / upgrade 同一渲染组件),workspace / 版本 / varset 变化时重拉:
  //  - upgrade:per-deployment 预览;后端与 upgrade 同一 mergeDeploymentOverrides 合并已存覆盖,
  //    前端不回传已存值,只带 unset_keys 与 target_version_id(参与敏感判定)。
  //    本次输入的新值在本地叠加显示(不随每次按键重拉,也避免以预览值为基准判定改动时自我抵消),
  //    提交 upgrade 时只发这些改动的 key。
  //  - install:首装预览(需已选 workspace + 版本)。
  const unsetKeysForPreview = useMemo(
    () => (canWriteVariables ? Array.from(unsetKeys).sort() : []),
    [unsetKeys, canWriteVariables],
  )
  // 本次提交 / 预览用的 varset 列表:可写 = 所选顺序即优先级;
  // 无变量写权限 = install 恒空;upgrade / upgrade 预览为 undefined => 省略 varsets 字段(后端保持已存 varset)
  const varsetEntries = useMemo<DeploymentVarsetEntry[] | undefined>(() => {
    if (canWriteVariables) return varsetIds.map((id, i) => ({ varset_id: id, priority: i }))
    return activeDeploymentForWs ? undefined : []
  }, [canWriteVariables, varsetIds, activeDeploymentForWs])
  useEffect(() => {
    const varsets = varsetEntries
    let req: Promise<DeploymentPreviewVariable[]> | null = null
    if (activeDeploymentForWs) {
      req = previewDeploymentVariables(ctx, activeDeploymentForWs.id, {
        ...(versionId ? { target_version_id: versionId } : {}),
        ...(varsets !== undefined ? { varsets } : {}),
        ...(unsetKeysForPreview.length > 0 ? { unset_keys: unsetKeysForPreview } : {}),
      })
    } else if (workspaceId && versionId) {
      req = previewInstallVariables(ctx, {
        workspace_id: workspaceId,
        version_id: versionId,
        varsets: varsets ?? [],
      })
    }
    const isUpgradePreview = !!activeDeploymentForWs
    setPreviewForbidden(false)
    if (!req) {
      setPreviewVars([])
      setPreviewError(null)
      return
    }
    let cancelled = false
    setPreviewLoading(true)
    setPreviewError(null)
    req
      .then((vars) => {
        if (!cancelled) setPreviewVars(vars)
      })
      .catch((err) => {
        if (cancelled) return
        setPreviewVars([])
        // upgrade 预览 403:无变量查看权限,仍允许只换版本
        if (isUpgradePreview && getHttpStatus(err) === 403) {
          setPreviewForbidden(true)
          return
        }
        if (isBundleRepublishRequired(err)) {
          // upgrade 预览未带 target 时预览的是当前版本
          handleRepublishRequired(err, versionId ?? activeDeploymentForWs?.version_id)
        }
        setPreviewError(errorText(err))
      })
      .finally(() => {
        if (!cancelled) setPreviewLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [ctx, activeDeploymentForWs, unsetKeysForPreview, workspaceId, versionId, varsetEntries, handleRepublishRequired])

  const overrideByKey = useMemo(
    () => new Map(existingOverrides.map((o) => [o.key, o])),
    [existingOverrides],
  )
  const overriddenKeys = useMemo(() => new Set(overrideByKey.keys()), [overrideByKey])

  // 预览行 = 预览变量 + 预览里没有的已存覆盖 key(仍可移除 / 改值)。
  // 已存覆盖:有 value(非敏感且有读权限)才预填,否则输入框为空并提示"已设置"。
  const previewRows = useMemo<PreviewRow[]>(() => {
    const isUpgrade = !!activeDeploymentForWs
    const toRow = (key: string, v?: DeploymentPreviewVariable): PreviewRow => {
      const ov = isUpgrade ? overrideByKey.get(key) : undefined
      const lockedSensitive = !!v?.sensitive || !!ov?.sensitive
      const ovHidden = !!ov && ov.value === undefined
      const valueHidden = lockedSensitive || ovHidden
      const baseline = valueHidden ? '' : ov ? (ov.value ?? '') : (v?.value ?? '')
      return {
        key,
        description: v?.description,
        baseline,
        valueHidden,
        lockedSensitive,
        overridden: !!ov,
        hasHiddenValue: !!ov && ov.has_value && ov.value === undefined,
      }
    }
    const rows = previewVars.map((v) => toRow(v.key, v))
    if (isUpgrade) {
      const seen = new Set(previewVars.map((v) => v.key))
      for (const o of existingOverrides) if (!seen.has(o.key)) rows.push(toRow(o.key))
    }
    return rows
  }, [previewVars, existingOverrides, overrideByKey, activeDeploymentForWs])

  // 只提交用户改过的 key:值不回显的行只有输入了非空新值才提交(空 = 未修改);
  // 其余行与初始值不同才提交;被标记移除覆盖的 key 不提交值。
  // 响应里的 sensitive 不回传;只有新值 + 用户勾选"敏感"(且该行未锁定敏感)才发 {value, sensitive: true}。
  const changedOverrides = useMemo(() => {
    const out: OverrideInputs = {}
    if (!canWriteVariables) return out // 无变量写权限:永不发送覆盖
    const byKey = new Map(previewRows.map((r) => [r.key, r]))
    for (const [k, val] of Object.entries(overrideEdits)) {
      if (unsetKeys.has(k)) continue
      const row = byKey.get(k)
      if (!row) continue
      const changed = row.valueHidden ? val !== '' : val !== row.baseline
      if (!changed) continue
      out[k] = !row.lockedSensitive && sensitiveMarks.has(k) ? { value: val, sensitive: true } : val
    }
    return out
  }, [overrideEdits, unsetKeys, previewRows, sensitiveMarks, canWriteVariables])
  const changedKeys = useMemo(() => new Set(Object.keys(changedOverrides)), [changedOverrides])

  const unsetKeyList = useMemo(
    () => (canWriteVariables ? Array.from(unsetKeys).filter((k) => overriddenKeys.has(k)) : []),
    [unsetKeys, overriddenKeys, canWriteVariables],
  )

  const handleEditOverride = useCallback((key: string, value: string) => {
    setOverrideEdits((prev) => ({ ...prev, [key]: value }))
  }, [])

  const toggleSensitive = useCallback((key: string) => {
    setSensitiveMarks((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])

  const toggleUnset = useCallback((key: string) => {
    setUnsetKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])

  const sameVersion = useMemo(
    () => !!activeDeploymentForWs && activeDeploymentForWs.version_id === versionId,
    [activeDeploymentForWs, versionId],
  )
  const sameVarsets = useMemo(() => {
    if (!canWriteVariables) return true // 省略 varsets = 不变
    if (varsetIds.length !== currentVarsetIds.length) return false
    return varsetIds.every((id, i) => id === currentVarsetIds[i])
  }, [varsetIds, currentVarsetIds, canWriteVariables])
  const noChange =
    sameVersion && sameVarsets && Object.keys(changedOverrides).length === 0 && unsetKeyList.length === 0

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
    const vs = varsetEntries
    setSubmitting(true)
    setSubmitError(null)
    try {
      await installDeployment(ctx, {
        version_id: versionId,
        workspace_id: workspaceId,
        varsets: vs ?? [],
        ...(Object.keys(changedOverrides).length > 0 ? { variable_overrides: changedOverrides } : {}),
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
      if (isBundleRepublishRequired(err)) handleRepublishRequired(err, versionId)
      setSubmitError(`${andRun ? '部署并运行' : 'Install'} 失败: ${errorText(err)}`)
      return false
    } finally {
      setSubmitting(false)
    }
  }

  const doUpgrade = async (andRun: boolean) => {
    if (!versionId || !activeDeploymentForWs) return
    const wsId = activeDeploymentForWs.workspace_id
    const vs = varsetEntries
    setSubmitting(true)
    setSubmitError(null)
    try {
      await upgradeDeployment(ctx, activeDeploymentForWs.id, {
        target_version_id: versionId,
        ...(vs !== undefined ? { varsets: vs } : {}),
        // 后端合并覆盖:只发用户改过的 key + 明确移除的 key,未动的 key 不发送
        ...(Object.keys(changedOverrides).length > 0 ? { variable_overrides: changedOverrides } : {}),
        ...(unsetKeyList.length > 0 ? { unset_keys: unsetKeyList } : {}),
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
      if (isBundleRepublishRequired(err)) {
        handleRepublishRequired(err, versionId)
        setSubmitError(`${andRun ? '更新并运行' : '更新'} 失败: ${republishRequiredMessage(err)}`)
        return
      }
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
      message.success('已卸载：云上资源不会被删除；要删除资源，请到该 workspace 上运行 Plan + Apply', 6)
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
    if (!canWriteVariables) return
    setVarsetIds((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      return [...prev, id]
    })
  }

  // 选中的版本需重新发布(或尚未选中)=> install / upgrade 到它会 409,直接禁用
  const selectedVersion = useMemo(() => versions.find((v) => v.id === versionId), [versions, versionId])
  const selectedNotDeployable = !selectedVersion || !isDeployable(selectedVersion)
  const upgradeBlocked = submitting || selectedNotDeployable
  const installBlocked = submitting || selectedNotDeployable

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
              卸载只解除 Manifest 与 workspace 的绑定，云上资源不会被删除。要删除资源，请到该 workspace 上运行 Plan + Apply。
              <div style={{ marginTop: 6 }}>
                <Link
                  to={`/workspaces/${activeDeploymentForWs.workspace_id}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  style={{ color: '#3794ff' }}
                >
                  打开 workspace
                  {(() => {
                    const ws = workspaces.find(
                      (w) => (w.workspace_id || String(w.id)) === activeDeploymentForWs.workspace_id,
                    )
                    return ws?.name ? ` ${ws.name}` : ''
                  })()}{' '}
                  <i className="codicon codicon-link-external" style={{ fontSize: 11 }} />
                </Link>
              </div>
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
          {installedStale ? (
            // 已装版本失效:运行必失败,不提供"运行";升级为主操作
            <>
              <button style={btnSecondaryStyle} onClick={() => void doUpgrade(false)} disabled={upgradeBlocked || noChange}>
                {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
                仅升级
              </button>
              <button
                style={{ ...btnPrimaryStyle, fontWeight: 600 }}
                onClick={() => void doUpgrade(true)}
                disabled={upgradeBlocked || noChange}
              >
                {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
                <i className="codicon codicon-arrow-up" /> 升级到所选版本并运行
              </button>
            </>
          ) : noChange ? (
            <button style={btnPrimaryStyle} onClick={() => void handleRunOnly()} disabled={submitting}>
              {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
              <i className="codicon codicon-play" /> 运行 (Plan+Apply)
            </button>
          ) : (
            <>
              <button style={btnSecondaryStyle} onClick={() => void doUpgrade(false)} disabled={upgradeBlocked}>
                {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
                更新
              </button>
              <button style={btnPrimaryStyle} onClick={() => void doUpgrade(true)} disabled={upgradeBlocked}>
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
        <button style={btnSecondaryStyle} onClick={() => void doInstall(false)} disabled={installBlocked}>
          {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
          Install
        </button>
        <button style={btnPrimaryStyle} onClick={() => void doInstall(true)} disabled={installBlocked}>
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
            {versions.map((v) => {
              // 原生 option 放不下 Tag:后缀文案 + title 作为提示,并禁用(install / upgrade 目标都不可选)
              const status =
                bundleStatusLabel(v) ??
                (republishIds.has(v.id) ? { label: '需重新发布', tooltip: REPUBLISH_REQUIRED_MESSAGE } : null)
              return (
                <option key={v.id} value={v.id} disabled={!!status} title={status?.tooltip}>
                  {status ? `${v.version} (${status.label})` : v.version}
                </option>
              )
            })}
          </select>
          {selectedVersion && !isDeployable(selectedVersion) && (
            <div style={{ marginTop: 6, display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: '#cca700' }}>
              {versionNeedsRepublish(selectedVersion) ? (
                <BundleStatusTag version={selectedVersion} />
              ) : null}
              <span>{REPUBLISH_REQUIRED_MESSAGE}</span>
            </div>
          )}
          {!loading && versions.length > 0 && !versions.some(isDeployable) && (
            <div style={{ ...warnBoxStyle, marginTop: 6, marginBottom: 0 }}>
              <i className="codicon codicon-warning" />
              <span>所有已发布版本都需要重新发布,请先发布新版本</span>
            </div>
          )}
          {versionId && selectedVersionVars.length > 0 && (
            <div style={varBoxStyle}>
              <div style={varLabelStyle}>
                该版本声明的输入变量{' '}
                <span style={{ color: 'var(--red)' }}>(red = 必填)</span>
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
                    {v.sensitive && <span style={{ color: 'var(--amber)', marginLeft: 4 }}>·敏感</span>}
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
            onChange={(e) => {
              setWorkspaceId(e.target.value || undefined)
              // varset 按 workspace 收口:切换后清空已选(upgrade 会按 deployment 重新预填)
              setVarsetIds([])
              setVarsetDropdownOpen(false)
            }}
            disabled={loading || workspaces.length === 0}
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
          {!loading && workspaces.length === 0 && (
            <div style={{ ...warnBoxStyle, marginTop: 6, marginBottom: 0 }}>
              <i className="codicon codicon-warning" />
              <span>没有可部署的 workspace:需要目标 workspace 的 WORKSPACE_RESOURCES 写权限</span>
            </div>
          )}
          {targetMode === 'upgrade' && activeDeploymentForWs && (
            <div style={{ marginTop: 6, display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={tagBlueStyle}>当前已装</span>
              <span style={{ color: '#888', fontSize: 12 }}>
                version:{' '}
                {versions.find((v) => v.id === activeDeploymentForWs.version_id)?.version ??
                  activeDeploymentForWs.version_id}
              </span>
              {(() => {
                const cur = versions.find((v) => v.id === activeDeploymentForWs.version_id)
                return cur ? <BundleStatusTag version={cur} /> : null
              })()}
            </div>
          )}
          {targetMode === 'upgrade' && installedStale && (
            <Alert
              type="error"
              showIcon
              style={{ marginTop: 8, fontSize: 12 }}
              message={
                <span>
                  <Tag color="error" style={{ marginInlineEnd: 6 }}>已失效</Tag>
                  {STALE_DEPLOYMENT_MESSAGE}
                </span>
              }
            />
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

        {workspaceId && !canWriteVariables && (
          <Alert
            type="info"
            showIcon
            message="无变量写权限，只能更换版本"
            style={{ marginBottom: 12, padding: '4px 10px', fontSize: 12 }}
          />
        )}

        {/* Variable Sets 多选 */}
        <div style={formGroupStyle}>
          <label style={labelStyle}>
            关联 Variable Sets
            <span style={labelHintStyle}>(顺序即优先级,后选的优先级高)</span>
          </label>
          <div
            style={{
              ...multiSelectWrapStyle,
              cursor: workspaceId && canWriteVariables ? 'pointer' : 'not-allowed',
              ...(workspaceId && !canWriteVariables ? { opacity: 0.6 } : {}),
            }}
            onClick={() => {
              if (workspaceId && canWriteVariables) setVarsetDropdownOpen((v) => !v)
            }}
          >
            {!workspaceId ? (
              <span style={{ opacity: 0.5, fontSize: 12 }}>请先选择目标 workspace</span>
            ) : !canWriteVariables && targetMode === 'install' ? (
              <span style={{ opacity: 0.5, fontSize: 12 }}>无变量写权限,不关联 Variable Set</span>
            ) : varsetIds.length === 0 ? (
              <span style={{ opacity: 0.5, fontSize: 12 }}>可不选 — 仅用 workspace 自有变量</span>
            ) : (
              varsetIds.map((id) => {
                const vs = varsets.find((v) => v.varset_id === id)
                return (
                  <span key={id} style={multiSelectChipStyle}>
                    {vs?.name ?? id}
                    {canWriteVariables && (
                      <i
                        className="codicon codicon-close"
                        style={{ fontSize: 11, cursor: 'pointer', opacity: 0.7 }}
                        onClick={(e) => {
                          e.stopPropagation()
                          toggleVarset(id)
                        }}
                      />
                    )}
                  </span>
                )
              })
            )}
          </div>
          {varsetDropdownOpen && workspaceId && canWriteVariables && (
            <div style={multiSelectDropdownStyle}>
              {varsetsLoading && (
                <div style={{ padding: '6px 10px', fontSize: 12, opacity: 0.5 }}>加载中...</div>
              )}
              {!varsetsLoading && varsets.length === 0 && (
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

        {/* 变量预览:install(已选 workspace + 版本)与 upgrade 共用同一组件 */}
        {workspaceId && (targetMode === 'upgrade' ? !!activeDeploymentForWs : !!versionId) && (
          <div style={formGroupStyle}>
            <label style={labelStyle}>
              变量预览
              <span style={labelHintStyle}>(按所选 Variable Sets 合并后的最终值;可在此填写覆盖值)</span>
            </label>
            <VariablePreview
              rows={previewRows}
              loading={previewLoading}
              error={previewError}
              forbidden={previewForbidden}
              edits={overrideEdits}
              onEdit={handleEditOverride}
              changedKeys={changedKeys}
              sensitiveMarks={sensitiveMarks}
              onToggleSensitive={toggleSensitive}
              unsetKeys={targetMode === 'upgrade' ? unsetKeys : undefined}
              onToggleUnset={targetMode === 'upgrade' ? toggleUnset : undefined}
              disabled={submitting || !canWriteVariables}
            />
          </div>
        )}

        {/* 错误提示 */}
        {submitError && <div style={errorStyle}>{submitError}</div>}

        {/* 底栏按钮 */}
        {renderFooter()}
      </div>
    </div>
  )
}
