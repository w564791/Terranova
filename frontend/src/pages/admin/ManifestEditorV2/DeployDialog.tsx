/**
 * 部署到 Workspace 对话框
 *
 * 三态合一:
 *  1. 选定 workspace 未装 manifest → install
 *  2. 选定 workspace 已装本 manifest (active) → upgrade / uninstall 二选一
 *  3. 选定 workspace 装了别的 manifest → 提示禁止
 *
 * 接 PR1 后端:
 *   POST /manifests/:id/v2/deployments/install
 *   POST /manifests/:id/v2/deployments/:id/upgrade
 *   POST /manifests/:id/v2/deployments/:id/uninstall
 */
import { useEffect, useMemo, useState } from 'react'
import { Modal, Form, Select, Tag, Alert, Button, Space, message } from 'antd'
import {
  listVersions,
  listDeployments,
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

interface Props {
  open: boolean
  ctx: ManifestEditorContext
  onClose: () => void
}

export default function DeployDialog({ open, ctx, onClose }: Props) {
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

  useEffect(() => {
    if (!open) return
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
        // 默认: 最新已发布版本
        if (v.length > 0) setVersionId(v[0].id)
      })
      .finally(() => setLoading(false))
  }, [open, ctx])

  const activeDeploymentForWs = useMemo<ManifestDeployment | undefined>(() => {
    if (!workspaceId) return undefined
    return deployments.find(
      (d) => d.workspace_id === workspaceId && d.status === 'active',
    )
  }, [workspaceId, deployments])

  // 选中版本声明的 input variables(发布时静态解析得到),供用户对照该喂哪些变量
  const selectedVersionVars = useMemo(() => {
    const v = versions.find((x) => x.id === versionId)
    // variables 是后端 JSONB,理论上恒为数组,但防御性兜底避免 .map 崩
    return Array.isArray(v?.variables) ? v!.variables : []
  }, [versions, versionId])

  const targetMode: 'install' | 'upgrade' | 'foreign' = useMemo(() => {
    if (!workspaceId) return 'install'
    if (activeDeploymentForWs) return 'upgrade'
    // PR1 后端约束: workspace 必须为空才能 install,这里前端不知道是否有
    // workspace_resources / state,让后端校验,前端只看是否已装"本 manifest"
    return 'install'
  }, [workspaceId, activeDeploymentForWs])

  const reset = () => {
    setVersionId(undefined)
    setWorkspaceId(undefined)
    setVarsetIds([])
  }

  const handleInstall = async () => {
    if (!versionId || !workspaceId) {
      message.warning('请选择版本与 workspace')
      return
    }
    const varsets: DeploymentVarsetEntry[] = varsetIds.map((id, i) => ({
      varset_id: id,
      priority: i,
    }))
    setSubmitting(true)
    try {
      await installDeployment(ctx, {
        version_id: versionId,
        workspace_id: workspaceId,
        varsets,
      })
      message.success('已 install,请到 workspace 跑 Plan+Apply 落地云端')
      reset()
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
    const varsets: DeploymentVarsetEntry[] = varsetIds.map((id, i) => ({
      varset_id: id,
      priority: i,
    }))
    setSubmitting(true)
    try {
      await upgradeDeployment(ctx, activeDeploymentForWs.id, {
        target_version_id: versionId,
        varsets,
      })
      message.success('已 upgrade,请到 workspace 跑 Plan+Apply')
      reset()
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
    Modal.confirm({
      title: '确认 uninstall?',
      content: '解除 manifest 与 workspace 的关联。残留云端资源需要你之后到 workspace 跑 Plan+Apply 销毁。',
      okText: 'Uninstall',
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          await uninstallDeployment(ctx, activeDeploymentForWs.id)
          message.success('已 uninstall,请到 workspace 跑 Plan+Apply 销毁残留资源')
          onClose()
        } catch (err) {
          const msg = typeof err === 'string' ? err : (err as Error)?.message
          message.error(`Uninstall 失败: ${msg ?? '未知错误'}`)
        }
      },
    })
  }

  const renderFooter = () => {
    if (loading) return null
    if (versions.length === 0) {
      return (
        <Button onClick={onClose}>关闭 — 请先发布至少一个版本</Button>
      )
    }
    if (targetMode === 'upgrade' && activeDeploymentForWs) {
      return (
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button danger onClick={handleUninstall} loading={submitting}>
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
    <Modal
      title="部署到 Workspace"
      open={open}
      onCancel={onClose}
      footer={renderFooter()}
      width={560}
      destroyOnClose
    >
      {versions.length === 0 && !loading && (
        <Alert
          type="warning"
          showIcon
          message="还没有任何已发布版本"
          description="请先点击 顶栏「发布版本」 把当前草稿固化为 vX.Y.Z,然后才能部署。"
          style={{ marginBottom: 12 }}
        />
      )}

      <Form layout="vertical">
        <Form.Item label="选择版本">
          <Select
            value={versionId}
            onChange={setVersionId}
            options={versions.map((v) => ({
              value: v.id,
              label: v.version,
            }))}
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
                <span style={{ color: '#aaa' }}>
                  (red = 必填,需由 workspace/varset 提供值)
                </span>
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
                    {v.sensitive && (
                      <span style={{ color: '#fa8c16', marginLeft: 4 }}>·敏感</span>
                    )}
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
                  <span style={{ color: '#888', fontSize: 11 }}>
                    {w.workspace_id || `id:${w.id}`}
                  </span>
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
                version: {versions.find((v) => v.id === activeDeploymentForWs.version_id)?.version ?? activeDeploymentForWs.version_id}
              </span>
            </div>
          )}
        </Form.Item>

        <Form.Item
          label={
            <span>
              关联 Variable Sets <span style={{ color: '#888', fontWeight: 400 }}>(顺序即优先级,后选的优先级高)</span>
            </span>
          }
        >
          <Select
            mode="multiple"
            value={varsetIds}
            onChange={setVarsetIds}
            options={varsets.map((vs) => ({
              value: vs.varset_id,
              label: `${vs.name} (${vs.scope})`,
            }))}
            placeholder="可不选 — 仅用 workspace 自有变量"
            disabled={loading}
          />
        </Form.Item>
      </Form>
    </Modal>
  )
}
