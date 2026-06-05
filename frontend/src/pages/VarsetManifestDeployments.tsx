/**
 * Variable Set 反向关联面板
 *
 * 接 PR1 后端: GET /variable-sets/:varset_id/manifest-deployments
 * 列出当前 active 的 manifest deployment 中正在使用此 varset 的列表。
 *
 * 用于 VariableSetDetail 页面底部, 让用户看到"改这个 varset 会影响哪些 manifest 部署"。
 */
import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import api from '../services/api'

interface ManifestDeploymentLink {
  deployment_id: string
  manifest_id: string
  workspace_id: string
  version_id: string
  priority: number
  deployed_at: string
}

interface Props {
  varsetId: string
}

const VarsetManifestDeployments: React.FC<Props> = ({ varsetId }) => {
  const navigate = useNavigate()
  const [items, setItems] = useState<ManifestDeploymentLink[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .get(`/variable-sets/${varsetId}/manifest-deployments`)
      .then((data) => {
        if (cancelled) return
        const resp = data as unknown as { deployments?: ManifestDeploymentLink[] }
        setItems(resp.deployments ?? [])
        setError(null)
      })
      .catch((err) => {
        if (cancelled) return
        const msg = typeof err === 'string' ? err : (err as Error)?.message
        setError(msg ?? '加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [varsetId])

  return (
    <div
      style={{
        marginTop: 24,
        paddingTop: 20,
        borderTop: '1px solid #e5e7eb',
      }}
    >
      <h3
        style={{
          fontSize: 16,
          fontWeight: 600,
          color: '#111827',
          margin: '0 0 4px 0',
        }}
      >
        关联的 Manifest Deployment
      </h3>
      <p style={{ color: '#6b7280', fontSize: 13, marginTop: 0, marginBottom: 12 }}>
        修改此变量集后,以下正在使用它的 manifest 部署在下次 plan/apply 时会获取新值。
      </p>

      {loading && <div style={{ color: '#6b7280', fontSize: 13 }}>加载中...</div>}
      {error && <div style={{ color: '#dc2626', fontSize: 13 }}>加载失败: {error}</div>}
      {!loading && !error && items.length === 0 && (
        <div style={{ color: '#6b7280', fontSize: 13, padding: '8px 0' }}>
          暂无 manifest deployment 引用此变量集。
        </div>
      )}
      {!loading && !error && items.length > 0 && (
        <div
          style={{
            border: '1px solid #e5e7eb',
            borderRadius: 6,
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 1fr 1fr 80px 1fr',
              padding: '8px 12px',
              background: '#f9fafb',
              fontSize: 12,
              fontWeight: 600,
              color: '#374151',
              borderBottom: '1px solid #e5e7eb',
            }}
          >
            <div>Manifest</div>
            <div>Deployment</div>
            <div>Workspace</div>
            <div style={{ textAlign: 'right' }}>Priority</div>
            <div>Deployed at</div>
          </div>
          {items.map((it) => (
            <div
              key={it.deployment_id}
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 1fr 1fr 80px 1fr',
                padding: '8px 12px',
                fontSize: 13,
                color: '#374151',
                borderBottom: '1px solid #f3f4f6',
                cursor: 'pointer',
                background: '#fff',
              }}
              onClick={() => {
                // 跳到 manifest 编辑器(不带 org_id 的话编辑器会从 localStorage 取)
                navigate(`/admin/manifests-v2/${it.manifest_id}/edit`)
              }}
            >
              <div style={{ fontFamily: 'monospace', fontSize: 12 }}>
                {it.manifest_id}
              </div>
              <div style={{ fontFamily: 'monospace', fontSize: 12, color: '#6b7280' }}>
                {it.deployment_id}
              </div>
              <div style={{ fontFamily: 'monospace', fontSize: 12 }}>
                {it.workspace_id}
              </div>
              <div style={{ textAlign: 'right' }}>{it.priority}</div>
              <div style={{ color: '#6b7280', fontSize: 12 }}>
                {it.deployed_at ? new Date(it.deployed_at).toLocaleString() : '-'}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default VarsetManifestDeployments
