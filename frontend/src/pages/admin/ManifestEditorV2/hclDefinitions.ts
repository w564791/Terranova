/**
 * HCL 工作区符号索引 + 转到定义 / Hover。
 *
 * 提供:
 *  - 稳定 model URI 工具(manifest:/<path> ↔ path),让跨文件跳转能反解目标文件
 *  - 跨文件定义索引:variable / locals / module / resource / data / output
 *  - 引用解析(var. / local. / module. / data.T.N / TYPE.NAME)
 *  - DefinitionProvider(Cmd+Click 跳转)+ HoverProvider(提示可跳转)
 *
 * 跨文件机制:DefinitionProvider 返回带 manifest URI 的 Location;目标非当前 model 时,
 * standalone editor 走 registerEditorOpener(在 ManifestEditorV2 注册)委托到 openFile。
 */
import * as monaco from 'monaco-editor'
import { HCL_LANGUAGE_IDS } from './hclLanguage'

const MANIFEST_SCHEME = 'manifest'

// path(无前导 /,如 "modules/vpc/main.tf")→ 稳定 model URI
export function pathToManifestUri(path: string): monaco.Uri {
  return monaco.Uri.from({ scheme: MANIFEST_SCHEME, path: '/' + path.replace(/^\/+/, '') })
}

// manifest URI → path(去前导 /)。非本 scheme 返回 null。
export function manifestUriToPath(uri: monaco.Uri): string | null {
  if (uri.scheme !== MANIFEST_SCHEME) return null
  return uri.path.replace(/^\/+/, '')
}

// ---- 定义索引 ----

export interface DefLoc {
  path: string
  line: number // 1-based
  column: number // 1-based,NAME 起始列
  endColumn: number // 1-based,NAME 结束列(exclusive+1,monaco 习惯)
}

/** resource 地址键: "aws_instance.web" ; data 键: "aws_ami.ubuntu" */
export interface DefinitionIndex {
  variables: Map<string, DefLoc>
  locals: Map<string, DefLoc>
  modules: Map<string, DefLoc>
  resources: Map<string, DefLoc> // type.name
  data: Map<string, DefLoc> // type.name
  outputs: Map<string, DefLoc>
}

export function emptyIndex(): DefinitionIndex {
  return {
    variables: new Map(),
    locals: new Map(),
    modules: new Map(),
    resources: new Map(),
    data: new Map(),
    outputs: new Map(),
  }
}

function setIfAbsent(map: Map<string, DefLoc>, key: string, loc: DefLoc): void {
  if (!map.has(key)) map.set(key, loc)
}

function nameColInQuotes(line: string, name: string, occurrence = 0): { column: number; endColumn: number } {
  let from = 0
  for (let i = 0; i <= occurrence; i++) {
    const q = line.indexOf('"' + name + '"', from)
    if (q < 0) {
      const fallback = line.indexOf(name)
      const col = fallback >= 0 ? fallback + 1 : 1
      return { column: col, endColumn: col + name.length }
    }
    if (i === occurrence) {
      const col = q + 2 // 1-based,引号后
      return { column: col, endColumn: col + name.length }
    }
    from = q + 1
  }
  return { column: 1, endColumn: 1 + name.length }
}

// 扫单个文件,把其中的定义并入 index(同名保留首个)。
// 只处理 .tf;其他扩展名跳过。
export function indexFile(index: DefinitionIndex, path: string, content: string): void {
  if (!path.endsWith('.tf')) return
  const lines = content.split('\n')
  let inLocals = false
  let localsDepth = 0
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const trimmed = line.trim()
    const lineNo = i + 1

    // variable "X" {
    const vm = line.match(/^\s*variable\s+"([^"]+)"/)
    if (vm) {
      const name = vm[1]
      const { column, endColumn } = nameColInQuotes(line, name)
      setIfAbsent(index.variables, name, { path, line: lineNo, column, endColumn })
      continue
    }

    // output "X" {
    const om = line.match(/^\s*output\s+"([^"]+)"/)
    if (om) {
      const name = om[1]
      const { column, endColumn } = nameColInQuotes(line, name)
      setIfAbsent(index.outputs, name, { path, line: lineNo, column, endColumn })
      continue
    }

    // module "X" {
    const mm = line.match(/^\s*module\s+"([^"]+)"/)
    if (mm) {
      const name = mm[1]
      const { column, endColumn } = nameColInQuotes(line, name)
      setIfAbsent(index.modules, name, { path, line: lineNo, column, endColumn })
      continue
    }

    // data "TYPE" "NAME" {
    const dm = line.match(/^\s*data\s+"([^"]+)"\s+"([^"]+)"/)
    if (dm) {
      const type = dm[1]
      const name = dm[2]
      const { column, endColumn } = nameColInQuotes(line, name, 1)
      setIfAbsent(index.data, `${type}.${name}`, { path, line: lineNo, column, endColumn })
      continue
    }

    // resource "TYPE" "NAME" {
    const rm = line.match(/^\s*resource\s+"([^"]+)"\s+"([^"]+)"/)
    if (rm) {
      const type = rm[1]
      const name = rm[2]
      const { column, endColumn } = nameColInQuotes(line, name, 1)
      setIfAbsent(index.resources, `${type}.${name}`, { path, line: lineNo, column, endColumn })
      continue
    }

    // locals { ... } 块跟踪(深度,支持嵌套对象)
    if (/^locals\s*\{/.test(trimmed)) {
      const inner = trimmed.replace(/^locals\s*\{/, '')
      if (inner.includes('}')) {
        // 单行 locals { a = 1, b = 2 }
        const body = inner.slice(0, inner.indexOf('}'))
        for (const m of body.matchAll(/(?:^|,)\s*([A-Za-z_][A-Za-z0-9_-]*)\s*=/g)) {
          const name = m[1]
          const col = line.indexOf(name) + 1
          if (col > 0) {
            setIfAbsent(index.locals, name, {
              path,
              line: lineNo,
              column: col,
              endColumn: col + name.length,
            })
          }
        }
        // 单行闭合,不进入多行状态
      } else {
        inLocals = true
        localsDepth = 0
      }
    }
    if (inLocals) {
      localsDepth += line.match(/\{/g)?.length ?? 0
      localsDepth -= line.match(/\}/g)?.length ?? 0
      // 只认顶层 key =(深度为 1 时,即 locals{} 直接子级)
      if (localsDepth === 1) {
        const lm = trimmed.match(/^([A-Za-z_][A-Za-z0-9_-]*)\s*=/)
        if (lm && lm[1] !== 'locals') {
          const name = lm[1]
          const col = line.indexOf(name) + 1
          if (col > 0) {
            setIfAbsent(index.locals, name, {
              path,
              line: lineNo,
              column: col,
              endColumn: col + name.length,
            })
          }
        }
      }
      if (localsDepth <= 0 && /\}/.test(line)) inLocals = false
    }
  }
}

