/**
 * HCL 变量引用「转到定义」(var. / local.,支持跨文件)。
 *
 * 提供:
 *  - 稳定 model URI 工具(manifest:/<path> ↔ path),让跨文件跳转能反解目标文件
 *  - 跨文件定义索引(扫所有 .tf 的 variable "X" 与 locals 内 key =,带精确行列)
 *  - 引用解析(光标是否落在 var.NAME / local.NAME 的 NAME 上)
 *  - DefinitionProvider(Cmd+Click 跳转)+ HoverProvider(提示可跳转)
 *
 * 跨文件机制:DefinitionProvider 返回带 manifest URI 的 Location;目标非当前 model 时,
 * standalone editor 走 registerEditorOpener(在 ManifestEditorV2 注册)委托到 openFile。
 */
import * as monaco from 'monaco-editor'

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

export interface DefinitionIndex {
  variables: Map<string, DefLoc>
  locals: Map<string, DefLoc>
}

export function emptyIndex(): DefinitionIndex {
  return { variables: new Map(), locals: new Map() }
}

// 扫单个文件,把其中的 variable / local 定义并入 index(同名保留首个)。
// 只处理 .tf;其他扩展名跳过。
export function indexFile(index: DefinitionIndex, path: string, content: string): void {
  if (!path.endsWith('.tf')) return
  const lines = content.split('\n')
  let inLocals = false
  let localsDepth = 0
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const trimmed = line.trim()

    // variable "X" {
    const vm = line.match(/^(\s*)variable\s+"([^"]+)"/)
    if (vm) {
      const name = vm[2]
      // NAME 在引号内的精确列:找第一个 " 之后
      const q = line.indexOf('"')
      const col = q + 2 // 1-based,引号后第一个字符
      if (!index.variables.has(name)) {
        index.variables.set(name, { path, line: i + 1, column: col, endColumn: col + name.length })
      }
      continue
    }

    // locals { ... } 块跟踪(深度,支持嵌套对象)
    if (/^locals\s*\{/.test(trimmed)) {
      inLocals = true
      localsDepth = 0
    }
    if (inLocals) {
      localsDepth += (line.match(/\{/g)?.length ?? 0)
      localsDepth -= (line.match(/\}/g)?.length ?? 0)
      // 只认顶层 key =(深度为 1 时,即 locals{} 直接子级)。用 trimmed 提名,行内定列。
      if (localsDepth === 1) {
        const lm = trimmed.match(/^([A-Za-z_][A-Za-z0-9_-]*)\s*=/)
        if (lm && lm[1] !== 'locals') {
          const name = lm[1]
          const col = line.indexOf(name) + 1 // 1-based
          if (col > 0 && !index.locals.has(name)) {
            index.locals.set(name, { path, line: i + 1, column: col, endColumn: col + name.length })
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
  for (const [name, loc] of index.variables) {
    if (loc.path === path) index.variables.delete(name)
  }
  for (const [name, loc] of index.locals) {
    if (loc.path === path) index.locals.delete(name)
  }
}

// ---- 引用解析 ----

export interface RefHit {
  kind: 'var' | 'local'
  name: string
  // 引用 token(var.NAME / local.NAME 整体)在行内的范围,用于 hover 高亮
  range: monaco.IRange
}

// 判断光标是否落在 var.NAME / local.NAME 的引用上(NAME 段)。var.a.b 取首段 a。
export function resolveReferenceAt(
  model: monaco.editor.ITextModel,
  position: monaco.Position,
): RefHit | null {
  const line = model.getLineContent(position.lineNumber)
  // 匹配所有 var.xxx / local.xxx 出现,找包含光标列的那个
  const re = /(^|[^\w.])(var|local)\.([A-Za-z_][A-Za-z0-9_-]*)/g
  let m: RegExpExecArray | null
  const col = position.column // 1-based
  while ((m = re.exec(line)) !== null) {
    const prefixLen = m[1].length
    const kindStart = m.index + prefixLen // 0-based,'var'/'local' 起始
    const tokenStart = kindStart + 1 // 1-based 起始列
    const tokenEnd = m.index + m[0].length // 0-based exclusive end of whole match
    const tokenEndCol = tokenEnd + 1 // 1-based exclusive
    // 光标在 [tokenStart, tokenEndCol) 内即命中(含 var./local. 前缀与名字)
    if (col >= tokenStart && col < tokenEndCol) {
      const keyword = m[2] as 'var' | 'local'
      const name = m[3]
      return {
        kind: keyword,
        name,
        range: {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: tokenStart,
          endColumn: tokenEndCol,
        },
      }
    }
  }
  return null
}

// ---- Provider 注册 ----

// HMR 会重算本模块、抹掉模块级变量,但 Monaco 全局 provider 注册表不会跟着重置。
// 把 disposable 存到 globalThis,重注册前先 dispose 旧的,避免 provider 叠加。
// (与 hclProviders.ts 同一思路;详见 initServices.ts 对 HMR 的说明。)
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

  const lookup = (hit: RefHit): DefLoc | undefined => {
    const idx = getIndex()
    return hit.kind === 'var' ? idx.variables.get(hit.name) : idx.locals.get(hit.name)
  }

  disposables.push(monaco.languages.registerDefinitionProvider('hcl', {
    provideDefinition(model, position) {
      const hit = resolveReferenceAt(model, position)
      if (!hit) return null
      const def = lookup(hit)
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

  disposables.push(monaco.languages.registerHoverProvider('hcl', {
    provideHover(model, position) {
      const hit = resolveReferenceAt(model, position)
      if (!hit) return null
      const def = lookup(hit)
      const label = hit.kind === 'var' ? `variable "${hit.name}"` : `local.${hit.name}`
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
