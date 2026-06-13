/**
 * 发布版本对话框 — VS Code 暗色主题风格
 *
 * 仅负责版本发布表单。检查流程由父组件管理,通过右侧面板展示。
 * 对话框打开时:若未检查 → 提示"请先检查";已检查 → 直接显示发布表单。
 *
 * 接 POST /manifests/:id/v2/versions
 */
import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { message } from 'antd'
import {
  publishVersion,
  listVersions,
  type ManifestEditorContext,
  type ManifestVersion,
} from './manifestApi'
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
  onPublished?: (version: ManifestVersion) => void
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
  width: 480,
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
}

const bodyStyle: React.CSSProperties = {
  padding: '12px 14px',
}

const footerStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: 8,
  padding: '8px 14px',
  borderTop: '1px solid #454545',
  background: '#2d2d2d',
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

  const publishReady = !!checkSummary?.done || !!checkSummary?.skipped

  useEffect(() => {
    if (!open) return
    setVersionError(null)
    setSubmitError(null)
    setChangelog('')
    listVersions({ orgId, manifestId })
      .then((vs) => setVersion(suggestNextVersion(vs)))
      .catch(() => setVersion('v1.0.0'))
  }, [open, orgId, manifestId])

  const handleOk = async () => {
    if (!publishReady) return
    if (!SEMVER_RE.test(version)) {
      setVersionError('格式必须为 vMAJOR.MINOR.PATCH (如 v1.2.0)')
      return
    }
    setVersionError(null)
    setSubmitError(null)
    try {
      setSubmitting(true)
      const v = await publishVersion(ctx, { version, changelog })
      message.success(`已发布版本 ${version}`)
      onPublished?.(v)
      onClose()
    } catch (err) {
      const msg = typeof err === 'string' ? err : (err as Error)?.message
      if (msg) setSubmitError(`发布失败: ${msg}`)
    } finally {
      setSubmitting(false)
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
            style={{ cursor: 'pointer', opacity: 0.7, padding: '0 4px' }}
            onClick={onClose}
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
                  borderColor: versionError ? '#f14c4c' : '#454545',
                }}
                value={version}
                onChange={(e) => {
                  setVersion(e.target.value)
                  if (versionError) setVersionError(null)
                }}
                placeholder="v1.2.0"
                autoFocus
              />
              {versionError && <div style={{ color: '#f14c4c', fontSize: 12, marginTop: 2 }}>{versionError}</div>}
            </div>
            <div>
              <label style={labelStyle}>发布说明 (可选)</label>
              <textarea
                style={textAreaStyle}
                value={changelog}
                onChange={(e) => setChangelog(e.target.value)}
                rows={3}
                placeholder="例如: 增加 NAT Gateway 配置"
              />
            </div>
            {submitError && (
              <div style={{ marginTop: 8, color: '#f14c4c', fontSize: 12 }}>{submitError}</div>
            )}
          </div>
        </div>

        {/* 底栏按钮 */}
        <div style={footerStyle}>
          <button style={btnSecondaryStyle} onClick={onClose}>取消</button>
          <button
            style={!publishReady || submitting ? btnPrimaryDisabledStyle : btnPrimaryStyle}
            disabled={!publishReady || submitting}
            onClick={() => void handleOk()}
          >
            {submitting && <i className="codicon codicon-loading codicon-modifier-spin" />}
            <i className="codicon codicon-tag" /> 发布
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
