/**
 * Manifest Editor v2 — 后端 API 客户端
 *
 * 接 PR1 实现的路由:
 *   GET    /api/v1/organizations/:org_id/manifests/:id/files
 *   GET    /api/v1/organizations/:org_id/manifests/:id/files/*path
 *   PUT    /api/v1/organizations/:org_id/manifests/:id/files/*path
 *   DELETE /api/v1/organizations/:org_id/manifests/:id/files/*path
 *   POST   /api/v1/organizations/:org_id/manifests/:id/files/_move
 *   POST   /api/v1/organizations/:org_id/manifests/:id/draft/_reset_from
 */
import api from '../../../services/api'

export interface ManifestFileEntry {
  path: string
  name: string
  type: 'file' | 'dir'
  size: number
  mime: string
  is_binary: boolean
}

export interface ManifestFileContent {
  path: string
  size: number
  mime: string
  is_binary: boolean
  content?: string
  content_b64?: string
  updated_at: string
}

export interface ManifestEditorContext {
  orgId: string | number
  manifestId: string
}

function basePath(ctx: ManifestEditorContext) {
  return `/organizations/${ctx.orgId}/manifests/${ctx.manifestId}`
}

/** 列文件树(草稿区, 自动绑定当前登录用户) */
export async function listFiles(ctx: ManifestEditorContext): Promise<ManifestFileEntry[]> {
  const { data } = await api.get(`${basePath(ctx)}/files`)
  return data.files || []
}

/** 读单文件(text 直接返回 content, binary 走 content_b64) */
export async function readFile(
  ctx: ManifestEditorContext,
  path: string,
): Promise<ManifestFileContent> {
  const { data } = await api.get(`${basePath(ctx)}/files/${encodeURIComponent(path)}`)
  return data
}

/** 写入草稿单文件 */
export async function putFile(
  ctx: ManifestEditorContext,
  path: string,
  content: string,
): Promise<void> {
  await api.put(`${basePath(ctx)}/files/${encodeURIComponent(path)}`, { content })
}

/** 删除草稿单文件 */
export async function deleteFile(
  ctx: ManifestEditorContext,
  path: string,
): Promise<void> {
  await api.delete(`${basePath(ctx)}/files/${encodeURIComponent(path)}`)
}

/** 重命名 / 移动 */
export async function moveFile(
  ctx: ManifestEditorContext,
  from: string,
  to: string,
): Promise<void> {
  await api.post(`${basePath(ctx)}/files/_move`, { from, to })
}

/** 用某 published 版本覆盖当前用户草稿 */
export async function resetDraftFrom(
  ctx: ManifestEditorContext,
  versionId: string,
): Promise<void> {
  await api.post(`${basePath(ctx)}/draft/_reset_from?version_id=${encodeURIComponent(versionId)}`)
}

/** 路径 → 编辑器语言(PR2-C 会精细化, 当前最小集) */
export function languageOfPath(path: string): string {
  if (path.endsWith('.tf') || path.endsWith('.tfvars') || path.endsWith('.hcl')) return 'hcl'
  if (path.endsWith('.md') || path.endsWith('.markdown')) return 'markdown'
  if (path.endsWith('.json')) return 'json'
  if (path.endsWith('.yaml') || path.endsWith('.yml')) return 'yaml'
  if (path.endsWith('.sh') || path.endsWith('.tpl')) return 'shellscript'
  return 'plaintext'
}