// 全量构建:从一组 (path, content) 重建索引。
export function buildDefinitionIndex(files: { path: string; content: string }[]): DefinitionIndex {
  const index = emptyIndex()
  for (const f of files) indexFile(index, f.path, f.content)
  return index
}

// 从 index 中删除某 path 贡献的所有条目(增量更新前先清,再重扫该文件)。
export function removePathFromIndex(index: DefinitionIndex, path: string): void {
  for (const map of [
    index.variables,
    index.locals,
    index.modules,
    index.resources,
    index.data,
    index.outputs,
  ]) {
    for (const [name, loc] of map) {
      if (loc.path === path) map.delete(name)
    }
  }
}

/** 从索引导出声明符号(补全用,跨文件)。 */
export function symbolsFromIndex(index: DefinitionIndex): {
  variables: string[]
  locals: string[]
  modules: string[]
  resources: { type: string; name: string }[]
  data: { type: string; name: string }[]
  outputs: string[]
} {
  const resources: { type: string; name: string }[] = []
  for (const key of index.resources.keys()) {
    const dot = key.indexOf('.')
    if (dot > 0) resources.push({ type: key.slice(0, dot), name: key.slice(dot + 1) })
  }
  const data: { type: string; name: string }[] = []
  for (const key of index.data.keys()) {
    const dot = key.indexOf('.')
    if (dot > 0) data.push({ type: key.slice(0, dot), name: key.slice(dot + 1) })
  }
  return {
    variables: [...index.variables.keys()],
    locals: [...index.locals.keys()],
    modules: [...index.modules.keys()],
    resources,
    data,
    outputs: [...index.outputs.keys()],
  }
}

// ---- 引用解析 ----

export type RefKind = 'var' | 'local' | 'module' | 'data' | 'resource'

export interface RefHit {
  kind: RefKind
  /** 查找键: var/local/module 为名字; data/resource 为 type.name */
  name: string
  // 引用 token 在行内的范围,用于 hover 高亮
  range: monaco.IRange
}

interface Candidate {
  kind: RefKind
  name: string
  start: number // 1-based
  end: number // 1-based exclusive
}

