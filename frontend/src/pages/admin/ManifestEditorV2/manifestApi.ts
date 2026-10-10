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
 *   POST   /api/v1/organizations/:org_id/manifests/:id/v2/deployments/variable-preview (首装预览)
 */
import api, { getHttpStatus } from '../../../services/api'

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

/** 列某已发布版本的文件树(只读) */
export async function listVersionFiles(
  ctx: ManifestEditorContext,
  versionId: string,
): Promise<ManifestFileEntry[]> {
  const body = (await api.get(
    `${basePath(ctx)}/files?version=${encodeURIComponent(versionId)}`,
  )) as { files?: ManifestFileEntry[] }
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
// Provider schema（post_init 落库；按 manifest+subpath）
// =============================================================================

export interface ManifestProviderSchemaResponse {
  exists: boolean
  manifest_id: string
  subpath: string
  schema_kind?: string
  providers?: { source: string; version: string }[]
  provider_versions_key?: string
  resources?: string[]
  data?: string[]
  content_hash?: string
  version?: string
  terraform_version?: string
  captured_at?: string
}

/** 拉取编辑器类型补全目录；无缓存时 exists=false */
export async function getProviderSchemas(
  ctx: ManifestEditorContext,
  subpath = '',
): Promise<ManifestProviderSchemaResponse> {
  const qs = subpath ? `?subpath=${encodeURIComponent(subpath)}` : ''
  return (await api.get(
    `${basePath(ctx)}/provider-schemas${qs}`,
  )) as ManifestProviderSchemaResponse
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
  /** 不可变 bundle 哈希;null = 无合法 bundle,部署路径 409 bundle_republish_required,需重新发布 */
  bundle_hash?: string | null
  /** 无合法 bundle 的原因(规则名 + 路径,如 `secret_scan:aws_access_key @ main.tf`,或 `hash_mismatch`) */
  bundle_invalid_reason?: string | null
  created_by: string
  created_at: string
}

// deployment / 任务响应里的覆盖(后端 services.RedactOverrides,b2bb4a7):
//   - sensitive=true 时永不带 value;无该 workspace WORKSPACE_VARIABLES READ 时也不带 value;
//   - 后端 sensitive_keys 为 NULL(未计算)时所有条目都是 sensitive。
// sensitive 只用于展示,绝不能原样回传给后端。
export interface DeploymentOverrideView {
  key: string
  sensitive: boolean
  has_value: boolean
  value?: string
}

// 提交覆盖(install / upgrade / 预览):"k": "v",或用户新输入值并勾选"敏感"时 "k": {value, sensitive: true}
export type OverrideInputValue = string | { value: string; sensitive?: boolean }
export type OverrideInputs = Record<string, OverrideInputValue>

export interface ManifestDeployment {
  id: string
  manifest_id: string
  version_id: string
  workspace_id: string
  status: 'active' | 'uninstalled' | string
  overrides?: DeploymentOverrideView[] | null
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

// 取某 deployment 详情 + 已关联的 varset(按 priority 升序),用于 upgrade 表单预填。
export async function getDeploymentVarsets(
  ctx: ManifestEditorContext,
  deploymentId: string,
): Promise<string[]> {
  const ctx2 = await getDeploymentUpgradeContext(ctx, deploymentId)
  return ctx2.varsetIds
}

/** upgrade 时需要保留的既有 varset / 已存覆盖(脱敏视图,仅供展示,不回传) */
export interface DeploymentUpgradeContext {
  varsetIds: string[]
  varsets: DeploymentVarsetEntry[]
  overrides: DeploymentOverrideView[]
}

// 取 deployment 的 varset + 已存覆盖(overrides 脱敏视图),供 upgrade / 发布后自动更新复用。
export async function getDeploymentUpgradeContext(
  ctx: ManifestEditorContext,
  deploymentId: string,
): Promise<DeploymentUpgradeContext> {
  const data = (await api.get(`${basePath(ctx)}/v2/deployments/${deploymentId}`)) as {
    deployment?: { overrides?: DeploymentOverrideView[] | null }
    varsets?: { varset_id: string; priority: number }[]
  }
  const sorted = (data.varsets ?? [])
    .slice()
    .sort((a, b) => a.priority - b.priority)
  const varsetIds = sorted.map((v) => v.varset_id)
  const varsets: DeploymentVarsetEntry[] = sorted.map((v, i) => ({
    varset_id: v.varset_id,
    priority: v.priority ?? i,
  }))
  const overrides = Array.isArray(data.deployment?.overrides)
    ? data.deployment!.overrides!.filter((o) => o && typeof o.key === 'string')
    : []
  return { varsetIds, varsets, overrides }
}

// 变量预览(upgrade 用):合并后的最终变量,每条带 sensitive。
// sensitive=true 时 value 恒为空串 —— 不是真实值,前端不得当作值预填。
export interface DeploymentPreviewVariable {
  key: string
  value: string
  sensitive: boolean
  variable_type?: string
  value_format?: string
  description?: string
  source_type?: string
}

// per-deployment 预览(后端 DeploymentPreviewRequest):服务端与 upgrade 同一 mergeDeploymentOverrides
// 合并已存覆盖,前端不得回传已存值;只发本次改动的 key 与 unset_keys。
// target_version_id 可选,参与敏感判定(须属于本 manifest,否则 400)。
// varsets 缺省 = 使用部署已挂的 varset(后端 b89568d 指针语义);[] = 无;非空 = 以此为准。
export interface DeploymentPreviewRequest {
  target_version_id?: string
  varsets?: DeploymentVarsetEntry[]
  variable_overrides?: OverrideInputs
  unset_keys?: string[]
}

export async function previewDeploymentVariables(
  ctx: ManifestEditorContext,
  deploymentId: string,
  body: DeploymentPreviewRequest,
): Promise<DeploymentPreviewVariable[]> {
  const data = (await api.post(
    `${basePath(ctx)}/v2/deployments/${deploymentId}/variable-preview`,
    body,
  )) as { variables?: DeploymentPreviewVariable[] }
  return data.variables ?? []
}

// 首次安装前预览(尚无 deployment):与上面的 per-deployment 预览同形响应,敏感值恒为空串。
// workspace 不属于本 org / version 不存在或为草稿 => 404(见 isManifestTargetNotFound)。
export interface InstallPreviewRequest {
  workspace_id: string
  version_id: string
  varsets: DeploymentVarsetEntry[]
  variable_overrides?: OverrideInputs
}

export async function previewInstallVariables(
  ctx: ManifestEditorContext,
  body: InstallPreviewRequest,
): Promise<DeploymentPreviewVariable[]> {
  const data = (await api.post(`${basePath(ctx)}/v2/deployments/variable-preview`, body)) as {
    variables?: DeploymentPreviewVariable[]
  }
  return data.variables ?? []
}

// install / 首装预览的 404(resolveInstallTarget / ManifestInAuthOrg):以 HTTP 状态码判断;
// 仅当错误不带状态码(非拦截器抛出的旧式字符串错误等)时,才回退到后端固定的 404 文案匹配。
const MANIFEST_TARGET_NOT_FOUND_ERRORS = new Set([
  'workspace not found',
  'version not found',
  'manifest not found',
])

export const MANIFEST_TARGET_NOT_FOUND_MESSAGE = '版本或工作区不存在或无权访问'

export function isManifestTargetNotFound(err: unknown): boolean {
  const status = getHttpStatus(err)
  if (status !== undefined) return status === 404
  const msg = typeof err === 'string' ? err : (err as Error | undefined)?.message
  return !!msg && MANIFEST_TARGET_NOT_FOUND_ERRORS.has(msg.trim().toLowerCase())
}

export interface InstallDeploymentRequest {
  version_id: string
  workspace_id: string
  varsets: DeploymentVarsetEntry[]
  variable_overrides?: OverrideInputs
  // terraform 执行子目录(空串=根)。省略则后端沿用 workspace 已有 manifest_subpath。
  workdir?: string
}

export async function installDeployment(
  ctx: ManifestEditorContext,
  body: InstallDeploymentRequest,
) {
  return await api.post(`${basePath(ctx)}/v2/deployments/install`, body)
}

// upgrade 的 variable_overrides 与已存覆盖合并(后端 c89940e):
//   缺省的 key 保留原值;敏感 key 传空串(预览掩码占位)也保留原值;
//   只有 unset_keys 里的 key 会从已存覆盖中删除。因此只需发送用户改动过的 key。
//   敏感标记粘滞(后端 b2bb4a7):已敏感的 key 无法改回普通。
// varsets(后端 b89568d 指针):缺省 = 保持已挂 varset 不变(不算变量变更);[] = 清空;非空 = 整体替换。
export interface UpgradeDeploymentRequest {
  target_version_id: string
  varsets?: DeploymentVarsetEntry[]
  variable_overrides?: OverrideInputs
  unset_keys?: string[]
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
  /** external_files 为某已发布版本的内容时给出(后端 1aed72a:校验版本并比对 bundle_hash) */
  manifest_version_id?: string
}

export async function runPlanWithDraft(req: RunPlanRequest) {
  // 注意路径: 用 workspace 现有的 task 创建路径,不在 manifest namespace 下
  return await api.post(`/workspaces/${req.workspace_id}/tasks/plan`, {
    description: req.manifest_version_id ? 'Manifest Run (已发布版本预览)' : 'Manifest Run (草稿预览)',
    run_type: 'plan',
    external_files: req.external_files,
    ...(req.manifest_version_id ? { manifest_version_id: req.manifest_version_id } : {}),
  })
}

// 在已装 manifest 的 workspace 触发一次 Plan+Apply(部署并运行)。
// 不带 external_files → 走 manifest 分支,按已发布版本 + subpath 执行。返回 task id。
export async function triggerWorkspacePlanApply(
  workspaceId: string,
  description?: string,
): Promise<string | number | undefined> {
  const resp = (await api.post(`/workspaces/${workspaceId}/tasks/plan`, {
    description: description || 'Manifest 部署并运行 (Plan+Apply)',
    run_type: 'plan_and_apply',
  })) as { task?: { id?: number | string }; task_id?: number | string; id?: number | string }
  return resp.task?.id ?? resp.task_id ?? resp.id
}

// =============================================================================
// 工具: 路径 → 编辑器语言
// =============================================================================

/** 路径 → 编辑器语言(PR2-C 会精细化, 当前最小集) */
export function languageOfPath(path: string): string {
  // 对齐 HashiCorp 官方 language id 映射(hashicorp/syntax + marketplace 插件)
  if (path.endsWith('.tf')) return 'terraform'
  if (path.endsWith('.tfvars')) return 'terraform-vars'
  if (path.endsWith('.hcl')) return 'hcl'
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
