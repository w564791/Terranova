/**
 * 任务结构化失败码(workspace_tasks.error_code,后端 c9f030f / 0014d5f / 932b699)-> 中文提示。
 * 任务详情 / 列表据此显示提示;未知码不显示额外内容(仍有原 error_message)。
 *
 * - bundle_republish_required:manifest 版本没有合法 bundle;error_message 为
 *   "bundle_republish_required: <reason>",reason 为 `rule @ path; ...` 或 hash_mismatch。
 * - plan_expired:保存的 plan 超过保留期(PLAN_DATA_TTL)或已清理,apply 失败。
 * - agent_upgrade_required:池中没有支持 manifest 任务所需能力的 agent(error_reason = 缺少的能力)。
 * - bundle_hash_mismatch:执行端(agent / local)收到的 bundle 哈希与 bundle_hash 不符(error_reason = hash_mismatch);
 *   平台记审计并复核存储文件，仅当平台自己的校验失败时才把版本标记为失效。
 * - approval_hash_mismatch:manifest apply 前校验失败，要 apply 的内容不是审批过的内容
 *   (error_reason = not_approved / bundle_changed / plan_changed)。
 *
 * error_reason(932b699):与 error_code 并存的短 token(规则名 / 能力名,绝无路径或内容),
 * 如 denylisted_file、hash_mismatch、no_valid_bundle、plan_data_expired、manifest_bundle_v1。
 */
import { localizeBundleReason, ruleText } from '../pages/admin/ManifestEditorV2/bundleStatus'

export const TASK_ERROR_BUNDLE_REPUBLISH_REQUIRED = 'bundle_republish_required'
export const TASK_ERROR_AGENT_UPGRADE_REQUIRED = 'agent_upgrade_required'

/** 详情页 Alert 的引导操作 */
export type TaskErrorAction = 'upgrade_manifest' | 'rerun_plan'

interface TaskErrorCodeDef {
  /** 详情页 Alert 标题 */
  title: string
  /** 列表行小标签 */
  tag: string
  action?: TaskErrorAction
}

const TASK_ERROR_CODES: Record<string, TaskErrorCodeDef> = {
  bundle_republish_required: {
    title: '该任务使用的 Manifest 版本已失效，请升级到有效版本',
    tag: '版本已失效',
    action: 'upgrade_manifest',
  },
  plan_expired: {
    title: 'plan 已过期（超过保留期未 apply），请重新运行 plan',
    tag: 'plan 已过期',
    action: 'rerun_plan',
  },
  agent_upgrade_required: {
    title: '执行该任务的 agent 版本过旧，请升级 agent',
    tag: 'agent 过旧',
  },
  bundle_hash_mismatch: {
    title: '执行端收到的 bundle 完整性校验失败（平台已复核已发布文件）',
    tag: '完整性校验失败',
    action: 'upgrade_manifest',
  },
  approval_hash_mismatch: {
    title: '要 apply 的 bundle 或 plan 与审批时不一致，已拒绝 apply，请重新运行 plan 并审批',
    tag: '与审批不一致',
    action: 'rerun_plan',
  },
}

/** 非 bundle 规则的 error_reason token */
const REASON_TEXT: Record<string, string> = {
  hash_mismatch: '完整性校验失败（已发布内容与哈希不符）',
  no_valid_bundle: '该版本没有合法 bundle',
  invalid_bundle: '该版本 bundle 不合法',
  plan_data_expired: 'plan 数据已超过保留期',
  plan_data_missing: 'plan 数据已清理或缺失',
  not_approved: 'Manifest workspace 不能直接 apply，请先生成并审批 plan',
  bundle_changed: '审批后 Manifest 版本内容已变化，请重新 plan',
  plan_changed: 'plan 文件与审批时不一致，请重新 plan',
}

/** agent 能力(models.AgentCapability*)的可读说明 */
const AGENT_CAPABILITY_TEXT: Record<string, string> = {
  manifest_bundle_v1: '解包并校验 Manifest bundle',
  task_data_overrides_v1: '应用部署变量覆盖',
  agent_token_v1: '使用按 agent 签发的 token',
}

function has(map: Record<string, unknown>, k: string): boolean {
  return Object.prototype.hasOwnProperty.call(map, k)
}

/**
 * error_reason 的中文说明:agent_upgrade_required 下为"缺少能力：<name>(说明)";
 * 其余先查 bundle 规则(RULE_TEXT),再查通用 reason;未知 token 原样返回。空 => ''。
 */