// 判断光标是否落在 HCL 引用上。var.a.b / module.x.out 取定义段。
export function resolveReferenceAt(
  model: monaco.editor.ITextModel,
  position: monaco.Position,
): RefHit | null {
  const line = model.getLineContent(position.lineNumber)
  const col = position.column // 1-based
  const candidates: Candidate[] = []

  // 定义行本身不当作引用(避免 variable "x" 里的 x 被当成 var.x)
  const trimmed = line.trim()
  if (/^(variable|output|module|resource|data|locals)\b/.test(trimmed)) {
    return null
  }

  // var.xxx / local.xxx
  {
    const re = /(^|[^\w.])(var|local)\.([A-Za-z_][A-Za-z0-9_-]*)/g
    let m: RegExpExecArray | null
    while ((m = re.exec(line)) !== null) {
      const prefixLen = m[1].length
      const kindStart = m.index + prefixLen
      const tokenStart = kindStart + 1
      const tokenEndCol = m.index + m[0].length + 1
      candidates.push({
        kind: m[2] as 'var' | 'local',
        name: m[3],
        start: tokenStart,
        end: tokenEndCol,
      })
    }
  }

  // module.NAME
  {
    const re = /(^|[^\w.])module\.([A-Za-z_][A-Za-z0-9_-]*)/g
    let m: RegExpExecArray | null
    while ((m = re.exec(line)) !== null) {
      const prefixLen = m[1].length
      const kindStart = m.index + prefixLen
      const tokenStart = kindStart + 1
      const nameStart = kindStart + 'module.'.length + 1
      const nameEnd = nameStart + m[2].length
      candidates.push({
        kind: 'module',
        name: m[2],
        start: tokenStart,
        end: nameEnd,
      })
    }
  }

  // data.TYPE.NAME
  {
    const re = /(^|[^\w.])data\.([A-Za-z0-9_-]+)\.([A-Za-z_][A-Za-z0-9_-]*)/g
    let m: RegExpExecArray | null
    while ((m = re.exec(line)) !== null) {
      const prefixLen = m[1].length
      const kindStart = m.index + prefixLen
      const tokenStart = kindStart + 1
      const nameEnd = kindStart + `data.${m[2]}.${m[3]}`.length + 1
      candidates.push({
        kind: 'data',
        name: `${m[2]}.${m[3]}`,
        start: tokenStart,
        end: nameEnd,
      })
    }
  }

  // TYPE.NAME — provider_resource 形态(含下划线),避免误伤普通点表达式
  {
    const re = /(^|[^\w.])([a-z][a-z0-9]*_[a-z0-9_]+)\.([A-Za-z_][A-Za-z0-9_-]*)/g
    let m: RegExpExecArray | null
    while ((m = re.exec(line)) !== null) {
      const type = m[2]
      if (type === 'var' || type === 'local') continue
      const prefixLen = m[1].length
      const kindStart = m.index + prefixLen
      const tokenStart = kindStart + 1
      const nameEnd = kindStart + `${type}.${m[3]}`.length + 1
      candidates.push({
        kind: 'resource',
        name: `${type}.${m[3]}`,
        start: tokenStart,
        end: nameEnd,
      })
    }
  }

  // 优先更长匹配
  candidates.sort((a, b) => (b.end - b.start) - (a.end - a.start))
  for (const c of candidates) {
    if (col >= c.start && col < c.end) {
      return {
        kind: c.kind,
        name: c.name,
        range: {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: c.start,
          endColumn: c.end,
        },
      }
    }
  }
  return null
}

export function lookupDefinition(index: DefinitionIndex, hit: RefHit): DefLoc | undefined {
  switch (hit.kind) {
    case 'var':
      return index.variables.get(hit.name)
    case 'local':
      return index.locals.get(hit.name)
    case 'module':
      return index.modules.get(hit.name)
    case 'data':
      return index.data.get(hit.name)
    case 'resource':
      return index.resources.get(hit.name)
    default:
      return undefined
  }
}

function refLabel(hit: RefHit): string {
  switch (hit.kind) {
    case 'var':
      return `variable "${hit.name}"`
    case 'local':
      return `local.${hit.name}`
    case 'module':
      return `module "${hit.name}"`
    case 'data':
      return `data.${hit.name}`
    case 'resource':
      return hit.name
    default:
      return hit.name
  }
}

// ---- Provider 注册 ----

const REGISTRY_KEY = '__manifestHclDefinition__'

interface RegisterOpts {
  getIndex: () => DefinitionIndex
}

export function registerHclDefinition({ getIndex }: RegisterOpts): void {
  const g = globalThis as unknown as { [k: string]: monaco.IDisposable[] | undefined }
  const prev = g[REGISTRY_KEY]
  if (prev) prev.forEach((d) => { try { d.dispose() } catch { /* 已 dispose 或已注销 */ } })
  const disposables: monaco.IDisposable[] = []
  g[REGISTRY_KEY] = disposables

  const registerOn = (langId: string) => {
    disposables.push(monaco.languages.registerDefinitionProvider(langId, {
      provideDefinition(model, position) {
        const hit = resolveReferenceAt(model, position)
        if (!hit) return null
        const def = lookupDefinition(getIndex(), hit)
        if (!def) return null
        return {
          uri: pathToManifestUri(def.path),
          range: {
            startLineNumber: def.line,
            startColumn: def.column,
            endLineNumber: def.line,
            endColumn: def.endColumn,
          },
        }
      },
    }))

    disposables.push(monaco.languages.registerHoverProvider(langId, {
      provideHover(model, position) {
        const hit = resolveReferenceAt(model, position)
        if (!hit) return null
        const def = lookupDefinition(getIndex(), hit)
        const label = refLabel(hit)
        const lines: string[] = []
        if (def) {
          const where =
            def.path === (manifestUriToPath(model.uri) ?? '') ? '当前文件' : `\`${def.path}\``
          lines.push(`**${label}** — 定义于 ${where}:${def.line}`)
          lines.push('')
          lines.push('Cmd/Ctrl + 单击 跳转到定义')
        } else {
          lines.push(`**${label}** — 未找到定义`)
        }
        return {
          range: hit.range,
          contents: lines.map((value) => ({ value })),
        }
      },
    }))
  }
  for (const langId of HCL_LANGUAGE_IDS) registerOn(langId)
}
