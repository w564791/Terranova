/**
 * HCL 轻量语法诊断(纯前端,画红/黄波浪线)。
 *
 * 原则:绝不误报(误报比漏报更伤信任)。为此用一个"字符串/注释感知"的单遍扫描器,
 * 正确跳过注释、字符串、${} 插值,只在真正的代码位置做结构判断与引用收集。
 *
 * 覆盖:
 *  1. 结构性语法错误(error):括号/方括号/花括号不配对、字符串未闭合。
 *     —— 检测到 heredoc(<<EOT)时跳过结构诊断(前端难精确解析),仍保留重复定义/未定义引用。
 *  2. 重复定义(error):variable / output / module / resource / data / local 同名。
 *  3. 未定义引用(warning):var.X / local.X 在跨文件索引里找不到定义。
 *     —— 引用只在代码区与 ${} 插值区收集(不碰注释与普通字符串),零误报。
 *
 * 真正的 HCL 语义校验由 terraform plan 兜底;这里只抓最常见、可前端可靠判断的错误。
 */
import * as monaco from 'monaco-editor'
import type { DefinitionIndex } from './hclDefinitions'

export const HCL_DIAG_OWNER = 'hcl-lint'

const OPEN_TO_CLOSE: Record<string, string> = { '{': '}', '(': ')', '[': ']' }
const CLOSE_TO_OPEN: Record<string, string> = { '}': '{', ')': '(', ']': '[' }

interface RefHit {
  kind: 'var' | 'local'
  name: string
  line: number
  column: number
  endColumn: number
}

interface ScanResult {
  structural: monaco.editor.IMarkerData[]
  refs: RefHit[]
  hadHeredoc: boolean
}

