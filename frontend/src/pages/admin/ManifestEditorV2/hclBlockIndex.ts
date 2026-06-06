/**
 * HCL 块定位索引(manifest AI 专用,独立于 hclDefinitions 的 DefinitionIndex)。
 *
 * 把草稿里所有 .tf 的「定义块」按 terraform 地址(resource_id)索引到 文件+行号:
 *   variable "X"            -> var.X
 *   locals { Y = }          -> local.Y
 *   module "M"              -> module.M
 *   data "T" "N"            -> data.T.N
 *   resource "TYPE" "NAME"  -> TYPE.NAME   (如 aws_instance.test)
 *
 * 两个出口:
 *   - locateBlock(index, resourceId)  : workspace 资源跳转 / 问题点击定位用
 *   - findExternalRefs(file, content, index) : 跨文件检查——找当前文件引用了但定义在别处的文件
 *
 * 不依赖 monaco,纯字符串扫描,便于复用与测试。不碰 hclDefinitions.ts。
 */

export interface BlockLoc {
  file: string
  line: number // 1-based,块起始行
}

// resourceId -> 定义位置。resourceId 形如 var.x / local.y / module.m / data.t.n / aws_instance.test
export type BlockIndex = Map<string, BlockLoc>

// 单文件扫描:把 path 内的定义块并入 index(同名保留首个,与 hclDefinitions 行为一致)。
export function indexBlocks(index: BlockIndex, path: string, content: string): void {
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
      addBlock(index, `var.${vm[1]}`, path, lineNo)
      continue
    }

    // module "M" {
    const mm = line.match(/^\s*module\s+"([^"]+)"/)
    if (mm) {
      addBlock(index, `module.${mm[1]}`, path, lineNo)
      continue
    }

    // data "TYPE" "NAME" {
    const dm = line.match(/^\s*data\s+"([^"]+)"\s+"([^"]+)"/)
    if (dm) {
      addBlock(index, `data.${dm[1]}.${dm[2]}`, path, lineNo)
      continue
    }

    // resource "TYPE" "NAME" {  ->  TYPE.NAME
    const rm = line.match(/^\s*resource\s+"([^"]+)"\s+"([^"]+)"/)
    if (rm) {
      addBlock(index, `${rm[1]}.${rm[2]}`, path, lineNo)
      continue
    }

    // locals { ... } 顶层 key(深度跟踪,支持嵌套对象),逻辑同 hclDefinitions.indexFile
    const opensLocals = /^locals\s*\{/.test(trimmed)
    if (opensLocals) {
      // 单行写法 locals { region = "x" ... } / locals { a = 1, b = 2 }:从 { 到 } 内直接抓顶层 key
      const inner = trimmed.replace(/^locals\s*\{/, '')
      if (inner.includes('}')) {
        // 单行块:截到第一个顶层 } 之前,抓 key(简单逗号/起始分隔)
        const body = inner.slice(0, inner.indexOf('}'))
        for (const m of body.matchAll(/(?:^|,)\s*([A-Za-z_][A-Za-z0-9_-]*)\s*=/g)) {
          addBlock(index, `local.${m[1]}`, path, lineNo)
        }
        // 单行块在本行闭合,不进入多行状态
      } else {
        inLocals = true
        localsDepth = 1 // 已进入 locals 的 { 内,顶层 key 在 depth==1
      }
      continue
    }
    if (inLocals) {
      // 用「行首深度」判断顶层 key:本行 brace 增减只影响后续行,
      // 否则 `tags = {`(多行对象)会先 +1 变成 depth 2 而漏掉 tags。
      const depthBefore = localsDepth
      if (depthBefore === 1) {
        const lm = trimmed.match(/^([A-Za-z_][A-Za-z0-9_-]*)\s*=/)
        if (lm) addBlock(index, `local.${lm[1]}`, path, lineNo)
      }
      localsDepth += (line.match(/\{/g)?.length ?? 0)
      localsDepth -= (line.match(/\}/g)?.length ?? 0)
      if (localsDepth <= 0) inLocals = false
    }
  }
}

function addBlock(index: BlockIndex, id: string, file: string, line: number): void {
  if (!index.has(id)) index.set(id, { file, line })
}

// 全量构建:从一组 (path, content) 重建索引。
export function buildBlockIndex(files: { path: string; content: string }[]): BlockIndex {
  const index: BlockIndex = new Map()
  for (const f of files) indexBlocks(index, f.path, f.content)
  return index
}

// 按 resourceId 定位块。支持传入带属性后缀的地址(如 aws_x.y.id / module.m.out),取定义前缀匹配。
export function locateBlock(index: BlockIndex, resourceId: string): BlockLoc | null {
  if (index.has(resourceId)) return index.get(resourceId)!
  // 容错:resource_id 带了属性段(module.m.out / aws_x.y.id / data.t.n.attr),逐段回退到定义地址
  const parts = resourceId.split('.')
  for (let n = parts.length; n >= 2; n--) {
    const candidate = parts.slice(0, n).join('.')
    if (index.has(candidate)) return index.get(candidate)!
  }
  return null
}

// 匹配一行内的所有引用地址(供 findExternalRefs 用)。返回引用对应的「定义地址」集合。
//   var.X / local.Y / module.M(.out) / data.T.N(.attr) / TYPE.NAME(.attr)
// 注:跳过出现在定义处的 token(行首的 variable/module/data/resource 声明本身)。
function refsInLine(line: string): string[] {
  const out: string[] = []
  // 定义行本身不算引用
  if (/^\s*(variable|module|data|resource|locals)\b/.test(line)) {
    // data/resource/module 定义行右侧可能仍有引用(如默认值),但起始声明不计;
    // 简化:定义行整体跳过引用提取,避免把自身名字当引用。
    return out
  }

  // var. / local.
  let m: RegExpExecArray | null
  const vl = /(^|[^\w.])(var|local)\.([A-Za-z_][A-Za-z0-9_-]*)/g
  while ((m = vl.exec(line)) !== null) out.push(`${m[2]}.${m[3]}`)

  // module.M / data.T.N
  const md = /(^|[^\w.])module\.([A-Za-z_][A-Za-z0-9_-]*)/g
  while ((m = md.exec(line)) !== null) out.push(`module.${m[2]}`)
  const dt = /(^|[^\w.])data\.([A-Za-z_][A-Za-z0-9_-]*)\.([A-Za-z_][A-Za-z0-9_-]*)/g
  while ((m = dt.exec(line)) !== null) out.push(`data.${m[2]}.${m[3]}`)

  // TYPE.NAME(.attr) 资源属性引用,如 aws_instance.test.id。
  // TYPE 必须形如 provider_xxx(含下划线)以避免误伤 module./data./var./local. 与普通点号表达式。
  const res = /(^|[^\w.])([a-z][a-z0-9]*_[a-z0-9_]+)\.([A-Za-z_][A-Za-z0-9_-]*)/g
  while ((m = res.exec(line)) !== null) {
    const type = m[2]
    if (type === 'var' || type === 'local') continue
    out.push(`${type}.${m[3]}`)
  }

  return out
}

// 找当前文件引用了、但定义在别的文件里的符号,返回那些文件的去重路径集合。
export function findExternalRefs(currentFile: string, content: string, index: BlockIndex): string[] {
  const files = new Set<string>()
  for (const line of content.split('\n')) {
    for (const ref of refsInLine(line)) {
      const loc = locateBlock(index, ref)
      if (loc && loc.file !== currentFile) files.add(loc.file)
    }
  }
  return [...files]
}