export function taskErrorReasonText(code: string | null | undefined, reason: string | null | undefined): string {
  const r = (reason ?? '').trim()
  if (!r) return ''
  if (code === TASK_ERROR_AGENT_UPGRADE_REQUIRED || has(AGENT_CAPABILITY_TEXT, r)) {
    return has(AGENT_CAPABILITY_TEXT, r) ? `缺少能力：${r}（${AGENT_CAPABILITY_TEXT[r]}）` : `缺少能力：${r}`
  }
  return ruleText(r) ?? (has(REASON_TEXT, r) ? REASON_TEXT[r] : r)
}

export interface TaskErrorInfo extends TaskErrorCodeDef {
  code: string
  /** 原因(已尽量本地化):优先 error_reason,否则解析 error_message 的 "<code>: " 前缀;无则空串 */
  detail: string
  /** 原因原文(未翻译),用于悬停 / 对照 */
  rawDetail: string
}

/** 已知 error_code => 提示信息;未知 / 空 => null */
export function taskErrorInfo(
  code: string | null | undefined,
  errorMessage?: string | null,
  errorReason?: string | null,
): TaskErrorInfo | null {
  if (!code || !Object.prototype.hasOwnProperty.call(TASK_ERROR_CODES, code)) return null
  const def = TASK_ERROR_CODES[code]
  const reason = (errorReason ?? '').trim()
  if (reason) {
    return { ...def, code, detail: taskErrorReasonText(code, reason), rawDetail: reason }
  }
  // 回退:旧任务没有 error_reason,解析 message 前缀
  const msg = (errorMessage ?? '').trim()
  const prefix = `${code}:`
  const rawDetail = msg.startsWith(prefix) ? msg.slice(prefix.length).trim() : ''
  const detail = rawDetail && code === TASK_ERROR_BUNDLE_REPUBLISH_REQUIRED ? localizeBundleReason(rawDetail) : rawDetail
  return { ...def, code, detail, rawDetail }
}

// ===== 审批(confirm-apply)409 拒绝(后端 b69043a:{error, error_code, error_reason}) =====

export interface ApprovalErrorInfo {
  code: string
  reason: string
  message: string
  /** 需要重新 plan(提供 New run 入口) */
  rerunPlan: boolean
  /** 状态已变化(如已审批),应刷新任务 */
  refresh: boolean
}

const APPROVAL_ERROR_TEXT: Record<string, Omit<ApprovalErrorInfo, 'code' | 'reason'>> = {
  run_not_approvable: { message: '预览 plan 不能审批，请在 Deploy 面板生成待审批的 plan', rerunPlan: false, refresh: false },
  approval_run_required: { message: '该任务没有审批记录（升级前创建），请重新 plan', rerunPlan: true, refresh: false },
  run_not_active: { message: '该任务的审批记录已结束，请重新 plan', rerunPlan: true, refresh: true },
  already_approved: { message: '已经审批过', rerunPlan: false, refresh: true },
}

/**
 * confirm-apply 等审批接口的结构化拒绝 => 中文提示;不是已知码时返回 null(调用方按原逻辑提示)。
 * 读取 err.data(ApiError)或 err.response.data 的 error_code / code 与 error_reason / reason。
 */
export function approvalErrorInfo(err: unknown): ApprovalErrorInfo | null {
  if (!err || typeof err !== 'object') return null
  const e = err as { data?: unknown; response?: { data?: unknown } }
  const raw = e.data ?? e.response?.data
  if (!raw || typeof raw !== 'object') return null
  const d = raw as Record<string, unknown>
  const str = (v: unknown) => (typeof v === 'string' ? v : '')
  const code = str(d.error_code) || str(d.code)
  const reason = str(d.error_reason) || str(d.reason)
  if (!code) return null
  if (has(APPROVAL_ERROR_TEXT, code)) return { code, reason, ...APPROVAL_ERROR_TEXT[code] }
  if (code === 'approval_hash_mismatch') {
    const text = has(REASON_TEXT, reason) ? REASON_TEXT[reason] : TASK_ERROR_CODES.approval_hash_mismatch.title
    return { code, reason, message: text, rerunPlan: reason !== 'not_approved', refresh: false }
  }
  if (code === 'plan_expired') {
    return { code, reason, message: TASK_ERROR_CODES.plan_expired.title, rerunPlan: true, refresh: false }
  }
  if (code === TASK_ERROR_BUNDLE_REPUBLISH_REQUIRED) {
    const detail = reason ? taskErrorReasonText(code, reason) : ''
    return {
      code,
      reason,
      message: TASK_ERROR_CODES[code].title + (detail ? `（${detail}）` : ''),
      rerunPlan: false,
      refresh: false,
    }
  }
  return null
}
