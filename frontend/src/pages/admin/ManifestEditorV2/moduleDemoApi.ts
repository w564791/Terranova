/**
 * Module / Demo 摘要 API client + 内存缓存
 *
 * 接 PR2-C-1 后端实现的:
 *   GET /api/v1/manifest-editor/modules
 *   GET /api/v1/manifest-editor/modules/:id/demos
 *
 * 编辑器 IntelliSense 频繁调用,这里做内存缓存:
 *  - modules 全量列表缓存 60s (足够 1 次编辑会话)
 *  - 每个 module 的 demos 也缓存
 *  - 不做磁盘缓存,刷新页面重拉
 */
import api from '../../../services/api'

export interface ModuleSummary {
  module_id: number
  name: string
  source: string
  latest_version: string
  description: string
  demo_count: number
}

export interface DemoSummary {
  demo_id: number
  name: string
  description: string
  is_default: boolean
  config_data: Record<string, unknown>
  change_summary: string
}

// module 输入变量定义(Tier3 属性补全用),来自 /manifest-editor/modules/:id/inputs
// 扁平参数+类型；不做 OpenAPI 条件分支。
export interface ModuleInputField {
  name: string
  /** OpenAPI base type: string | number | boolean | object | array | any */
  type: string
  /** 展示/snippet: string, bool, number, list(string), map(string), object… */
  type_label?: string
  required: boolean
  description: string
  default?: string
  enum?: string[]
  title?: string
}

const CACHE_TTL_MS = 60_000
let modulesCache: { at: number; data: ModuleSummary[] } | null = null
const demosCache = new Map<number, { at: number; data: DemoSummary[] }>()
const inputsCache = new Map<number, { at: number; data: ModuleInputField[] }>()

function isFresh(at: number) {
  return Date.now() - at < CACHE_TTL_MS
}

export async function fetchModules(): Promise<ModuleSummary[]> {
  if (modulesCache && isFresh(modulesCache.at)) {
    return modulesCache.data
  }
  try {
    const data = (await api.get('/manifest-editor/modules')) as { modules?: ModuleSummary[] }
    const modules = data.modules ?? []
    modulesCache = { at: Date.now(), data: modules }
    return modules
  } catch {
    // 没有 module 不影响编辑器使用,静默返空
    modulesCache = { at: Date.now(), data: [] }
    return []
  }
}

export async function fetchDemos(moduleId: number): Promise<DemoSummary[]> {
  const cached = demosCache.get(moduleId)
  if (cached && isFresh(cached.at)) {
    return cached.data
  }
  try {
    const data = (await api.get(`/manifest-editor/modules/${moduleId}/demos`)) as {
      demos?: DemoSummary[]
    }
    const demos = data.demos ?? []
    demosCache.set(moduleId, { at: Date.now(), data: demos })
    return demos
  } catch {
    demosCache.set(moduleId, { at: Date.now(), data: [] })
    return []
  }
}

export async function fetchInputs(moduleId: number): Promise<ModuleInputField[]> {
  const cached = inputsCache.get(moduleId)
  if (cached && isFresh(cached.at)) {
    return cached.data
  }
  try {
    const data = (await api.get(`/manifest-editor/modules/${moduleId}/inputs`)) as {
      inputs?: ModuleInputField[]
    }
    const inputs = data.inputs ?? []
    inputsCache.set(moduleId, { at: Date.now(), data: inputs })
    return inputs
  } catch {
    inputsCache.set(moduleId, { at: Date.now(), data: [] })
    return []
  }
}

/** 同步访问已缓存数据(provider 用,避免每次 await) */
export function getCachedModules(): ModuleSummary[] {
  return modulesCache?.data ?? []
}

export function getCachedDemos(moduleId: number): DemoSummary[] {
  return demosCache.get(moduleId)?.data ?? []
}

export function getCachedInputs(moduleId: number): ModuleInputField[] {
  return inputsCache.get(moduleId)?.data ?? []
}

/** 后台预热: 进入编辑器时调一次,把 modules + 每个 module 的 demos/inputs 都拉好 */
export async function warmUpCache(): Promise<void> {
  const modules = await fetchModules()
  await Promise.all(
    modules.flatMap((m) => [fetchDemos(m.module_id), fetchInputs(m.module_id)]),
  )
}