// 单遍字符扫描:跟踪 行注释 / 块注释 / 字符串 / 插值深度,
// 在代码区与插值区做括号配对 + 收集 var./local. 引用。
function scan(content: string): ScanResult {
  const structural: monaco.editor.IMarkerData[] = []
  const refs: RefHit[] = []
  let hadHeredoc = false

  // 括号栈(只在代码区,插值区的 {} 用 interpDepth 单独计,不入栈)
  const stack: { ch: string; line: number; col: number }[] = []

  let inLineComment = false
  let inBlockComment = false
  let inString = false
  let interpDepth = 0 // 字符串内 ${...} 的嵌套深度;>0 时按代码处理
  let strStartLine = 0
  let strStartCol = 0

  let line = 1
  let col = 1
  const n = content.length
  let i = 0

  const isIdentStart = (c: string) => /[A-Za-z_]/.test(c)
  const isIdentChar = (c: string) => /[A-Za-z0-9_.-]/.test(c)

  // 在代码语义下处理一个标识符 token(可能是 var.x / local.x 引用)
  const consumeIdentifier = (): void => {
    const startCol = col
    let j = i
    while (j < n && isIdentChar(content[j])) j++
    const token = content.slice(i, j)
    const m = token.match(/^(var|local)\.([A-Za-z_][A-Za-z0-9_-]*)/)
    if (m) {
      const kind = m[1] as 'var' | 'local'
      const name = m[2]
      const tokenLen = m[0].length // var.NAME 整体长度(忽略后续 .attr)
      refs.push({
        kind,
        name,
        line,
        column: startCol,
        endColumn: startCol + tokenLen,
      })
    }
    col += j - i
    i = j
  }

  while (i < n) {
    const c = content[i]
    const next = i + 1 < n ? content[i + 1] : ''

    // 行注释
    if (inLineComment) {
      if (c === '\n') {
        inLineComment = false
        line++
        col = 1
      } else {
        col++
      }
      i++
      continue
    }
    // 块注释
    if (inBlockComment) {
      if (c === '*' && next === '/') {
        inBlockComment = false
        i += 2
        col += 2
        continue
      }
      if (c === '\n') {
        line++
        col = 1
      } else {
        col++
      }
      i++
      continue
    }
    // 字符串(非插值区):不透明,跳到收尾 " / 转义 / 换行(未闭合)/ ${ 进插值
    if (inString && interpDepth === 0) {
      if (c === '\\') {
        i += 2
        col += 2
        continue
      }
      if (c === '"') {
        inString = false
        i++
        col++
        continue
      }
      if (c === '\n') {
        // 字符串未闭合(HCL 普通字符串不能跨行)
        structural.push({
          severity: monaco.MarkerSeverity.Error,
          message: '字符串未闭合(缺少右引号 ")',
          startLineNumber: strStartLine,
          startColumn: strStartCol,
          endLineNumber: strStartLine,
          endColumn: strStartCol + 1,
        })
        inString = false
        line++
        col = 1
        i++
        continue
      }
      if (c === '$' && next === '{') {
        interpDepth = 1
        i += 2
        col += 2
        continue
      }
      i++
      col++
      continue
    }

    // 代码区 或 插值区(interpDepth>0):统一按代码处理
    // 注释起始
    if (c === '#') {
      inLineComment = true
      i++
      col++
      continue
    }
    if (c === '/' && next === '/') {
      inLineComment = true
      i += 2
      col += 2
      continue
    }
    if (c === '/' && next === '*') {
      inBlockComment = true
      i += 2
      col += 2
      continue
    }
    // 字符串起始(仅在非插值代码区;插值区内再开字符串也按起始处理)
    if (c === '"') {
      inString = true
      strStartLine = line
      strStartCol = col
      i++
      col++
      continue
    }
    // heredoc 检测:前端不精确解析其内容,标记后整文件放弃结构诊断
    if (c === '<' && next === '<') {
      hadHeredoc = true
      i += 2
      col += 2
      continue
    }
    if (c === '\n') {
      line++
      col = 1
      i++
      continue
    }
    // 括号处理
    if (interpDepth > 0) {
      // 插值区:{} 计入 interpDepth,到 0 时回到字符串;() [] 不影响
      if (c === '{') {
        interpDepth++
        i++
        col++
        continue
      }
      if (c === '}') {
        interpDepth--
        i++
        col++
        if (interpDepth === 0) {
          // 回到字符串不透明态(inString 仍为 true)
        }
        continue
      }
    } else {
      // 普通代码区:三种括号入栈/配对
      if (c === '{' || c === '(' || c === '[') {
        stack.push({ ch: c, line, col })
        i++
        col++
        continue
      }
      if (c === '}' || c === ')' || c === ']') {
        const top = stack.pop()
        if (!top || top.ch !== CLOSE_TO_OPEN[c]) {
          structural.push({
            severity: monaco.MarkerSeverity.Error,
            message: `多余或不匹配的 '${c}'`,
            startLineNumber: line,
            startColumn: col,
            endLineNumber: line,
            endColumn: col + 1,
          })
        }
        i++
        col++
        continue
      }
    }
    // 标识符(可能是引用)
    if (isIdentStart(c)) {
      consumeIdentifier()
      continue
    }
    i++
    col++
  }

  // 收尾:字符串/未闭合括号
  if (inString) {
    structural.push({
      severity: monaco.MarkerSeverity.Error,
      message: '字符串未闭合(缺少右引号 ")',
      startLineNumber: strStartLine,
      startColumn: strStartCol,
      endLineNumber: strStartLine,
      endColumn: strStartCol + 1,
    })
  }
  for (const left of stack) {
    structural.push({
      severity: monaco.MarkerSeverity.Error,
      message: `未闭合的 '${left.ch}'(缺少 '${OPEN_TO_CLOSE[left.ch]}')`,
      startLineNumber: left.line,
      startColumn: left.col,
      endLineNumber: left.line,
      endColumn: left.col + 1,
    })
  }

  return { structural, refs, hadHeredoc }
}

