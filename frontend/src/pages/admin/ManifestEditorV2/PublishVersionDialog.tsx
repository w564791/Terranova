/**
 * 发布版本对话框
 *
 * 把当前用户草稿快照成一个不可变的 ManifestVersion (vX.Y.Z)
 * 接 POST /manifests/:id/v2/versions
 */
import { useEffect, useState } from 'react'
import { Modal, Form, Input, message } from 'antd'
import {
  publishVersion,
  listVersions,
  type ManifestEditorContext,
  type ManifestVersion,
} from './manifestApi'

interface Props {
  open: boolean
  ctx: ManifestEditorContext
  onClose: () => void
  onPublished?: (version: ManifestVersion) => void
}

const SEMVER_RE = /^v\d+\.\d+\.\d+$/

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

export default function PublishVersionDialog({ open, ctx, onClose, onPublished }: Props) {
  const [form] = Form.useForm<{ version: string; changelog: string }>()
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!open) return
    form.resetFields()
    listVersions(ctx)
      .then((vs) => {
        form.setFieldsValue({ version: suggestNextVersion(vs), changelog: '' })
      })
      .catch(() => {
        form.setFieldsValue({ version: 'v1.0.0', changelog: '' })
      })
  }, [open, ctx, form])

  const handleOk = async () => {
    try {
      const values = await form.validateFields()
      setSubmitting(true)
      const v = await publishVersion(ctx, values)
      message.success(`已发布版本 ${values.version}`)
      onPublished?.(v)
      onClose()
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      if (msg) message.error(`发布失败: ${msg}`)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="发布新版本"
      open={open}
      onCancel={onClose}
      onOk={handleOk}
      confirmLoading={submitting}
      okText="发布"
      cancelText="取消"
      destroyOnClose
    >
      <p style={{ color: '#999', marginBottom: 16 }}>
        把当前草稿快照成不可变版本,后续可被部署到 Workspace。
      </p>
      <Form form={form} layout="vertical" preserve={false}>
        <Form.Item
          label="版本号"
          name="version"
          rules={[
            { required: true, message: '请输入版本号' },
            {
              pattern: SEMVER_RE,
              message: '格式必须为 vMAJOR.MINOR.PATCH (如 v1.2.0)',
            },
          ]}
        >
          <Input placeholder="v1.2.0" autoFocus />
        </Form.Item>
        <Form.Item label="发布说明 (可选)" name="changelog">
          <Input.TextArea rows={3} placeholder="例如: 增加 NAT Gateway 配置" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
