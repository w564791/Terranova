/**
 * 部署到 Workspace — 全宽面板(盖住编辑区,非弹窗)
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
 *
 * 仅在 ManifestEditorV2 内挂载;只在 open 时渲染,故数据在挂载时加载即可。
 */
import { useEffect, useMemo, useState } from 'react'
import { Form, Select, Tag, Alert, Button, Space, message } from 'antd'
import {
  listVersions,
  listDeployments,
  listVersionWorkdirs,
  installDeployment,
  upgradeDeployment,
  uninstallDeployment,
  type ManifestEditorContext,
  type ManifestVersion,
  type ManifestDeployment,
  type DeploymentVarsetEntry,
} from './manifestApi'
import { workspaceService, type Workspace } from '../../../services/workspaces'
import { variableSetService, type VariableSet } from '../../../services/variableSets'
import styles from './ManifestEditorV2.module.css'

interface Props {
  ctx: ManifestEditorContext
  onClose: () => void
  // 部署成功后回调(父组件可刷新 deployments 状态等)
  onDeployed?: () => void
}

export default function DeployPanel({ ctx, onClose, onDeployed }: Props) {
  const [versions, setVersions] = useState<ManifestVersion[]>([])
  const [deployments, setDeployments] = useState<ManifestDeployment[]>([])
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [varsets, setVarsets] = useState<VariableSet[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  // 表单状态
  const [versionId, setVersionId] = useState<string | undefined>()
  const [workspaceId, setWorkspaceId] = useState<string | undefined>()
  const [varsetIds, setVarsetIds] = useState<string[]>([])
  // install 的执行子目录(workdir):'' = 根。来自该版本可用目录列表。
  const [workdir, setWorkdir] = useState<string>('')
  const [workdirs, setWorkdirs] = useState<string[]>([''])
  const [workdirsLoading, setWorkdirsLoading] = useState(false)
  // uninstall 内联确认(antd v5 静态 Modal.confirm 在 React19 下静默失效)
  const [confirmingUninstall, setConfirmingUninstall] = useState(false)

  useEffect(() => {
    setLoading(true)
    Promise.all([
      listVersions(ctx).catch(() => []),
      listDeployments(ctx).catch(() => []),
      // /workspaces 响应是 {code, data:{items:[...]}};取 data.items,带数组兜底
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

  // install 模式下:版本变更时拉该版本可用 workdir 目录,默认选根 ''
  useEffect(() => {
    if (!versionId || targetMode !== 'install') return
    let cancelled = false
    setWorkdirsLoading(true)
    listVersionWorkdirs(ctx, versionId)
      .then((dirs) => {
        if (cancelled) return
        setWorkdirs(dirs)
        // 选中 workspace 已有 subpath 且在可用列表里 → 预填;否则默认根
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

  const reset = () => {
    setVersionId(undefined)
    setWorkspaceId(undefined)
    setVarsetIds([])
    setWorkdir('')
  }

  const handleInstall = async () => {
    if (!versionId || !workspaceId) {
      message.warning('请选择版本与 workspace')
      return
    }
    const varsets: DeploymentVarsetEntry[] = varsetIds.map((id, i) => ({ varset_id: id, priority: i }))
    setSubmitting(true)
    try {
      await installDeployment(ctx, {
        version_id: versionId,
        workspace_id: workspaceId,
        varsets,
        workdir, // '' = 根
      })
      message.success('已 install,请到 workspace 跑 Plan+Apply 落地云端')
      reset()
      onDeployed?.()
      onClose()
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      message.error(`Install 失败: ${msg ?? '未知错误'}`)
    } finally {
      setSubmitting(false)
    }
  }

  const handleUpgrade = async () => {
    if (!versionId || !activeDeploymentForWs) return
    const varsets: DeploymentVarsetEntry[] = varsetIds.map((id, i) => ({ varset_id: id, priority: i }))
    setSubmitting(true)
    try {
      await upgradeDeployment(ctx, activeDeploymentForWs.id, {
        target_version_id: versionId,
        varsets,
      })
      message.success('已 upgrade,请到 workspace 跑 Plan+Apply')
      reset()
      onDeployed?.()
      onClose()
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      message.error(`Upgrade 失败: ${msg ?? '未知错误'}`)
    } finally {
      setSubmitting(false)
    }
  }

  const handleUninstall = async () => {
    if (!activeDeploymentForWs) return
    setSubmitting(true)
    try {
      await uninstallDeployment(ctx, activeDeploymentForWs.id)
      message.success('已 uninstall,请到 workspace 跑 Plan+Apply 销毁残留资源')
      setConfirmingUninstall(false)
      onDeployed?.()
      onClose()
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      message.error(`Uninstall 失败: ${msg ?? '未知错误'}`)
    } finally {
      setSubmitting(false)
    }
  }

  const renderActions = () => {
    if (loading) return null
    if (versions.length === 0) {
      return <Button onClick={onClose}>关闭 — 请先发布至少一个版本</Button>
    }
    if (targetMode === 'upgrade' && activeDeploymentForWs) {
      if (confirmingUninstall) {
        return (
          <Space wrap>
            <span style={{ color: '#cf1322', marginRight: 4 }}>
              确认解除关联?残留云端资源需到 workspace 跑 Plan+Apply 销毁。
            </span>
            <Button onClick={() => setConfirmingUninstall(false)} disabled={submitting}>
              取消
            </Button>
            <Button danger type="primary" onClick={handleUninstall} loading={submitting}>
              确认 Uninstall
            </Button>
          </Space>
        )
      }
      return (
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button danger onClick={() => setConfirmingUninstall(true)} loading={submitting}>
            Uninstall
          </Button>
          <Button type="primary" onClick={handleUpgrade} loading={submitting}>
            Upgrade
          </Button>
        </Space>
      )
    }
    return (
      <Space>
        <Button onClick={onClose}>取消</Button>
        <Button type="primary" onClick={handleInstall} loading={submitting}>
          Install
        </Button>
      </Space>
    )
  }

  return (
    <div className={styles.deployPanel}>
      <div className={styles.deployPanelHeader}>
        <span>部署到 Workspace</span>
        <i className="codicon codicon-close" title="关闭" onClick={onClose} />
      </div>

      <div className={styles.deployPanelBody}>
        {versions.length === 0 && !loading && (
          <Alert
            type="warning"
            showIcon
            message="还没有任何已发布版本"
            description="请先点击 顶栏「发布版本」 把当前草稿固化为 vX.Y.Z,然后才能部署。"
            style={{ marginBottom: 12 }}
          />
        )}

        <Form layout="vertical" style={{ maxWidth: 640 }}>
          <Form.Item label="选择版本">
            <Select
              value={versionId}
              onChange={setVersionId}
              options={versions.map((v) => ({ value: v.id, label: v.version }))}
              placeholder="vX.Y.Z"
              disabled={loading || versions.length === 0}
            />
            {versionId && selectedVersionVars.length > 0 && (
              <div
                style={{
                  marginTop: 8,
                  padding: '8px 10px',
                  background: '#fafafa',
                  border: '1px solid #f0f0f0',
                  borderRadius: 4,
                }}
              >
                <div style={{ fontSize: 12, color: '#888', marginBottom: 6 }}>
                  该版本声明的输入变量{' '}
                  <span style={{ color: '#aaa' }}>(red = 必填,需由 workspace/varset 提供值)</span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {selectedVersionVars.map((v) => (
                    <Tag
                      key={v.name}
                      color={v.required ? 'red' : 'default'}
                      title={
                        (v.type_raw ? `type: ${v.type_raw}\n` : '') +
                        (v.default_raw ? `default: ${v.default_raw}\n` : '') +
                        (v.description ? `\n${v.description}` : '')
                      }
                      style={{ margin: 0 }}
                    >
                      {v.name}
                      {v.sensitive && <span style={{ color: '#fa8c16', marginLeft: 4 }}>·敏感</span>}
                    </Tag>
                  ))}
                </div>
              </div>
            )}
          </Form.Item>

          <Form.Item label="目标 Workspace">
            <Select
              value={workspaceId}
              onChange={setWorkspaceId}
              options={workspaces.map((w) => ({
                value: w.workspace_id || String(w.id),
                label: (
                  <span>
                    {w.name}{' '}
                    <span style={{ color: '#888', fontSize: 11 }}>{w.workspace_id || `id:${w.id}`}</span>
                  </span>
                ),
              }))}
              showSearch
              optionFilterProp="label"
              placeholder="选择 workspace"
              disabled={loading}
            />
            {targetMode === 'upgrade' && activeDeploymentForWs && (
              <div style={{ marginTop: 6 }}>
                <Tag color="blue">当前已装</Tag>
                <span style={{ color: '#888', fontSize: 12 }}>
                  version:{' '}
                  {versions.find((v) => v.id === activeDeploymentForWs.version_id)?.version ??
                    activeDeploymentForWs.version_id}
                </span>
              </div>
            )}
          </Form.Item>

          {/* 工作目录:仅 install 模式可选(upgrade 沿用已装目录,不可改) */}
          {targetMode === 'install' && (
            <Form.Item
              label={
                <span>
                  工作目录 (workdir){' '}
                  <span style={{ color: '#888', fontWeight: 400 }}>
                    terraform 执行子目录,默认根 /
                  </span>
                </span>
              }
            >
              <Select
                value={workdir}
                onChange={setWorkdir}
                loading={workdirsLoading}
                disabled={loading || !versionId}
                options={workdirs.map((d) => ({ value: d, label: d === '' ? '/ (根目录)' : d }))}
              />
            </Form.Item>
          )}

          <Form.Item
            label={
              <span>
                关联 Variable Sets{' '}
                <span style={{ color: '#888', fontWeight: 400 }}>(顺序即优先级,后选的优先级高)</span>
              </span>
            }
          >
            <Select
              mode="multiple"
              value={varsetIds}
              onChange={setVarsetIds}
              options={varsets.map((vs) => ({ value: vs.varset_id, label: `${vs.name} (${vs.scope})` }))}
              placeholder="可不选 — 仅用 workspace 自有变量"
              disabled={loading}
            />
          </Form.Item>
        </Form>
      </div>

      <div className={styles.deployPanelFooter}>{renderActions()}</div>
    </div>
  )
}