// 重复定义检测(行扫描,跳过注释行)。定义块头在 HCL 中总在行首。
function findDuplicates(content: string): monaco.editor.IMarkerData[] {
  const markers: monaco.editor.IMarkerData[] = []
  const lines = content.split('\n')
  const seen = new Set<string>()
  let inLocals = false
  let localsDepth = 0
  const localKeys = new Set<string>()

  const pushDup = (lineIdx: number, col: number, len: number, label: string) => {
    markers.push({
      severity: monaco.MarkerSeverity.Error,
      message: `重复定义:${label}`,
      startLineNumber: lineIdx + 1,
      startColumn: col,
      endLineNumber: lineIdx + 1,
      endColumn: col + len,
    })
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const trimmed = line.trim()
    if (trimmed.startsWith('#') || trimmed.startsWith('//')) continue

    // variable / output / module "X"
    let m = line.match(/^(\s*)(variable|output|module)\s+"([^"]+)"/)
    if (m) {
      const kind = m[2]
      const name = m[3]
      const key = `${kind}:${name}`
      const nameCol = line.indexOf('"' + name + '"') + 2 // 引号后
      if (seen.has(key)) pushDup(i, nameCol, name.length, `${kind} "${name}"`)
      else seen.add(key)
      continue
    }
    // resource / data "T" "N"
    m = line.match(/^(\s*)(resource|data)\s+"([^"]+)"\s+"([^"]+)"/)
    if (m) {
      const kind = m[2]
      const addr = `${m[3]}.${m[4]}`
      const key = `${kind}:${addr}`
      const nameCol = line.indexOf('"' + m[4] + '"') + 2
      if (seen.has(key)) pushDup(i, nameCol, m[4].length, `${kind} ${addr}`)
      else seen.add(key)
      continue
    }
    // locals 块内顶层 key 重复
    if (/^locals\s*\{/.test(trimmed)) {
      inLocals = true
      localsDepth = 0
      localKeys.clear()
    }
    if (inLocals) {
      localsDepth += (line.match(/\{/g)?.length ?? 0)
      localsDepth -= (line.match(/\}/g)?.length ?? 0)
      if (localsDepth === 1) {
        const lm = trimmed.match(/^([A-Za-z_][A-Za-z0-9_-]*)\s*=/)
        if (lm && lm[1] !== 'locals') {
          const name = lm[1]
          const col = line.indexOf(name) + 1
          if (localKeys.has(name)) pushDup(i, col, name.length, `local.${name}`)
          else localKeys.add(name)
        }
      }
      if (localsDepth <= 0 && /\}/.test(line)) inLocals = false
    }
  }
  return markers
}

// 计算某文件全部诊断 marker。index 用于"未定义引用"判断(跨文件)。
// indexReady=false 时跳过"未定义引用"检查 —— 全量索引还没建好时,引用的定义可能在
// 尚未读入的文件里,过早报 undefined 会误报。结构/重复错误始终可靠,不受此限。
export function computeHclDiagnostics(
  content: string,
  index: DefinitionIndex,
  indexReady: boolean,
): monaco.editor.IMarkerData[] {
  const { structural, refs, hadHeredoc } = scan(content)

  // heredoc 存在时结构诊断(括号配对)不可靠,跳过 structural;重复定义与未定义引用仍可用
  const markers: monaco.editor.IMarkerData[] = hadHeredoc
    ? [...findDuplicates(content)]
    : [...structural, ...findDuplicates(content)]

  if (!indexReady) return markers

  // 未定义引用(warning):索引里查不到定义。索引含本文件(增量更新),故同文件定义也算。
  // 仅对 var./local. 报未定义(module/resource 可能来自外部 module 源,误报风险高)。
  for (const r of refs) {
    const defined = r.kind === 'var' ? index.variables.has(r.name) : index.locals.has(r.name)
    if (!defined) {
      markers.push({
        severity: monaco.MarkerSeverity.Warning,
        message:
          r.kind === 'var'
            ? `未定义的变量 var.${r.name}(找不到 variable "${r.name}")`
            : `未定义的 local.${r.name}(找不到 locals 里的 ${r.name})`,
        startLineNumber: r.line,
        startColumn: r.column,
        endLineNumber: r.line,
        endColumn: r.endColumn,
      })
    }
  }

  return markers
}
