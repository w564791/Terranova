/**
 * 通用 HCL 自动补全 provider —— 对标 terraform-ls 的离线能力,纯前端实现。
 *
 * 分三层(参考 hashicorp/terraform-ls 架构):
 *   Tier 1  Core 关键字 + 块骨架 + 块内 meta 参数(静态表,无依赖)
 *   Tier 2  引用补全 var./local./module.x./<type>.<name>/data.x.y(扫当前 buffer)
 *   Tier 3  平台 module 块内输入变量补全(用 /manifest-editor/modules/:id/inputs 缓存)
 *
 * 与 hclProviders.ts 的 demo provider 分工:那个管"插入平台 module 块/应用 demo",
 * 这个管"写原生 HCL 的通用智能"。两者注册在同一 'hcl' 语言上,候选项合并展示。
 */
import * as monaco from 'monaco-editor'
import { getCachedModules, getCachedInputs, fetchInputs, type ModuleInputField } from './moduleDemoApi'

// HMR 会重算本模块、抹掉模块级变量,但 Monaco 全局 provider 注册表不会跟着重置。
// 把 disposable 存到 globalThis,重注册前先 dispose 旧的,避免 provider 叠加。
// (与 hclProviders.ts 同一思路;详见 initServices.ts 对 HMR 的说明。)
const REGISTRY_KEY = '__manifestHclCompletion__'

// ---- Tier 1 静态数据 ----

// 顶层块骨架。${N:..} 是 monaco snippet placeholder。
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

// 块内 meta 参数(resource/data/module 通用的一组)
const META_ARGS: { label: string; snippet: string; detail: string }[] = [
  { label: 'count', snippet: 'count = ${1:1}', detail: 'meta-argument' },
  { label: 'for_each', snippet: 'for_each = ${1:toset([])}', detail: 'meta-argument' },
  { label: 'depends_on', snippet: 'depends_on = [${1}]', detail: 'meta-argument' },
  { label: 'provider', snippet: 'provider = ${1}', detail: 'meta-argument' },
  { label: 'lifecycle', snippet: 'lifecycle {\n\t$0\n}', detail: 'meta-argument block' },
  { label: 'dynamic', snippet: 'dynamic "${1:name}" {\n\tfor_each = ${2}\n\tcontent {\n\t\t$0\n\t}\n}', detail: 'dynamic block' },
]

// variable 块内常用字段
const VARIABLE_FIELDS: { label: string; snippet: string }[] = [
  { label: 'type', snippet: 'type = ${1:string}' },
  { label: 'default', snippet: 'default = ${1}' },
  { label: 'description', snippet: 'description = "${1}"' },
  { label: 'sensitive', snippet: 'sensitive = ${1:true}' },
  { label: 'nullable', snippet: 'nullable = ${1:false}' },
  { label: 'validation', snippet: 'validation {\n\tcondition     = ${1}\n\terror_message = "${2}"\n}' },
]

