/**
 * Module / Demo 摘要 API client + 内存缓存
 *
 * 接 PR2-C-1 后端实现的:
 *   GET /api/v1/manifest-editor/modules
 *   GET /api/v1/manifest-editor/modules/:id/demos
 *
 * 编辑器 IntelliSense 频繁调用,这里做内存缓存:
 *  - modules 全量列表缓存 60s (足够 1 次编辑会话)
 *  - 每个 module 的 demos / inputs 按需拉取(ensureDemos / ensureInputs)并缓存,不预热
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

// ---- 按需加载(每个 module 首次被补全 / hover / inlay / quick fix 用到时才拉,结果按 module 缓存) ----
// 不再在打开编辑器时预热全部 module 的 demos/inputs(原来 2N+1 个请求)。
// 语义与原 getCached* 一致:已有非空缓存一直复用;空结果过期(TTL)后才重拉;并发调用共享同一请求。
const modulesInflight: { p: Promise<ModuleSummary[]> | null } = { p: null }
const demosInflight = new Map<number, Promise<DemoSummary[]>>()
const inputsInflight = new Map<number, Promise<ModuleInputField[]>>()

function usable<T>(c: { at: number; data: T[] } | null | undefined): c is { at: number; data: T[] } {
  return !!c && (c.data.length > 0 || isFresh(c.at))
}

/** module 列表:有缓存直接用,否则拉一次(并发去重) */
export function ensureModules(): Promise<ModuleSummary[]> {
  if (usable(modulesCache)) return Promise.resolve(modulesCache.data)
  if (!modulesInflight.p) {
    modulesInflight.p = fetchModules().finally(() => {
      modulesInflight.p = null
    })
  }
  return modulesInflight.p
}

/** 某 module 的 demos:有缓存直接用,否则拉一次(并发去重) */
export function ensureDemos(moduleId: number): Promise<DemoSummary[]> {
  const cached = demosCache.get(moduleId)
  if (usable(cached)) return Promise.resolve(cached.data)
  let p = demosInflight.get(moduleId)
  if (!p) {
    p = fetchDemos(moduleId).finally(() => demosInflight.delete(moduleId))
    demosInflight.set(moduleId, p)
  }
  return p
}

/** 某 module 的 inputs:有缓存直接用,否则拉一次(并发去重) */
export function ensureInputs(moduleId: number): Promise<ModuleInputField[]> {
  const cached = inputsCache.get(moduleId)
  if (usable(cached)) return Promise.resolve(cached.data)
  let p = inputsInflight.get(moduleId)
  if (!p) {
    p = fetchInputs(moduleId).finally(() => inputsInflight.delete(moduleId))
    inputsInflight.set(moduleId, p)
  }
  return p
}
