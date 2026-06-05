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
  // 注意: api 响应拦截器已返回 response.data,所以这里拿到的就是后端 JSON 体 {files:[...]}
  const body = (await api.get(`${basePath(ctx)}/files`)) as { files?: ManifestFileEntry[] }
  return body.files || []
}

/** 读单文件(text 直接返回 content, binary 走 content_b64) */
export async function readFile(
  ctx: ManifestEditorContext,
  path: string,
  ref?: string, // 版本 ref: 省略/'draft'=当前用户草稿; 否则 version_id
): Promise<ManifestFileContent> {
  const qs = ref && ref !== 'draft' ? `?version=${encodeURIComponent(ref)}` : ''
  // 拦截器已解包,直接就是 ManifestFileContent
  return (await api.get(`${basePath(ctx)}/files/${encodeURIComponent(path)}${qs}`)) as ManifestFileContent
}

/** 写入草稿单文件 */
export async function putFile(
  ctx: ManifestEditorContext,
  path: string,
  content: string,
): Promise<void> {
  await api.put(`${basePath(ctx)}/files/${encodeURIComponent(path)}`, { content })
}

/** 上传草稿单文件(二进制安全,走 content_b64);用于拖拽上传本地文件 */
export async function putFileB64(
  ctx: ManifestEditorContext,
  path: string,
  contentB64: string,
): Promise<void> {
  await api.put(`${basePath(ctx)}/files/${encodeURIComponent(path)}`, { content_b64: contentB64 })
}

/** 删除草稿单文件 */
export async function deleteFile(
  ctx: ManifestEditorContext,
  path: string,
): Promise<void> {
  await api.delete(`${basePath(ctx)}/files/${encodeURIComponent(path)}`)
}

/** 删除整个目录(删该前缀下所有草稿文件) */
export async function deleteDir(
  ctx: ManifestEditorContext,
  dir: string,
): Promise<void> {
  await api.post(`${basePath(ctx)}/files/_delete_dir`, { dir })
}

/** 重命名 / 移动(单文件) */
export async function moveFile(
  ctx: ManifestEditorContext,
  from: string,
  to: string,
): Promise<void> {
  await api.post(`${basePath(ctx)}/files/_move`, { from, to })
}

/** 移动整个目录(按前缀批量移动草稿文件,后端事务原子,冲突即整体失败) */
export async function moveDir(
  ctx: ManifestEditorContext,
  from: string,
  to: string,
): Promise<void> {
  await api.post(`${basePath(ctx)}/files/_move_dir`, { from, to })
}

/** 用某 published 版本覆盖当前用户草稿 */
export async function resetDraftFrom(
  ctx: ManifestEditorContext,
  versionId: string,
): Promise<void> {
  await api.post(`${basePath(ctx)}/draft/_reset_from?version_id=${encodeURIComponent(versionId)}`)
}

// =============================================================================
// 版本 / 部署
// =============================================================================

// manifest 版本声明的 input variable 元信息(发布时由后端浅 parse variable block 得到)
// 平台不维护类型系统: type_raw / default_raw 是 HCL 表达式原始源码字符串,仅供展示。
export interface ManifestVariableMeta {
  name: string
  description?: string
  required: boolean
  sensitive?: boolean
  type_raw?: string
  default_raw?: string
}

export interface ManifestVersion {
  id: string
  manifest_id: string
  version: string
  changelog: string
  variables?: ManifestVariableMeta[] | null
  created_by: string
  created_at: string
}

export interface ManifestDeployment {
  id: string
  manifest_id: string
  version_id: string
  workspace_id: string
  status: 'active' | 'uninstalled' | string
  variable_overrides?: Record<string, unknown>
  deployed_by: string
  deployed_at?: string
}

export interface PublishVersionRequest {
  version: string
  changelog?: string
}

export async function listVersions(ctx: ManifestEditorContext): Promise<ManifestVersion[]> {
  const data = (await api.get(`${basePath(ctx)}/v2/versions`)) as { versions?: ManifestVersion[] }
  return data.versions ?? []
}

export async function publishVersion(
  ctx: ManifestEditorContext,
  body: PublishVersionRequest,
): Promise<ManifestVersion> {
  return (await api.post(`${basePath(ctx)}/v2/versions`, body)) as ManifestVersion
}

// 文件级变更条目(target 相对 base)
export type DiffState = 'added' | 'removed' | 'changed' | 'unchanged'
export interface DiffEntry {
  path: string
  state: DiffState
}

