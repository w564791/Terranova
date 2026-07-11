/**
 * 通用 HCL 自动补全 provider —— 对标 terraform-ls 的离线能力,纯前端实现。
 *
 * 分三层(参考 hashicorp/terraform-ls 架构):
 *   Tier 1  Core 关键字 + 块骨架 + 块内 meta 参数(静态表,无依赖)
 *   Tier 2  引用补全 var./local./module./data./resource(优先工作区索引,回退当前 buffer)
 *   Tier 3  平台 module 块内输入变量补全(用 /manifest-editor/modules/:id/inputs 缓存)
 *
 * 与 hclProviders.ts 的 demo provider 分工:那个管"插入平台 module 块/应用 demo",
 * 这个管"写原生 HCL 的通用智能"。两者注册在同一 'hcl' 语言上,候选项合并展示。
 */
import * as monaco from 'monaco-editor'
import { HCL_LANGUAGE_IDS } from './hclLanguage'
import { getCachedModules, getCachedInputs, fetchInputs, type ModuleInputField } from './moduleDemoApi'
import { symbolsFromIndex, type DefinitionIndex } from './hclDefinitions'

// HMR 会重算本模块、抹掉模块级变量,但 Monaco 全局 provider 注册表不会跟着重置。
// 把 disposable 存到 globalThis,重注册前先 dispose 旧的,避免 provider 叠加。
const REGISTRY_KEY = '__manifestHclCompletion__'

/** post_init 落库后由 API 注入的类型目录（非打包静态文件） */
type RuntimeProviderCatalog = {
  version: string
  resources: string[]
  data: string[]
  providers?: { source: string; version: string }[]
  providerVersionsKey?: string
  capturedAt?: string
}

const emptyCatalog = (): RuntimeProviderCatalog => ({
  version: '—',
  resources: [],
  data: [],
})

let runtimeCatalog: RuntimeProviderCatalog = emptyCatalog()

/** 编辑器打开 / subpath 切换后写入；供补全与状态栏使用 */
export function setProviderTypeCatalog(cat: {
  resources?: string[]
  data?: string[]
  contentHash?: string
  version?: string
  providers?: { source: string; version: string }[]
  providerVersionsKey?: string
  capturedAt?: string
  exists?: boolean
} | null): void {
  if (!cat || cat.exists === false) {
    runtimeCatalog = emptyCatalog()
    return
  }
  runtimeCatalog = {
    version: cat.contentHash || cat.version || '—',
    resources: cat.resources ?? [],
    data: cat.data ?? [],
    providers: cat.providers,
    providerVersionsKey: cat.providerVersionsKey,
    capturedAt: cat.capturedAt,
  }
}

// ---- Tier 1 静态数据 ----

const BLOCK_SNIPPETS: { keyword: string; detail: string; snippet: string }[] = [
  { keyword: 'resource', detail: 'resource block', snippet: 'resource "${1:type}" "${2:name}" {\n\t$0\n}' },
  { keyword: 'variable', detail: 'variable block', snippet: 'variable "${1:name}" {\n\ttype = ${2:string}\n\t$0\n}' },
  { keyword: 'output', detail: 'output block', snippet: 'output "${1:name}" {\n\tvalue = ${2:value}\n}' },
  { keyword: 'module', detail: 'module block', snippet: 'module "${1:name}" {\n\tsource = "${2:./path}"\n\t$0\n}' },
  { keyword: 'data', detail: 'data source block', snippet: 'data "${1:type}" "${2:name}" {\n\t$0\n}' },
  { keyword: 'locals', detail: 'locals block', snippet: 'locals {\n\t$0\n}' },
  { keyword: 'provider', detail: 'provider block', snippet: 'provider "${1:name}" {\n\t$0\n}' },
  { keyword: 'terraform', detail: 'terraform settings block', snippet: 'terraform {\n\t$0\n}' },
]

