/**
 * 不可变版本 bundle 状态(后端 fdd46a8 / b89568d):
 *  - 版本 bundle_hash 为空 => 无合法 bundle,install / 预览 / upgrade 到该版本会 409 bundle_republish_required;
 *    bundle_invalid_reason 只含规则名与路径(如 `secret_scan:aws_access_key @ main.tf`)。
 *  - bundle_invalid_reason === 'hash_mismatch' => 完整性校验失败(红色标签)。
 *  - 发布被 bundle 规则拒绝 => 422 bundle_rules_violated + problems {file, line?, rule, message}(绝无文件内容)。
 *  - 部署路径 409 bundle_republish_required 带 reason(hash_mismatch 或规则名与路径)。
 * 注意:列表 / 详情只返回已存值,列表里看似正常的版本仍可能在部署路径上 409,调用方需在 409 后重拉版本。
 */
import { ApiError, getHttpStatus } from '../../../services/api'
import type { ManifestVersion } from './manifestApi'

export const REPUBLISH_REQUIRED_MESSAGE = '该版本需要重新发布'
export const HASH_MISMATCH_MESSAGE = '完整性校验失败，请重新发布'
const HASH_MISMATCH_REASON = 'hash_mismatch'
const HASH_MISMATCH_TOOLTIP = '已发布内容与哈希不符，请联系管理员并重新发布'

type BundleFields = Pick<ManifestVersion, 'bundle_hash' | 'bundle_invalid_reason'>

export function isHashMismatch(v: BundleFields | undefined | null): boolean {
  return v?.bundle_invalid_reason === HASH_MISMATCH_REASON
}

/** 版本不可部署(需重新发布):bundle_hash 为空,或完整性校验失败 */
export function versionNeedsRepublish(v: BundleFields | undefined | null): boolean {
  if (!v) return false
  return !v.bundle_hash || isHashMismatch(v)
}

/**
 * bundle 规则码 -> 中文说明(后端 backend/internal/manifestbundle/rules.go / hcl.go 的 Rule* 常量;
 * 后端 message 固定为英文,前端按 rule 显示中文,未知规则回退后端 message)。
 * 数值与后端 MaxPathLen / MaxFileSize / MaxBundleSize / MaxFiles 保持一致。
 */
const RULE_TEXT: Record<string, string> = {
  path_invalid: '路径不合法：须为相对 POSIX 路径，不能含空段、"."、".."、反斜杠或控制字符',
  path_too_long: '路径超过 256 字节',
  path_not_nfc: '路径未做 Unicode NFC 规范化',
  path_duplicate: '路径重复',
  path_case_duplicate: '路径与其他文件仅大小写不同',
  denylisted_file: '该文件类型不允许放入 bundle（变量值、state、git 元数据、env 或私钥文件）',
  file_too_large: '文件超过 1 MB',
  bundle_too_large: 'bundle 总大小超过 50 MB',
  too_many_files: 'bundle 文件数超过 2000 个',
  // HCL 执行面检查(后端 manifestbundle/hcl.go,dff2936;仅发布时检查,带行号)
  hcl_parse_error: 'Terraform 配置文件无法解析',
  hcl_provisioner: '不允许使用 provisioner 块（local-exec、remote-exec、file 等）',
  hcl_external_data: '不允许使用 "external" 数据源 / hashicorp/external provider',
  hcl_http_data: '不允许使用 "http" 数据源 / hashicorp/http provider',
  hcl_module_source: 'module source 不被允许：请使用 bundle 内的相对路径或平台模块目录中已注册的模块',
  // git 来源(后端 c9d7b3c / 6c28579)
  hcl_module_unpinned: 'git module 的 source 必须用 ?ref=<40 位 commit SHA> 固定',
  git_symlink: '仓库中包含符号链接，不允许发布',
  git_submodule: '仓库中包含 submodule，不允许发布',
}
const SECRET_SCAN_PREFIX = 'secret_scan:'