// 两个已发布版本 diff(target=versionId 相对 base=against)
export async function diffVersions(
  ctx: ManifestEditorContext,
  versionId: string,
  against: string,
): Promise<DiffEntry[]> {
  const data = (await api.get(
    `${basePath(ctx)}/v2/versions/${versionId}/diff?against=${encodeURIComponent(against)}`,
  )) as { files?: DiffEntry[] }
  return data.files ?? []
}

// 列某版本里可用的 workdir 目录(直接含 .tf 的目录,根用 '')。供 install 选执行子目录。
export async function listVersionWorkdirs(
  ctx: ManifestEditorContext,
  versionId: string,
): Promise<string[]> {
  const data = (await api.get(`${basePath(ctx)}/v2/versions/${versionId}/workdirs`)) as {
    workdirs?: string[]
  }
  return data.workdirs && data.workdirs.length > 0 ? data.workdirs : ['']
}

// 当前用户草稿 vs 某版本(against 省略=最新已发布)diff
export async function diffDraft(
  ctx: ManifestEditorContext,
  against?: string,
): Promise<{ baseVersionId: string; files: DiffEntry[] }> {
  const qs = against ? `?against=${encodeURIComponent(against)}` : ''
  const data = (await api.get(`${basePath(ctx)}/v2/draft/diff${qs}`)) as {
    base_version_id?: string
    files?: DiffEntry[]
  }
  return { baseVersionId: data.base_version_id ?? '', files: data.files ?? [] }
}

export async function listDeployments(ctx: ManifestEditorContext): Promise<ManifestDeployment[]> {
  const data = (await api.get(`${basePath(ctx)}/v2/deployments`)) as {
    deployments?: ManifestDeployment[]
  }
  return data.deployments ?? []
}

export interface DeploymentVarsetEntry {
  varset_id: string
  priority: number
}

export interface InstallDeploymentRequest {
  version_id: string
  workspace_id: string
  varsets: DeploymentVarsetEntry[]
  variable_overrides?: Record<string, string>
  // terraform 执行子目录(空串=根)。省略则后端沿用 workspace 已有 manifest_subpath。
  workdir?: string
}

export async function installDeployment(
  ctx: ManifestEditorContext,
  body: InstallDeploymentRequest,
) {
  return await api.post(`${basePath(ctx)}/v2/deployments/install`, body)
}

export interface UpgradeDeploymentRequest {
  target_version_id: string
  varsets: DeploymentVarsetEntry[]
  variable_overrides?: Record<string, string>
}

export async function upgradeDeployment(
  ctx: ManifestEditorContext,
  deploymentId: string,
  body: UpgradeDeploymentRequest,
) {
  return await api.post(`${basePath(ctx)}/v2/deployments/${deploymentId}/upgrade`, body)
}

export async function uninstallDeployment(
  ctx: ManifestEditorContext,
  deploymentId: string,
) {
  return await api.post(`${basePath(ctx)}/v2/deployments/${deploymentId}/uninstall`)
}

// =============================================================================
// Run (调 workspace 现有 plan-only,带 external_files)
// =============================================================================

export interface RunPlanRequest {
  workspace_id: string
  external_files: { path: string; content_b64: string }[]
}

export async function runPlanWithDraft(req: RunPlanRequest) {
  // 注意路径: 用 workspace 现有的 task 创建路径,不在 manifest namespace 下
  return await api.post(`/workspaces/${req.workspace_id}/tasks/plan`, {
    description: 'Manifest Run (草稿预览)',
    run_type: 'plan',
    external_files: req.external_files,
  })
}

// =============================================================================
// 工具: 路径 → 编辑器语言
// =============================================================================

/** 路径 → 编辑器语言(PR2-C 会精细化, 当前最小集) */
export function languageOfPath(path: string): string {
  if (path.endsWith('.tf') || path.endsWith('.tfvars') || path.endsWith('.hcl')) return 'hcl'
  if (path.endsWith('.md') || path.endsWith('.markdown')) return 'markdown'
  if (path.endsWith('.json')) return 'json'
  if (path.endsWith('.yaml') || path.endsWith('.yml')) return 'yaml'
  if (path.endsWith('.sh') || path.endsWith('.tpl')) return 'shellscript'
  if (path.endsWith('.xml')) return 'xml'
  if (path.endsWith('.html') || path.endsWith('.htm')) return 'html'
  if (path.endsWith('.toml')) return 'ini' // monaco 无 toml,ini 高亮近似
  if (path.endsWith('.env') || path.endsWith('.conf') || path.endsWith('.ini')) return 'ini'
  return 'plaintext'
}
