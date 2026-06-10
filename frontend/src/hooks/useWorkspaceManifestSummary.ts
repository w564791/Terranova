/**
 * useWorkspaceManifestSummary
 *
 * 拉 workspace 的 manifest 软链接摘要(deployment + manifest 名 + org_id)。
 * 共享给 ResourcesTab 的徽章 / 顶部 banner / 资源添加按钮锁定状态使用。
 *
 * 后端: GET /workspaces/:workspace_id/manifest-summary (PR3 新加)
 */
import { useEffect, useState } from 'react'
import api from '../services/api'

export interface WorkspaceManifestSummary {
  workspace_id: string
  has_manifest: boolean
  deployment_id?: string
  version_id?: string
  active_tag?: string
  subpath?: string | null
  manifest_id?: string
  manifest_name?: string
  org_id?: number
  status?: 'active' | 'uninstalled' | string
}

export function useWorkspaceManifestSummary(workspaceId: string | undefined) {
  const [summary, setSummary] = useState<WorkspaceManifestSummary | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!workspaceId) return
    let cancelled = false
    setLoading(true)
    api
      .get(`/workspaces/${workspaceId}/manifest-summary`)
      .then((data) => {
        // axios 响应拦截器已经返回 response.data, 这里 data 就是后端 JSON
        if (!cancelled) setSummary(data as unknown as WorkspaceManifestSummary)
      })
      .catch(() => {
        if (!cancelled) setSummary(null)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [workspaceId])

  return { summary, loading }
}
