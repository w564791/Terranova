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

/** 下拉 option 的后缀文案与 title(原生 select 无法放 Tag) */
export function bundleStatusLabel(v: BundleFields): { label: string; tooltip: string } | null {
  if (!versionNeedsRepublish(v)) return null
  if (isHashMismatch(v)) return { label: '完整性校验失败', tooltip: HASH_MISMATCH_TOOLTIP }
  return { label: '需重新发布', tooltip: v.bundle_invalid_reason || '该版本没有合法 bundle,请重新发布' }
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
  /** 仅密钥扫描(secret_scan:*)带 1-based 行号 */
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