const META_ARGS: { label: string; snippet: string; detail: string }[] = [
  { label: 'count', snippet: 'count = ${1:1}', detail: 'meta-argument' },
  { label: 'for_each', snippet: 'for_each = ${1:toset([])}', detail: 'meta-argument' },
  { label: 'depends_on', snippet: 'depends_on = [${1}]', detail: 'meta-argument' },
  { label: 'provider', snippet: 'provider = ${1}', detail: 'meta-argument' },
  { label: 'lifecycle', snippet: 'lifecycle {\n\t$0\n}', detail: 'meta-argument block' },
  { label: 'dynamic', snippet: 'dynamic "${1:name}" {\n\tfor_each = ${2}\n\tcontent {\n\t\t$0\n\t}\n}', detail: 'dynamic block' },
]

const VARIABLE_FIELDS: { label: string; snippet: string }[] = [
  { label: 'type', snippet: 'type = ${1:string}' },
  { label: 'default', snippet: 'default = ${1}' },
  { label: 'description', snippet: 'description = "${1}"' },
  { label: 'sensitive', snippet: 'sensitive = ${1:true}' },
  { label: 'nullable', snippet: 'nullable = ${1:false}' },
  { label: 'validation', snippet: 'validation {\n\tcondition     = ${1}\n\terror_message = "${2}"\n}' },
]

/** 运行时 API 目录中的 type 名（无完整 attribute schema）。 */
function catalogTypes(kind: 'resource' | 'data'): string[] {
  return kind === 'resource' ? runtimeCatalog.resources : runtimeCatalog.data
}

/** 状态栏展示：content_hash 短码，无缓存时为 — */
export function getProviderSchemaVersion(): string {
  return runtimeCatalog.version || '—'
}

export function getProviderSchemaMeta(): RuntimeProviderCatalog {
  return runtimeCatalog
}

// Terraform 内置函数(表达式内补全)
const BUILTIN_FUNCS: { name: string; snippet: string; detail: string }[] = [
  { name: 'length', snippet: 'length(${1})', detail: 'collection length' },
  { name: 'lookup', snippet: 'lookup(${1:map}, ${2:key}, ${3:default})', detail: 'map lookup' },
  { name: 'merge', snippet: 'merge(${1:map1}, ${2:map2})', detail: 'merge maps' },
  { name: 'coalesce', snippet: 'coalesce(${1})', detail: 'first non-null' },
  { name: 'try', snippet: 'try(${1:expr}, ${2:fallback})', detail: 'try expression' },
  { name: 'can', snippet: 'can(${1:expr})', detail: 'can evaluate' },
  { name: 'tostring', snippet: 'tostring(${1})', detail: 'convert to string' },
  { name: 'tonumber', snippet: 'tonumber(${1})', detail: 'convert to number' },
  { name: 'tobool', snippet: 'tobool(${1})', detail: 'convert to bool' },
  { name: 'tolist', snippet: 'tolist(${1})', detail: 'convert to list' },
  { name: 'toset', snippet: 'toset(${1})', detail: 'convert to set' },
  { name: 'tomap', snippet: 'tomap(${1})', detail: 'convert to map' },
  { name: 'format', snippet: 'format("${1:%s}", ${2})', detail: 'sprintf format' },
  { name: 'join', snippet: 'join("${1:,}", ${2:list})', detail: 'join list' },
  { name: 'split', snippet: 'split("${1:,}", ${2:string})', detail: 'split string' },
  { name: 'replace', snippet: 'replace(${1:string}, "${2:substr}", "${3:replace}")', detail: 'replace' },
  { name: 'regex', snippet: 'regex("${1:pattern}", ${2:string})', detail: 'regex match' },
  { name: 'jsonencode', snippet: 'jsonencode(${1})', detail: 'to JSON string' },
  { name: 'jsondecode', snippet: 'jsondecode(${1})', detail: 'from JSON string' },
  { name: 'yamlencode', snippet: 'yamlencode(${1})', detail: 'to YAML string' },
  { name: 'yamldecode', snippet: 'yamldecode(${1})', detail: 'from YAML string' },
  { name: 'file', snippet: 'file("${1:path}")', detail: 'read file' },
  { name: 'templatefile', snippet: 'templatefile("${1:path}", ${2:{}})', detail: 'render template' },
  { name: 'cidrsubnet', snippet: 'cidrsubnet(${1:prefix}, ${2:newbits}, ${3:netnum})', detail: 'subnet CIDR' },
  { name: 'element', snippet: 'element(${1:list}, ${2:index})', detail: 'list element' },
  { name: 'concat', snippet: 'concat(${1:list1}, ${2:list2})', detail: 'concat lists' },
  { name: 'flatten', snippet: 'flatten(${1:list})', detail: 'flatten nested lists' },
  { name: 'keys', snippet: 'keys(${1:map})', detail: 'map keys' },
  { name: 'values', snippet: 'values(${1:map})', detail: 'map values' },
  { name: 'contains', snippet: 'contains(${1:list}, ${2:value})', detail: 'list contains' },
  { name: 'for_each', snippet: 'for_each', detail: 'meta-argument (use in block)' },
]