/** 规则码的中文说明;未知规则返回 null(调用方回退后端 message / 原始规则名) */
export function ruleText(rule: string): string | null {
  if (Object.prototype.hasOwnProperty.call(RULE_TEXT, rule)) return RULE_TEXT[rule]
  if (rule.startsWith(SECRET_SCAN_PREFIX)) {
    const kind = rule.slice(SECRET_SCAN_PREFIX.length)
    return kind ? `疑似包含密钥（${kind}）` : '疑似包含密钥'
  }
  return null
}

/** 发布问题的展示文案:按 rule 显示中文,未知规则回退后端 message */
export function publishProblemText(p: Pick<PublishProblem, 'rule' | 'message'>): string {
  return ruleText(p.rule) ?? p.message
}

/**
 * bundle_invalid_reason(`rule @ path; rule @ path; (+N more)`)本地化为多行中文;
 * 无法识别的片段原样保留。hash_mismatch 单独处理(见 bundleStatusLabel)。
 */
export function localizeInvalidReason(reason: string): string {
  return reason
    .split('; ')
    .map((part) => {
      const more = /^\(\+(\d+) more\)$/.exec(part)
      if (more) return `（另有 ${more[1]} 个问题）`
      const at = part.indexOf(' @ ')
      const rule = at >= 0 ? part.slice(0, at) : part
      const text = ruleText(rule)
      if (!text) return part
      return at >= 0 ? `${text} @ ${part.slice(at + 3)}` : text
    })
    .join('\n')
}

/**
 * 失效原因(bundle_invalid_reason / 任务 error_message 前缀后的 reason)本地化:
 * hash_mismatch => 完整性校验失败,其余按 `rule @ path; ...` 逐条翻译(未知规则原样保留)。
 */
export function localizeBundleReason(reason: string): string {
  const r = reason.trim()
  if (!r) return ''
  if (r === HASH_MISMATCH_REASON) return '完整性校验失败（已发布内容与哈希不符）'
  return localizeInvalidReason(r)
}

/** 409 bundle_hash_mismatch(编辑器 Run 已发布版本时,提交的文件与该版本 bundle_hash 不符) */
export function isBundleHashMismatch(err: unknown): boolean {
  if (getHttpStatus(err) !== 409) return false
  return errorData(err)?.code === 'bundle_hash_mismatch'
}

/** 已部署版本失效(需重新发布 / 完整性校验失败)时的提示 */
export const STALE_DEPLOYMENT_MESSAGE = '当前版本已失效，workspace 上的 plan/apply/drift 会失败，请升级到有效版本'

/**
 * 部署的已装版本是否失效:在版本列表中找到该版本且 bundle_hash 为空或 hash_mismatch。
 * 版本列表里找不到(未加载 / 已删除)时不判定为失效。
 */
export function deploymentVersionStale(
  d: { version_id: string } | undefined | null,
  versions: readonly ManifestVersion[],
): boolean {
  if (!d) return false
  return versionNeedsRepublish(versions.find((v) => v.id === d.version_id))
}

/** 下拉 option 的后缀文案与 title(原生 select 无法放 Tag);tooltip 可能含换行 */
export function bundleStatusLabel(v: BundleFields): { label: string; tooltip: string } | null {
  if (!versionNeedsRepublish(v)) return null
  if (isHashMismatch(v)) return { label: '完整性校验失败', tooltip: HASH_MISMATCH_TOOLTIP }
  const reason = v.bundle_invalid_reason
  if (!reason) return { label: '需重新发布', tooltip: '该版本没有合法 bundle,请重新发布' }
  const localized = localizeInvalidReason(reason)
  return {
    label: '需重新发布',
    tooltip: localized === reason ? reason : `${localized}\n\n原始原因：${reason}`,
  }
}

function errorData(err: unknown): Record<string, unknown> | null {
  const d = err instanceof ApiError ? err.data : (err as { data?: unknown } | null)?.data
  return d && typeof d === 'object' ? (d as Record<string, unknown>) : null
}

/** 409 bundle_republish_required(后端 b89568d:{code, reason, version_id}) */
export function isBundleRepublishRequired(err: unknown): boolean {
  if (getHttpStatus(err) !== 409) return false
  return errorData(err)?.code === 'bundle_republish_required'
}

