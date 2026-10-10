/**
 * 任务结构化失败码(workspace_tasks.error_code,后端 c9f030f)-> 中文提示。
 * 任务详情 / 列表据此显示提示;未知码不显示额外内容(仍有原 error_message)。
 *
 * - bundle_republish_required:manifest 版本没有合法 bundle;error_message 为
 *   "bundle_republish_required: <reason>",reason 为 `rule @ path; ...` 或 hash_mismatch。
 * - agent_upgrade_required / bundle_hash_mismatch:后端后续提供,先备好文案。
 */
import { localizeBundleReason } from '../pages/admin/ManifestEditorV2/bundleStatus'

export const TASK_ERROR_BUNDLE_REPUBLISH_REQUIRED = 'bundle_republish_required'

interface TaskErrorCodeDef {
  /** 详情页 Alert 标题 */
  title: string
  /** 列表行小标签 */
  tag: string
  /** 需要引导去 manifest 升级部署 */
  upgradeManifest?: boolean
}

const TASK_ERROR_CODES: Record<string, TaskErrorCodeDef> = {
  bundle_republish_required: {
    title: '该任务使用的 Manifest 版本已失效，请升级到有效版本',
    tag: '版本已失效',
    upgradeManifest: true,
  },
  agent_upgrade_required: {
    title: '执行该任务的 agent 版本过旧，请升级 agent',
    tag: 'agent 过旧',
  },
  bundle_hash_mismatch: {
    title: 'bundle 完整性校验失败',
    tag: '完整性校验失败',
    upgradeManifest: true,
  },
}

export interface TaskErrorInfo extends TaskErrorCodeDef {
  code: string
  /** error_message 中 "<code>: " 之后的原因(已尽量本地化);无则为空串 */
  detail: string
  /** 原因原文(未翻译),用于悬停 / 对照 */
  rawDetail: string
}

/** 已知 error_code => 提示信息;未知 / 空 => null */
export function taskErrorInfo(code: string | null | undefined, errorMessage?: string | null): TaskErrorInfo | null {
  if (!code || !Object.prototype.hasOwnProperty.call(TASK_ERROR_CODES, code)) return null
  const def = TASK_ERROR_CODES[code]
  const msg = (errorMessage ?? '').trim()
  const prefix = `${code}:`
  const rawDetail = msg.startsWith(prefix) ? msg.slice(prefix.length).trim() : ''
  const detail = rawDetail && code === TASK_ERROR_BUNDLE_REPUBLISH_REQUIRED ? localizeBundleReason(rawDetail) : rawDetail
  return { ...def, code, detail, rawDetail }
}
