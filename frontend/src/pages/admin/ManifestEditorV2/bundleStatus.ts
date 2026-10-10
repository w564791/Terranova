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