// ---- buffer 静态分析:提取已声明符号(Tier 2) ----

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
    // variable "x" {
    let m = t.match(/^variable\s+"([^"]+)"\s*\{/)
    if (m) { variables.push(m[1]); continue }
    // module "x" {
    m = t.match(/^module\s+"([^"]+)"\s*\{/)
    if (m) { modules.push(m[1]); continue }
    // resource "type" "name" {
    m = t.match(/^resource\s+"([^"]+)"\s+"([^"]+)"\s*\{/)
    if (m) { resources.push({ type: m[1], name: m[2] }); continue }
    // data "type" "name" {
    m = t.match(/^data\s+"([^"]+)"\s+"([^"]+)"\s*\{/)
    if (m) { data.push({ type: m[1], name: m[2] }); continue }
    // locals { ... key = ... }
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

// ---- 块上下文判断 ----

interface BlockContext {
  kind: string // resource / variable / module / data / output / locals / provider / terraform / ''
  // module 专用: 该块的 source 值(用于 Tier3 匹配平台 module)
  moduleSource?: string
}

// 从光标位置向上扫,找最近的未闭合块头,判断当前在哪种块内
function currentBlock(model: monaco.editor.ITextModel, lineNumber: number): BlockContext | null {
  let depth = 0
  for (let ln = lineNumber; ln >= 1; ln--) {
    const line = model.getLineContent(ln)
    // 统计括号(粗略,够用):光标行只看到光标前的部分
    const closes = (line.match(/\}/g)?.length ?? 0)
    const opens = (line.match(/\{/g)?.length ?? 0)
    if (ln < lineNumber) depth += closes - opens
    // 块头匹配
    const head = line.trim()
    const m = head.match(/^(resource|data)\s+"([^"]+)"\s+"([^"]+)"\s*\{/)
      || head.match(/^(variable|module|provider|output)\s+"([^"]+)"\s*\{/)
      || head.match(/^(locals|terraform)\s*\{/)
    if (m && depth <= 0) {
      const kind = m[1]
      if (kind === 'module') {
        return { kind, moduleSource: findModuleSource(model, ln) }
      }
      return { kind }
    }
  }
  return null
}

// 从 module 块头行向下找 source = "..."
function findModuleSource(model: monaco.editor.ITextModel, headLine: number): string | undefined {
  const total = model.getLineCount()
  let depth = 0
  for (let ln = headLine; ln <= total; ln++) {
    const line = model.getLineContent(ln)
    depth += (line.match(/\{/g)?.length ?? 0)
    depth -= (line.match(/\}/g)?.length ?? 0)
    const sm = line.match(/source\s*=\s*"([^"]+)"/)
    if (sm) return sm[1]
    if (ln > headLine && depth <= 0) break
  }
  return undefined
}

export function registerHclCompletion(): void {
  const g = globalThis as unknown as { [k: string]: monaco.IDisposable[] | undefined }
  const prev = g[REGISTRY_KEY]
  if (prev) prev.forEach((d) => { try { d.dispose() } catch { /* 已 dispose 或已注销 */ } })
  const disposables: monaco.IDisposable[] = []
  g[REGISTRY_KEY] = disposables

  disposables.push(monaco.languages.registerCompletionItemProvider('hcl', {
    triggerCharacters: ['.'],
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

      // ===== Tier 2: 引用补全(优先级最高,有明确前缀触发)=====
      // var. / local. / module.x. / data.type.name / <restype>.<name>
      const refMatch = before.match(/([A-Za-z_][\w.-]*)\.?$/)
      const syms = scanSymbols(model.getValue())

      // var.<here>
      if (/(^|[^\w.])var\.$/.test(before)) {
        syms.variables.forEach((v) =>
          suggestions.push(refItem(v, 'variable', range, position)),
        )
        return { suggestions }
      }
      // local.<here>
      if (/(^|[^\w.])local\.$/.test(before)) {
        syms.locals.forEach((v) => suggestions.push(refItem(v, 'local', range, position)))
        return { suggestions }
      }
      // module.<here>
      if (/(^|[^\w.])module\.$/.test(before)) {
        syms.modules.forEach((v) => suggestions.push(refItem(v, 'module output', range, position)))
        return { suggestions }
      }
      // data.<here>  → data 源类型
      if (/(^|[^\w.])data\.$/.test(before)) {
        const types = [...new Set(syms.data.map((d) => d.type))]
        types.forEach((t) => suggestions.push(refItem(t, 'data type', range, position)))
        return { suggestions }
      }
      // data.type.<here>  → 该类型下的 name
      const dataNameMatch = before.match(/(^|[^\w.])data\.([A-Za-z0-9_-]+)\.$/)
      if (dataNameMatch) {
        const dtype = dataNameMatch[2]
        syms.data.filter((d) => d.type === dtype).forEach((d) =>
          suggestions.push(refItem(d.name, 'data source', range, position)),
        )
        return { suggestions }
      }

      // 顶层提示符:var / local / module / data / 资源类型(裸引用起始)
      // 仅当看起来在写表达式(= 后,或 [ 内)时给,避免污染块头
      const inExpr = /[=\[,(]\s*[\w.]*$/.test(before)
      if (inExpr && !before.trimEnd().endsWith('.')) {
        for (const kw of ['var', 'local', 'module', 'data']) {
          suggestions.push({
            label: kw,
            kind: monaco.languages.CompletionItemKind.Keyword,
            insertText: kw + '.',
            command: { id: 'editor.action.triggerSuggest', title: '' },
            range,
            sortText: '2_' + kw,
          })
        }
        // 资源引用 <type>.<name>
        syms.resources.forEach((r) =>
          suggestions.push({
            label: `${r.type}.${r.name}`,
            kind: monaco.languages.CompletionItemKind.Reference,
            insertText: `${r.type}.${r.name}`,
            detail: 'resource reference',
            range,
            sortText: '3_' + r.type,
          }),
        )
      }

      // ===== 块上下文 =====
      const block = currentBlock(model, position.lineNumber)

      // ===== Tier 3: 平台 module 块内输入变量补全 =====
      if (block?.kind === 'module' && block.moduleSource) {
        const mod = getCachedModules().find((m) => m.source === block.moduleSource)
        if (mod) {
          let inputs = getCachedInputs(mod.module_id)
          if (inputs.length === 0) void fetchInputs(mod.module_id) // 后台拉,下次生效
          // 只在"属性名位置"(行首裸 token,非 = 右侧)给输入变量
          const atAttrName = /^\s*[\w-]*$/.test(before)
          if (atAttrName) {
            inputs.forEach((inp) => suggestions.push(inputItem(inp, range)))
          }
        }
      }

      // ===== Tier 1: 关键字 / 骨架 / meta 参数 =====
      const atLineStart = /^\s*[\w-]*$/.test(before)
      if (atLineStart && !refMatch?.[0].includes('.')) {
        if (!block) {
          // 顶层 → 块骨架
          BLOCK_SNIPPETS.forEach((b) =>
            suggestions.push({
              label: b.keyword,
              kind: monaco.languages.CompletionItemKind.Snippet,
              detail: b.detail,
              insertText: b.snippet,
              insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
              range,
              sortText: '5_' + b.keyword,
            }),
          )
        } else if (block.kind === 'resource' || block.kind === 'data' || block.kind === 'module') {
          // module 块固有字段 source / version(排在 meta 参数前)
          if (block.kind === 'module') {
            suggestions.push({
              label: 'source',
              kind: monaco.languages.CompletionItemKind.Property,
              detail: 'module source',
              insertText: 'source = "${1}"',
              insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
              // 插入后立刻触发建议,弹出 source 引号内的平台 module 列表
              command: { id: 'editor.action.triggerSuggest', title: '' },
              range,
              sortText: '5a_source',
            })
            suggestions.push({
              label: 'version',
              kind: monaco.languages.CompletionItemKind.Property,
              detail: 'module version',
              insertText: 'version = "${1}"',
              insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
              range,
              sortText: '5b_version',
            })
          }
          // 资源/数据/模块块内 → meta 参数
          META_ARGS.forEach((a) =>
            suggestions.push({
              label: a.label,
              kind: monaco.languages.CompletionItemKind.Keyword,
              detail: a.detail,
              insertText: a.snippet,
              insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
              range,
              sortText: '6_' + a.label,
            }),
          )
        } else if (block.kind === 'variable') {
          VARIABLE_FIELDS.forEach((f) =>
            suggestions.push({
              label: f.label,
              kind: monaco.languages.CompletionItemKind.Property,
              insertText: f.snippet,
              insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
              range,
              sortText: '6_' + f.label,
            }),
          )
        }
      }

      return { suggestions }
    },
  }))
}

function refItem(
  name: string,
  detail: string,
  range: monaco.IRange,
  _position: monaco.Position,
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

function inputItem(inp: ModuleInputField, range: monaco.IRange): monaco.languages.CompletionItem {
  const valueSnippet = snippetForType(inp.type)
  return {
    label: { label: inp.name, description: `${inp.type}${inp.required ? ' · required' : ''}` },
    kind: inp.required
      ? monaco.languages.CompletionItemKind.Field
      : monaco.languages.CompletionItemKind.Property,
    detail: inp.required ? 'required input' : 'optional input',
    documentation: inp.description
      ? { value: `${inp.description}${inp.default ? `\n\n默认: \`${inp.default}\`` : ''}` }
      : undefined,
    insertText: `${inp.name} = ${valueSnippet}`,
    insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
    range,
    // required 排前面
    sortText: (inp.required ? '0_' : '1_') + inp.name,
  }
}

function snippetForType(t: string): string {
  switch (t) {
    case 'string':
      return '"${1}"'
    case 'number':
      return '${1:0}'
    case 'boolean':
      return '${1:true}'
    case 'array':
      return '[${1}]'
    case 'object':
      return '{\n\t$1\n}'
    default:
      return '${1}'
  }
}
