/**
 * Run 对话框 — 对当前用户草稿在已装本 manifest 的 workspace 上跑 plan-only
 *
 * 行为:
 *  1. 列出已装本 manifest 的 workspaces (deployments status='active')
 *  2. 用户选一个,前端把当前草稿全量打包成 external_files
 *  3. 调 POST /workspaces/:id/tasks/plan 带 external_files (PR1.5 已扩字段)
 *  4. 跳转到 workspace plan 任务结果页
 *
 * 灰禁条件:本 manifest 没有任何 active deployment (toolbar 直接 disabled)
 */
import { useEffect, useState } from 'react'
import { Modal, Select, Alert, Button, Space, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import {
  listFiles,
  readFile,
  listDeployments,
  runPlanWithDraft,
  type ManifestEditorContext,
  type ManifestDeployment,
} from './manifestApi'
import { workspaceService, type Workspace } from '../../../services/workspaces'

interface Props {
  open: boolean
  ctx: ManifestEditorContext
  onClose: () => void
}

interface RunTarget {
  deployment: ManifestDeployment
  workspace: Workspace
}

export default function RunDialog({ open, ctx, onClose }: Props) {
  const navigate = useNavigate()
  const [targets, setTargets] = useState<RunTarget[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [selected, setSelected] = useState<string | undefined>()

  useEffect(() => {
    if (!open) return
    setLoading(true)
    Promise.all([
      listDeployments(ctx).catch(() => []),
      // /workspaces 响应是 {code, data:{items:[...]}};取 data.items,带数组兜底
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

  const handleRun = async () => {
    if (!selected) return
    setSubmitting(true)
    try {
      // 1. 拉所有草稿文件
      const fileList = await listFiles(ctx)
      const fileContents = await Promise.all(
        fileList.map(async (f) => {
          const file = await readFile(ctx, f.path)
          let b64: string
          if (file.content_b64) {
            b64 = file.content_b64
          } else {
            // text 文件,前端编码 base64
            b64 = btoa(unescape(encodeURIComponent(file.content ?? '')))
          }
          return { path: file.path, content_b64: b64 }
        }),
      )
      if (fileContents.length === 0) {
        message.warning('草稿为空,无文件可 run')
        setSubmitting(false)
        return
      }
      // 2. 调 workspace plan-only
      const resp = (await runPlanWithDraft({
        workspace_id: selected,
        external_files: fileContents,
      })) as { task_id?: number | string; id?: number | string }
      const taskId = resp.task_id ?? resp.id
      message.success(`已提交 plan-only 任务${taskId ? ' #' + taskId : ''},跳转到 workspace 查看结果`)
      onClose()
      // 3. 跳转 workspace
      navigate(`/workspaces/${selected}`)
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      message.error(`Run 失败: ${msg ?? '未知错误'}`)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="Run — 在已装本 manifest 的 workspace 上跑 plan-only"
      open={open}
      onCancel={onClose}
      footer={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button
            type="primary"
            onClick={handleRun}
            loading={submitting}
            disabled={!selected || loading}
          >
            Run
          </Button>
        </Space>
      }
      destroyOnClose
    >
      <Alert
        type="info"
        showIcon
        message="Run 仅 plan,不会动云端"
        description="把当前草稿临时打包成文件上传到目标 workspace,跑一次 plan-only。结果在 workspace 任务结果页查看。不会写入 manifest_files,不会污染任何持久状态。"
        style={{ marginBottom: 16 }}
      />

      {!loading && targets.length === 0 ? (
        <Alert
          type="warning"
          showIcon
          message="本 manifest 没有任何 active deployment"
          description="请先发布版本并部署到至少一个 workspace,然后才能用 Run 检测草稿。"
        />
      ) : (
        <Select
          value={selected}
          onChange={setSelected}
          style={{ width: '100%' }}
          options={targets.map((t) => ({
            value: t.workspace.workspace_id || String(t.workspace.id),
            label: `${t.workspace.name} (${t.workspace.workspace_id || 'id:' + t.workspace.id})`,
          }))}
          placeholder="选择目标 workspace"
          disabled={loading}
        />
      )}
    </Modal>
  )
}