// ---- buffer 静态分析:提取已声明符号(Tier 2 回退) ----

interface DeclaredSymbols {
  variables: string[]
  locals: string[]
  modules: string[]
  resources: { type: string; name: string }[]
  data: { type: string; name: string }[]
}

function scanSymbols(text: string): DeclaredSymbols {
  const variables: string[] = []
  const locals: string[] = []
  const modules: string[] = []
  const resources: { type: string; name: string }[] = []
  const data: { type: string; name: string }[] = []

  const lines = text.split('\n')
  let inLocals = false
  let localsDepth = 0
  for (const line of lines) {
    const t = line.trim()
    let m = t.match(/^variable\s+"([^"]+)"\s*\{/)
    if (m) { variables.push(m[1]); continue }
    m = t.match(/^module\s+"([^"]+)"\s*\{/)
    if (m) { modules.push(m[1]); continue }
    m = t.match(/^resource\s+"([^"]+)"\s+"([^"]+)"\s*\{/)
    if (m) { resources.push({ type: m[1], name: m[2] }); continue }
    m = t.match(/^data\s+"([^"]+)"\s+"([^"]+)"\s*\{/)
    if (m) { data.push({ type: m[1], name: m[2] }); continue }
    if (/^locals\s*\{/.test(t)) { inLocals = true; localsDepth = 0 }
    if (inLocals) {
      localsDepth += (line.match(/\{/g)?.length ?? 0)
      localsDepth -= (line.match(/\}/g)?.length ?? 0)
      const lm = t.match(/^([A-Za-z_][A-Za-z0-9_-]*)\s*=/)
      if (lm && !/^locals$/.test(lm[1])) locals.push(lm[1])
      if (localsDepth <= 0 && /\}/.test(line)) inLocals = false
    }
  }
  return { variables, locals, modules, resources, data }
}

function uniqStrings(arr: string[]): string[] {
  return [...new Set(arr)]
}

function uniqResources(
  arr: { type: string; name: string }[],
): { type: string; name: string }[] {
  const seen = new Set<string>()
  const out: { type: string; name: string }[] = []
  for (const r of arr) {
    const k = `${r.type}.${r.name}`
    if (seen.has(k)) continue
    seen.add(k)
    out.push(r)
  }
  return out
}

/** 工作区索引 ∪ 当前 buffer(未保存/索引滞后时仍能补当前文件符号) */
function resolveSymbols(
  model: monaco.editor.ITextModel,
  getIndex?: () => DefinitionIndex | null | undefined,
): DeclaredSymbols {
  const fromBuf = scanSymbols(model.getValue())
  const idx = getIndex?.()
  if (!idx) return fromBuf
  const fromIdx = symbolsFromIndex(idx)
  return {
    variables: uniqStrings([...fromIdx.variables, ...fromBuf.variables]),
    locals: uniqStrings([...fromIdx.locals, ...fromBuf.locals]),
    modules: uniqStrings([...fromIdx.modules, ...fromBuf.modules]),
    resources: uniqResources([...fromIdx.resources, ...fromBuf.resources]),
    data: uniqResources([...fromIdx.data, ...fromBuf.data]),
  }
}

// ---- 块上下文判断 ----

interface BlockContext {
  kind: string
  moduleSource?: string
}

interface BlockHeader {
  kind: string
  /** 本行打开该 HCL 块的 `{` 在行内的 0-based 下标;无 `{` 则为 -1 */
  braceCol: number
}

/** 识别顶层 HCL 块头(resource/module/…)。不要求同行已有 `{`。 */
function matchBlockHeader(line: string): BlockHeader | null {
  const head = line.trim()
  let m =
    head.match(/^(resource|data)\s+"([^"]+)"\s+"([^"]+)"\s*\{?/) ||
    head.match(/^(variable|module|provider|output)\s+"([^"]+)"\s*\{?/) ||
    head.match(/^(locals|terraform)\s*\{?/)
  if (!m) return null
  // 用原行定位 `{`,避免 trim 后列偏移
  const braceCol = line.indexOf('{')
  return { kind: m[1], braceCol }
}

/**
 * 从文件开头扫到光标,用栈维护「当前在哪个 HCL 块内」。
 *
 * 旧实现从光标向上扫 + depth+=closes-opens,在「已闭合块之后」会把
 * 上面的 resource/module 头误判为仍在块内 → 块外补出 count/for_each/inputs,
 * 且顶层 resource/module 骨架被压制。
 */
function currentBlock(
  model: monaco.editor.ITextModel,
  lineNumber: number,
  column?: number,
): BlockContext | null {
  const curLine = model.getLineContent(lineNumber)
  const before = column != null ? curLine.slice(0, Math.max(0, column - 1)) : curLine

  // 行首正在敲块关键字 / resource "type → 一定算顶层
  if (/^\s*(resource|data|variable|module|output|locals|provider|terraform)[\w-]*\s*$/i.test(before)) {
    return null
  }
  if (/^\s*(resource|data)\s+"[^"]*$/.test(before)) {
    return null
  }
  if (/^\s*(variable|module|provider|output)\s+"[^"]*$/.test(before)) {
    return null
  }

  type Frame = { kind: string; openDepth: number; headerLine: number }
  const stack: Frame[] = []
  let depth = 0
  // 块头与 `{` 分行时: resource "x" "y" \n {
  let pending: { kind: string; headerLine: number } | null = null

  for (let ln = 1; ln <= lineNumber; ln++) {
    const line = model.getLineContent(ln)
    // 光标行只处理到光标前,后续字符尚未「发生」
    const limit =
      ln === lineNumber && column != null
        ? Math.max(0, Math.min(column - 1, line.length))
        : line.length
    const header = matchBlockHeader(line)
    if (header) {
      if (header.braceCol >= 0 && header.braceCol < limit) {
        // 同行有 `{`,在扫到该字符时压栈;这里先记下
      } else if (header.braceCol < 0) {
        pending = { kind: header.kind, headerLine: ln }
      }
    }

    let inLineComment = false
    let inString = false
    let escape = false

    for (let i = 0; i < limit; i++) {
      const c = line[i]
      const next = i + 1 < limit ? line[i + 1] : ''

      if (inLineComment) continue
      if (escape) {
        escape = false
        continue
      }
      if (inString) {
        if (c === '\\') {
          escape = true
          continue
        }
        if (c === '"') inString = false
        continue
      }
      if (c === '#' || (c === '/' && next === '/')) {
        inLineComment = true
        continue
      }
      if (c === '/' && next === '*') {
        const end = line.indexOf('*/', i + 2)
        if (end >= 0 && end < limit) {
          i = end + 1
          continue
        }
        inLineComment = true
        continue
      }
      if (c === '"') {
        inString = true
        continue
      }

      if (c === '{') {
        depth++
        if (header && header.braceCol === i) {
          stack.push({ kind: header.kind, openDepth: depth, headerLine: ln })
          pending = null
        } else if (pending) {
          stack.push({ kind: pending.kind, openDepth: depth, headerLine: pending.headerLine })
          pending = null
        }
      } else if (c === '}') {
        while (stack.length > 0 && stack[stack.length - 1].openDepth === depth) {
          stack.pop()
        }
        depth = Math.max(0, depth - 1)
      }
    }
  }

  if (stack.length === 0) return null
  const top = stack[stack.length - 1]
  if (top.kind === 'module') {
    return {
      kind: 'module',
      moduleSource: findModuleSource(model, top.headerLine),
    }
  }
  return { kind: top.kind }
}

