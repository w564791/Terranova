/**
 * 跨文件搜索/替换引擎(纯函数,前端在草稿全文件内容上跑)。
 *
 * 对标 VS Code 全局搜索:match case / whole word / regex 三选项,结果按文件分组、
 * 带行号与行预览、命中区间(用于跳转高亮)。不搜历史版本,只作用于当前草稿内容。
 */

export interface SearchOptions {
  matchCase: boolean
  wholeWord: boolean
  regex: boolean
}

// 单个命中:文件内某行的一处匹配
export interface SearchMatch {
  line: number // 1-based 行号
  column: number // 1-based 列(匹配起始)
  endColumn: number // 1-based 列(匹配结束,exclusive+1 即 monaco 习惯)
  preview: string // 该行原文(用于结果列表展示)
  matchStart: number // preview 内匹配起始下标(高亮用)
  matchEnd: number // preview 内匹配结束下标
}

// 一个文件的命中分组
export interface FileMatches {
  path: string
  matches: SearchMatch[]
}

// 把查询词构造成全局正则。非 regex 模式转义元字符;wholeWord 加 \b 边界。
// 返回 null 表示查询非法(regex 模式下语法错误)或为空。
export function buildSearchRegex(query: string, opts: SearchOptions): RegExp | null {
  if (!query) return null
  let pattern = opts.regex ? query : escapeRegExp(query)
  if (opts.wholeWord) pattern = `\\b${pattern}\\b`
  const flags = opts.matchCase ? 'g' : 'gi'
  try {
    return new RegExp(pattern, flags)
  } catch {
    return null // regex 语法错误
  }
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// 在一组 (path -> content) 上搜索,返回按文件分组的命中(无命中的文件不返回)。
// re 必须带 'g' flag。每行独立匹配,避免跨行 .* 吞整文件。
export function searchFiles(files: { path: string; content: string }[], re: RegExp): FileMatches[] {
  const out: FileMatches[] = []
  for (const f of files) {
    const matches: SearchMatch[] = []
    const lines = f.content.split('\n')
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]
      re.lastIndex = 0
      let m: RegExpExecArray | null
      while ((m = re.exec(line)) !== null) {
        const start = m.index
        const end = m.index + m[0].length
        matches.push({
          line: i + 1,
          column: start + 1,
          endColumn: end + 1,
          preview: line,
          matchStart: start,
          matchEnd: end,
        })
        if (m[0].length === 0) re.lastIndex++ // 防零宽匹配死循环
        if (matches.length > 1000) break // 单文件命中上限,防爆
      }
      if (matches.length > 1000) break
    }
    if (matches.length > 0) out.push({ path: f.path, matches })
  }
  return out
}

// 对单个文件内容做替换,返回替换后的新内容 + 替换次数。
// skip: 要跳过的命中(按 line+column 标识),用于"单条跳过"。
export function replaceInContent(
  content: string,
  re: RegExp,
  replacement: string,
  isRegex: boolean,
  skip?: Set<string>,
): { content: string; count: number } {
  let count = 0
  const lines = content.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    re.lastIndex = 0
    // 从右往左替换避免下标偏移;先收集本行命中区间
    const spans: { start: number; end: number }[] = []
    let m: RegExpExecArray | null
    while ((m = re.exec(line)) !== null) {
      spans.push({ start: m.index, end: m.index + m[0].length })
      if (m[0].length === 0) re.lastIndex++
    }
    if (spans.length === 0) continue
    let newLine = line
    for (let j = spans.length - 1; j >= 0; j--) {
      const s = spans[j]
      const key = `${i + 1}:${s.start + 1}`
      if (skip?.has(key)) continue
      // regex 模式下 replacement 支持 $1/$& 等捕获组引用;字面量模式原样插入
      let replaced = replacement
      if (isRegex) {
        const matched = line.slice(s.start, s.end)
        replaced = matched.replace(new RegExp(re.source, re.flags.replace('g', '')), replacement)
      }
      newLine = newLine.slice(0, s.start) + replaced + newLine.slice(s.end)
      count++
    }
    lines[i] = newLine
  }
  return { content: lines.join('\n'), count }
}