/** 409 bundle_republish_required 的提示:reason hash_mismatch => 完整性校验失败,其余 => 需要重新发布 */
export function republishRequiredMessage(err: unknown): string {
  return errorData(err)?.reason === HASH_MISMATCH_REASON ? HASH_MISMATCH_MESSAGE : REPUBLISH_REQUIRED_MESSAGE
}

/** 发布 422 的单条问题(后端 b89568d:{file, line?, rule, message},绝不含文件内容) */
export interface PublishProblem {
  /** 相对路径;bundle 级规则(如 bundle_too_large)为空串,不可跳转 */
  file: string
  /** 1-based 行号:密钥扫描(secret_scan:*)与 HCL 检查(hcl_*)带;路径 / 大小 / 黑名单规则不带 */
  line?: number
  rule: string
  /** 服务端固定说明;为空时回退为规则名 */
  message: string
}

/** 解析发布 422 的 problems;非 422 或无 problems 返回 null。 */
export function parsePublishProblems(err: unknown): PublishProblem[] | null {
  if (getHttpStatus(err) !== 422) return null
  const raw = errorData(err)?.problems
  if (!Array.isArray(raw)) return null
  const out: PublishProblem[] = []
  for (const p of raw) {
    if (!p || typeof p !== 'object') continue
    const o = p as Record<string, unknown>
    const rule = typeof o.rule === 'string' && o.rule ? o.rule : 'unknown'
    const file = typeof o.file === 'string' ? o.file : ''
    const line = typeof o.line === 'number' && o.line > 0 ? Math.floor(o.line) : undefined
    const message = typeof o.message === 'string' && o.message ? o.message : rule
    out.push({ file, line, rule, message })
  }
  return out
}

/**
 * git 来源 manifest 的错误码(后端 6c28579 响应 JSON 的 code)-> 中文提示。
 */
export const GIT_SOURCE_READ_ONLY_MESSAGE = 'Git 来源的 Manifest 不能在线编辑'
export const GIT_SOURCE_BANNER = 'Git 来源：内容来自仓库，在此只读。发布时选择 commit。'

const GIT_ERROR_TEXT: Record<string, string> = {
  git_source_read_only: GIT_SOURCE_READ_ONLY_MESSAGE,
  git_source_disabled: '平台未配置 GitHub App，暂不能使用 Git 来源',
  git_repo_not_accessible: 'GitHub App 无法访问该仓库（请确认仓库属于已连接的 GitHub 账户，且 App 已授权该仓库）',
  git_commit_not_found: '仓库中找不到该 commit',
  git_subpath_not_found: '该 commit 中不存在配置的子目录',
  git_fetch_failed: '从仓库获取内容失败，请稍后重试',
  not_git_source: '该 Manifest 不是 Git 来源',
  github_installation_not_registered: '该 GitHub App 安装未登记到本组织，请联系组织管理员',
  // 后端 9db6171
  repo_query_too_long: '搜索关键字过长（最多 100 个字符）',
  git_subpath_invalid: '子路径不合法：不能包含 ..、不能以 / 开头、不能包含反斜杠',
  git_source_immutable: 'Git 来源创建后不可修改',
}

/** 错误响应中的 git 错误码;不是 git 错误时返回 '' */
export function gitErrorCode(err: unknown): string {
  const code = errorData(err)?.code
  return typeof code === 'string' && Object.prototype.hasOwnProperty.call(GIT_ERROR_TEXT, code) ? code : ''
}

/** git 错误的中文提示;不是已知 git 错误码时返回 null(调用方按原逻辑提示) */
export function gitErrorMessage(err: unknown): string | null {
  const code = gitErrorCode(err)
  return code ? GIT_ERROR_TEXT[code] : null
}

/** 409 git_source_read_only:git 来源 manifest 的草稿写入被拒 */
export function isGitSourceReadOnly(err: unknown): boolean {
  return getHttpStatus(err) === 409 && gitErrorCode(err) === 'git_source_read_only'
}