function findModuleSource(model: monaco.editor.ITextModel, headLine: number): string | undefined {
  const total = model.getLineCount()
  let depth = 0
  for (let ln = headLine; ln <= total; ln++) {
    const line = model.getLineContent(ln)
    // 简单括号计数(够用);source 通常在块顶部
    const opens = (line.match(/\{/g)?.length ?? 0)
    const closes = (line.match(/\}/g)?.length ?? 0)
    if (ln === headLine) {
      depth = opens - closes
    } else {
      depth += opens - closes
    }
    const sm = line.match(/source\s*=\s*"([^"]+)"/)
    if (sm) return sm[1]
    if (ln > headLine && depth <= 0) break
  }
  return undefined
}

export interface RegisterCompletionOpts {
  /** 工作区符号索引;未提供时回退扫当前 buffer(兼容 MonacoHclEditor 单文件场景) */
  getIndex?: () => DefinitionIndex | null | undefined
}

export function registerHclCompletion(opts: RegisterCompletionOpts = {}): void {
  const g = globalThis as unknown as { [k: string]: monaco.IDisposable[] | undefined }
  const prev = g[REGISTRY_KEY]
  if (prev) prev.forEach((d) => { try { d.dispose() } catch { /* 已 dispose 或已注销 */ } })
  const disposables: monaco.IDisposable[] = []
  g[REGISTRY_KEY] = disposables

  for (const langId of HCL_LANGUAGE_IDS) {
    disposables.push(monaco.languages.registerCompletionItemProvider(langId, {
      // 不要用空格作 trigger:敲完 resource 后空格会再弹一次,且 atLineStart 已失效
      triggerCharacters: ['.', '"'],
      provideCompletionItems(model, position) {
        const lineContent = model.getLineContent(position.lineNumber)
        const before = lineContent.slice(0, position.column - 1)
        const word = model.getWordUntilPosition(position)
        const range: monaco.IRange = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn,
        }
        const suggestions: monaco.languages.CompletionItem[] = []
        const syms = resolveSymbols(model, opts.getIndex)

        // var.<here>
        if (/(^|[^\w.])var\.$/.test(before)) {
          syms.variables.forEach((v) => suggestions.push(refItem(v, 'variable', range)))
          return { suggestions }
        }
        if (/(^|[^\w.])local\.$/.test(before)) {
          syms.locals.forEach((v) => suggestions.push(refItem(v, 'local', range)))
          return { suggestions }
        }
        if (/(^|[^\w.])module\.$/.test(before)) {
          syms.modules.forEach((v) => suggestions.push(refItem(v, 'module', range)))
          return { suggestions }
        }
        if (/(^|[^\w.])data\.$/.test(before)) {
          ;[...new Set(syms.data.map((d) => d.type))].forEach((t) =>
            suggestions.push(refItem(t, 'data type', range)),
          )
          return { suggestions }
        }
        const dataNameMatch = before.match(/(^|[^\w.])data\.([A-Za-z0-9_-]+)\.$/)
        if (dataNameMatch) {
          const dtype = dataNameMatch[2]
          syms.data
            .filter((d) => d.type === dtype)
            .forEach((d) => suggestions.push(refItem(d.name, 'data source', range)))
          return { suggestions }
        }
        const resNameMatch = before.match(/(^|[^\w.])([a-z][a-z0-9]*_[a-z0-9_]+)\.$/)
        if (resNameMatch) {
          const rtype = resNameMatch[2]
          syms.resources
            .filter((r) => r.type === rtype)
            .forEach((r) => suggestions.push(refItem(r.name, 'resource', range)))
          if (suggestions.length) return { suggestions }
        }

        // resource "TYPE" / data "TYPE" 引号内 — 类型来自打包生成的 provider schema 目录
        const typeQuote = before.match(/^\s*(resource|data)\s+"([^"]*)$/)
        if (typeQuote) {
          const kind = typeQuote[1] as 'resource' | 'data'
          const typed = typeQuote[2].toLowerCase()
          const fromWs = new Set<string>()
          if (kind === 'resource') {
            for (const r of syms.resources) fromWs.add(r.type)
          } else {
            for (const d of syms.data) fromWs.add(d.type)
          }
          const fromCatalog = catalogTypes(kind)
          const allTypes = [...new Set([...fromWs, ...fromCatalog])]
            .filter((ty) => !typed || ty.toLowerCase().includes(typed))
            .sort((a, b) => a.localeCompare(b))
          // 前缀过滤后仍过多时截断,避免一次塞上万条卡 UI(完整列表靠继续输入缩小)
          const MAX_TYPE_SUGGESTIONS = 200
          const limited = allTypes.length > MAX_TYPE_SUGGESTIONS ? allTypes.slice(0, MAX_TYPE_SUGGESTIONS) : allTypes
          const qStart = before.lastIndexOf('"')
          const typeRange: monaco.IRange = {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: qStart + 2,
            endColumn: position.column,
          }
          limited.forEach((ty) => {
            suggestions.push({
              label: ty,
              kind: monaco.languages.CompletionItemKind.Class,
              detail: kind === 'resource' ? 'resource type' : 'data source type',
              insertText: ty,
              range: typeRange,
              sortText: (fromWs.has(ty) ? '0_' : '1_') + ty,
              filterText: ty,
            })
          })
          return { suggestions }
        }

        const block = currentBlock(model, position.lineNumber, position.column)
        const atAttrLine = /^\s*[\w-]*$/.test(before)
        const attrPrefix = before.trim().toLowerCase()

        // ===== 顶层:只出块骨架,绝不出 count/for_each/module inputs =====
        if (!block) {
          if (atAttrLine) {
            BLOCK_SNIPPETS.forEach((b) => {
              if (attrPrefix && !b.keyword.startsWith(attrPrefix)) return
              suggestions.push({
                label: b.keyword,
                kind: monaco.languages.CompletionItemKind.Keyword,
                detail: b.detail,
                insertText: b.snippet,
                insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
                range,
                sortText: '0_' + b.keyword,
                filterText: b.keyword,
                preselect: !!attrPrefix && b.keyword.startsWith(attrPrefix),
              })
            })
          }
          // 顶层表达式侧(极少见)仍允许 var./函数,但不给 meta-args
          const inExprTop = /(?:=|\[|,|\()\s*[\w.]*$/.test(before)
          const tokenPrefix = (before.match(/([A-Za-z_][\w]*)$/) || ['', ''])[1]
          if (inExprTop && !before.trimEnd().endsWith('.') && tokenPrefix.length >= 1) {
            const p = tokenPrefix.toLowerCase()
            for (const kw of ['var', 'local', 'module', 'data']) {
              if (!kw.startsWith(p)) continue
              suggestions.push({
                label: kw,
                kind: monaco.languages.CompletionItemKind.Keyword,
                insertText: kw + '.',
                command: { id: 'editor.action.triggerSuggest', title: '' },
                range,
                sortText: '2_' + kw,
                filterText: kw,
              })
            }
            BUILTIN_FUNCS.forEach((fn) => {
              if (!fn.name.startsWith(p)) return
              suggestions.push({
                label: fn.name,
                kind: monaco.languages.CompletionItemKind.Function,
                detail: fn.detail,
                insertText: fn.snippet,
                insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
                range,
                sortText: '4_' + fn.name,
                filterText: fn.name,
              })
            })
          }
          return { suggestions }
        }

        // ===== 块内:属性 / meta / 表达式引用;不出顶层 resource/module 骨架 =====
        if (atAttrLine) {
          if (block.kind === 'module') {
            if (!attrPrefix || 'source'.startsWith(attrPrefix)) {
              suggestions.push({
                label: 'source',
                kind: monaco.languages.CompletionItemKind.Property,
                detail: 'module source',
                insertText: 'source = "${1}"',
                insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
                command: { id: 'editor.action.triggerSuggest', title: '' },
                range,
                sortText: '0a_source',
                filterText: 'source',
              })
            }
            if (!attrPrefix || 'version'.startsWith(attrPrefix)) {
              suggestions.push({
                label: 'version',
                kind: monaco.languages.CompletionItemKind.Property,
                detail: 'module version',
                insertText: 'version = "${1}"',
                insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
                range,
                sortText: '0b_version',
                filterText: 'version',
              })
            }
            // 平台 module 输入参数（OpenAPI ModuleInput 扁平字段 + 类型；无条件过滤）
            if (block.moduleSource) {
              const mod = findModuleBySource(block.moduleSource)
              if (mod) {
                const inputs = getCachedInputs(mod.module_id)
                if (inputs.length === 0) {
                  // 异步拉完后重触发补全，避免首次打开块时 inputs 还是空
                  void fetchInputs(mod.module_id).then((list) => {
                    if (list.length > 0) retriggerSuggest()
                  })
                }
                inputs.forEach((inp) => {
                  if (attrPrefix && !inp.name.toLowerCase().startsWith(attrPrefix)) return
                  suggestions.push(inputItem(inp, range))
                })
              }
            }
          }

          if (block.kind === 'resource' || block.kind === 'data' || block.kind === 'module') {
            META_ARGS.forEach((a) => {
              if (attrPrefix && !a.label.startsWith(attrPrefix)) return
              suggestions.push({
                label: a.label,
                kind: monaco.languages.CompletionItemKind.Keyword,
                detail: a.detail,
                insertText: a.snippet,
                insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
                range,
                sortText: '1_' + a.label,
                filterText: a.label,
              })
            })
          } else if (block.kind === 'variable') {
            VARIABLE_FIELDS.forEach((f) => {
              if (attrPrefix && !f.label.startsWith(attrPrefix)) return
              suggestions.push({
                label: f.label,
                kind: monaco.languages.CompletionItemKind.Property,
                insertText: f.snippet,
                insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
                range,
                sortText: '1_' + f.label,
                filterText: f.label,
              })
            })
          }
        }

        // 块内表达式:var / local / 资源引用 / 函数
        const inExpr = /(?:=|\[|,|\()\s*[\w.]*$/.test(before)
        const tokenPrefix = (before.match(/([A-Za-z_][\w]*)$/) || ['', ''])[1]
        if (inExpr && !before.trimEnd().endsWith('.')) {
          const p = tokenPrefix.toLowerCase()
          for (const kw of ['var', 'local', 'module', 'data']) {
            if (p && !kw.startsWith(p)) continue
            suggestions.push({
              label: kw,
              kind: monaco.languages.CompletionItemKind.Keyword,
              insertText: kw + '.',
              command: { id: 'editor.action.triggerSuggest', title: '' },
              range,
              sortText: '2_' + kw,
              filterText: kw,
            })
          }
          syms.resources.forEach((r) => {
            const label = `${r.type}.${r.name}`
            if (tokenPrefix && !label.toLowerCase().includes(tokenPrefix.toLowerCase())) return
            suggestions.push({
              label,
              kind: monaco.languages.CompletionItemKind.Reference,
              insertText: label,
              detail: 'resource reference',
              range,
              sortText: '3_' + r.type,
              filterText: label,
            })
          })
          if (tokenPrefix.length >= 1) {
            BUILTIN_FUNCS.forEach((fn) => {
              if (!fn.name.startsWith(p)) return
              suggestions.push({
                label: fn.name,
                kind: monaco.languages.CompletionItemKind.Function,
                detail: fn.detail,
                insertText: fn.snippet,
                insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
                range,
                sortText: '4_' + fn.name,
                filterText: fn.name,
              })
            })
          }
        }

        return { suggestions }
      },
    }))
  }
}

function refItem(
  name: string,
  detail: string,
  range: monaco.IRange,
): monaco.languages.CompletionItem {
  return {
    label: name,
    kind: monaco.languages.CompletionItemKind.Variable,
    detail,
    insertText: name,
    range,
    sortText: '1_' + name,
  }
}

/** source 精确匹配；失败则后缀/basename 匹配（兼容 registry 全路径 vs 平台 source） */
function findModuleBySource(source: string) {
  const modules = getCachedModules()
  const exact = modules.find((m) => m.source === source)
  if (exact) return exact
  const base = source.split('/').filter(Boolean).pop() || source
  return (
    modules.find((m) => m.source === base) ||
    modules.find((m) => m.source.endsWith('/' + base) || m.source.endsWith(source)) ||
    modules.find((m) => source.endsWith('/' + m.source) || source.endsWith(m.source))
  )
}

function retriggerSuggest(): void {
  try {
    for (const ed of monaco.editor.getEditors()) {
      ed.trigger('hcl-module-inputs', 'editor.action.triggerSuggest', {})
    }
  } catch {
    /* ignore */
  }
}

function inputItem(inp: ModuleInputField, range: monaco.IRange): monaco.languages.CompletionItem {
  const typeLabel = inp.type_label || inp.type || 'any'
  const req = inp.required ? 'required' : 'optional'
  const valueSnippet = snippetForInput(inp)
  const docs: string[] = []
  if (inp.title && inp.title !== inp.name) docs.push(`**${inp.title}**`)
  if (inp.description) docs.push(inp.description)
  docs.push(`类型: \`${typeLabel}\``)
  if (inp.enum && inp.enum.length > 0) {
    docs.push('枚举: ' + inp.enum.map((e) => `\`${e}\``).join(', '))
  }
  if (inp.default) docs.push(`默认: \`${inp.default}\``)

  return {
    label: { label: inp.name, description: `${typeLabel} · ${req}` },
    kind: inp.required
      ? monaco.languages.CompletionItemKind.Field
      : monaco.languages.CompletionItemKind.Property,
    detail: `${typeLabel} · ${req}`,
    documentation: docs.length ? { value: docs.join('\n\n') } : undefined,
    insertText: `${inp.name} = ${valueSnippet}`,
    insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
    range,
    sortText: (inp.required ? '0_' : '1_') + inp.name,
    filterText: inp.name,
  }
}

/** 按 type_label / type / enum 生成 HCL 右侧 snippet */
function snippetForInput(inp: ModuleInputField): string {
  if (inp.enum && inp.enum.length > 0) {
    const first = inp.enum[0]
    // 字符串枚举加引号
    if (inp.type === 'string' || (!inp.type && Number.isNaN(Number(first)))) {
      return `"\${1|${inp.enum.join(',')}|}"`
    }
    return `\${1|${inp.enum.join(',')}|}`
  }

  const label = (inp.type_label || inp.type || '').toLowerCase()
  if (label === 'string' || label.startsWith('string')) return '"${1}"'
  if (label === 'number' || label === 'integer') return '${1:0}'
  if (label === 'bool' || label === 'boolean') return '${1:true}'
  if (label.startsWith('list(')) {
    // list(string) → ["${1}"]；list(object) → [{ $1 }]
    if (label.includes('string')) return '["${1}"]'
    if (label.includes('object') || label.includes('map')) return '[{\n\t$1\n}]'
    return '[${1}]'
  }
  if (label.startsWith('map(')) {
    if (label.includes('string')) return '{\n\t"${1:key}" = "${2:value}"\n}'
    return '{\n\t$1\n}'
  }
  if (label === 'object' || inp.type === 'object') return '{\n\t$1\n}'
  if (inp.type === 'array') return '[${1}]'
  if (inp.type === 'boolean') return '${1:true}'
  if (inp.type === 'number') return '${1:0}'
  if (inp.type === 'string') return '"${1}"'
  return '${1}'
}
