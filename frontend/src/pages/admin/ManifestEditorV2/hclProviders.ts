/**
 * 4 个 Monaco provider — 对齐 manifest-vscode-mockup.html demo
 *
 *  1. CompletionItemProvider (IntelliSense)
 *      - 在 source = "<here>" 引号内 → 平台 module 列表
 *      - 在普通空行 → (module × demo) 完整 module block 插入
 *
 *  2. HoverProvider (只读元数据)
 *      - 鼠标停在 source = "platform/xxx" 字面量上 → module 描述
 *
 *  3. InlayHintsProvider (demo 选择主入口)
 *      - 每个 module "x" { 块行末 "· N demos" 标签
 *      - 鼠标 hover tooltip 显示可点击的 demo 列表
 *
 *  4. CodeActionProvider (Quick Fix 灯泡)
 *      - 光标位于 module body 内, body 仅 source/version 时弹灯泡
 *
 * 数据来自 PR2-C-1 后端 /manifest-editor/modules + /:id/demos,
 * 内存缓存(moduleDemoApi.ts)。
 */
import * as monaco from 'monaco-editor'
import {
  warmUpCache,
  getCachedModules,
  getCachedDemos,
  fetchDemos,
  type ModuleSummary,
  type DemoSummary,
} from './moduleDemoApi'

// HMR 会重算本模块、抹掉模块级变量,但 Monaco 全局 provider 注册表不会跟着重置。
// 把各 provider 的 disposable 存到 globalThis,重注册前先 dispose 旧的,
// 否则每次保存源码热更都会叠加一份 provider,导致 InlayHint「· N demos」等
// 在同一行渲染多份。(initServices.ts 对 vscode-api initialize 用同样的兜底思路。)
const REGISTRY_KEY = '__manifestHclProviders__'

export function registerHclProviders(): void {
  const g = globalThis as unknown as { [k: string]: monaco.IDisposable[] | undefined }
  const prev = g[REGISTRY_KEY]
  if (prev) prev.forEach((d) => { try { d.dispose() } catch { /* 已 dispose 或已注销 */ } })
  const disposables: monaco.IDisposable[] = []
  g[REGISTRY_KEY] = disposables

  // 后台预热缓存(不阻塞)
  void warmUpCache()

  // ---- 通道 1: Completion ----
  disposables.push(monaco.languages.registerCompletionItemProvider('hcl', {
    triggerCharacters: ['"', ' '],
    async provideCompletionItems(model, position) {
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
      const modules = getCachedModules()

      // 场景 A: 在 source = "<here>" 内 → 仅 module source 列表
      const inSourceQuote = /source\s*=\s*"[^"]*$/.test(before)
      if (inSourceQuote) {
        const quoteStart = before.lastIndexOf('"')
        const sourceRange: monaco.IRange = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: quoteStart + 2,
          endColumn: position.column,
        }
        modules.forEach((m) => {
          suggestions.push({
            label: { label: m.source, description: `${m.demo_count} demos` },
            kind: monaco.languages.CompletionItemKind.Module,
            documentation: {
              value: `**${m.description || m.name}**\n\nLatest: \`${m.latest_version}\``,
            },
            detail: 'platform module',
            insertText: m.source,
            range: sourceRange,
            sortText: '0_' + m.source,
          })
        })
        return { suggestions }
      }

      // 场景 B: 普通位置 → (module × demo) 全部 block
      // 这里是同步路径(已缓存)
      modules.forEach((m) => {
        const demos = getCachedDemos(m.module_id)
        // 空配置(只 source/version)
        suggestions.push({
          label: { label: `module "${m.source}"`, description: '空配置 — 仅 source/version' },
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: {
            value: `**${m.description || m.name}**\n\n\`\`\`hcl\n${renderDemoToPreview(m, null)}\n\`\`\``,
          },
          detail: 'platform module',
          insertText: renderDemoToSnippet(m, null),
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range,
          sortText: `1_${m.source}_0`,
        })
        demos.forEach((d, i) => {
          suggestions.push({
            label: {
              label: `${m.source} · ${d.name}`,
              description: d.is_default ? '默认 demo' : 'demo',
            },
            kind: monaco.languages.CompletionItemKind.Snippet,
            documentation: {
              value: `**${m.description || m.name}**\n\n${d.description || d.change_summary || ''}\n\n\`\`\`hcl\n${renderDemoToPreview(m, d)}\n\`\`\``,
            },
            detail: m.source,
            insertText: renderDemoToSnippet(m, d),
            insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
            range,
            sortText: `1_${m.source}_${i + 1}${d.is_default ? '_default' : ''}`,
          })
        })
      })
      return { suggestions }
    },
  }))

  // ---- 命令: 用 demo 替换指定 module 块 (Inlay Hint tooltip 链接调用) ----
  disposables.push(monaco.editor.registerCommand(
    'manifestInsertDemo',
    (
      _accessor: unknown,
      payload: {
        moduleSource: string
        demoId: number
        blockStartLine: number
        blockEndLine: number
        instanceName: string
      },
    ) => {
      const editor = monaco.editor.getEditors()[0]
      if (!editor) return
      const model = editor.getModel()
      if (!model) return
      const mod = getCachedModules().find((p) => p.source === payload.moduleSource)
      if (!mod) return
      const demo = getCachedDemos(mod.module_id).find((d) => d.demo_id === payload.demoId)
      if (!demo) return
      const endCol = (model.getLineContent(payload.blockEndLine) || '').length + 1
      const fullRange = new monaco.Range(
        payload.blockStartLine,
        1,
        payload.blockEndLine,
        endCol,
      )
      const text = renderDemoToPreview(mod, demo).replace(
        /^module\s+"[^"]+"/,
        `module "${payload.instanceName}"`,
      )
      editor.executeEdits('manifest-demo', [{ range: fullRange, text, forceMoveMarkers: true }])
      editor.focus()
    },
  ))

  // ---- 通道 2: Hover ----
  disposables.push(monaco.languages.registerHoverProvider('hcl', {
    provideHover(model, position) {
      const line = model.getLineContent(position.lineNumber)
      const sm = line.match(/source\s*=\s*"([^"]+)"/)
      if (!sm) return null
      const mod = getCachedModules().find((p) => p.source === sm[1])
      if (!mod) return null
      const demoCount = mod.demo_count
      const demoHint =
        demoCount > 0
          ? `_鼠标移到行末 \`· ${demoCount} demos\` 标签上选择 demo_`
          : '_(该 module 暂无 demo)_'
      return {
        contents: [
          { value: `### ${mod.source}` },
          { value: `${mod.description || mod.name}\n\nLatest: \`${mod.latest_version}\`` },
          { value: demoHint },
        ],
      }
    },
  }))

  // ---- 通道 3: Inlay Hint (demo 选择主入口) ----
  disposables.push(monaco.languages.registerInlayHintsProvider('hcl', {
    async provideInlayHints(model, range) {
      const hints: monaco.languages.InlayHint[] = []
      const text = model.getValue()
      const lines = text.split('\n')
      const modules = getCachedModules()

      // 关键: 必须只返回落在 Monaco 请求的可见范围(range)内的 hint。
      // 否则 adapter 在滚动 / 草稿自动保存触发模型版本变化、对新可见范围重新请求时,
      // 会把同一个 hint 当成属于多个范围请求的结果,在 module 行上渲染多份
      // (上下滑表现不同正是「返回了范围外 hint」的典型特征)。
      // 扫描范围放大一点(±3 行),覆盖 hint 标签可能跨行的边界情况。
      const startLine = Math.max(0, range.startLineNumber - 4)
      const endLine = Math.min(lines.length, range.endLineNumber + 3)

      for (let i = startLine; i < endLine; i++) {
        const declMatch = lines[i].match(/^\s*module\s+"([^"]+)"\s*\{/)
        if (!declMatch) continue
        const instanceName = declMatch[1]
        let mod: ModuleSummary | null = null
        let blockEnd = -1
        for (let j = i + 1; j < lines.length; j++) {
          if (/^\s*\}/.test(lines[j])) {
            blockEnd = j
            break
          }
          if (!mod) {
            const sm = lines[j].match(/source\s*=\s*"([^"]+)"/)
            if (sm) mod = modules.find((p) => p.source === sm[1]) ?? null
          }
        }
        if (!mod || blockEnd < 0) continue
        // 拉 demo (异步,但不阻塞 inlay hint 渲染 — 已缓存就直接用)
        const demos = getCachedDemos(mod.module_id)
        if (demos.length === 0) {
          // 没缓存,触发后台拉,这次先不显示
          void fetchDemos(mod.module_id)
          continue
        }

        // 构 tooltip
        const tooltipLines = [
          `### ${mod.source} · 选择 demo 应用`,
          ``,
          `_点击下方任一项,该 \`module "${instanceName}"\` 块整段会被替换为该 demo:_`,
          ``,
        ]
        demos.forEach((d) => {
          const args = encodeURIComponent(
            JSON.stringify([
              {
                moduleSource: mod!.source,
                demoId: d.demo_id,
                blockStartLine: i + 1,
                blockEndLine: blockEnd + 1,
                instanceName,
              },
            ]),
          )
          const star = d.is_default ? '(默认) ' : ''
          // 列表显示 demo 名称 + demo 描述(不是版本变更摘要 change_summary)
          tooltipLines.push(
            `- ${star}[**${d.name}**](command:manifestInsertDemo?${args} "应用此 demo")  \n  ${d.description || ''}`,
          )
        })

        hints.push({
          position: { lineNumber: i + 1, column: lines[i].length + 1 },
          label: `   · ${demos.length} demos`,
          paddingLeft: true,
          kind: monaco.languages.InlayHintKind.Type,
          tooltip: { value: tooltipLines.join('\n'), isTrusted: true, supportHtml: true },
        })
      }
      return { hints, dispose: () => {} }
    },
  }))

  // ---- 通道 4: Code Action (Quick Fix 灯泡) ----
  disposables.push(monaco.languages.registerCodeActionProvider('hcl', {
    provideCodeActions(model, range) {
      const text = model.getValue()
      const lines = text.split('\n')
      const cursorLine = range.startLineNumber - 1
      // 向上找 module "x" { 开头
      let blockStart = -1
      for (let i = cursorLine; i >= 0; i--) {
        if (/^\s*module\s+"([^"]+)"\s*\{/.test(lines[i])) {
          blockStart = i
          break
        }
        if (i < cursorLine && /^\s*\}/.test(lines[i])) {
          return { actions: [], dispose: () => {} }
        }
      }
      if (blockStart < 0) return { actions: [], dispose: () => {} }
      // 向下找 }
      let blockEnd = -1
      for (let i = blockStart; i < lines.length; i++) {
        if (i > blockStart && /^\s*\}/.test(lines[i])) {
          blockEnd = i
          break
        }
      }
      if (blockEnd < 0) return { actions: [], dispose: () => {} }
      // 找 source
      let sourceLine = ''
      for (let i = blockStart; i <= blockEnd; i++) {
        const sm = lines[i].match(/source\s*=\s*"([^"]+)"/)
        if (sm) {
          sourceLine = sm[1]
          break
        }
      }
      const mod = getCachedModules().find((p) => p.source === sourceLine)
      if (!mod) return { actions: [], dispose: () => {} }
      const demos = getCachedDemos(mod.module_id)
      if (demos.length === 0) return { actions: [], dispose: () => {} }
      // body "基本为空" 检查 — 排除 source/version 后非空行 < 2
      const bodyContentLines: string[] = []
      for (let i = blockStart + 1; i < blockEnd; i++) {
        const t = lines[i].trim()
        if (t === '' || t.startsWith('#') || t.startsWith('//')) continue
        if (/^source\s*=/.test(t) || /^version\s*=/.test(t)) continue
        bodyContentLines.push(t)
      }
      if (bodyContentLines.length >= 2) return { actions: [], dispose: () => {} }
      const declMatch = lines[blockStart].match(/module\s+"([^"]+)"/)
      const instanceName = declMatch ? declMatch[1] : 'my_instance'
      const fullRange = new monaco.Range(
        blockStart + 1,
        1,
        blockEnd + 1,
        lines[blockEnd].length + 1,
      )
      const actions: monaco.languages.CodeAction[] = demos.map((d) => ({
        title: `应用 demo: ${d.name}${d.is_default ? ' (默认)' : ''}`,
        kind: 'quickfix',
        isPreferred: d.is_default,
        edit: {
          edits: [
            {
              resource: model.uri,
              versionId: model.getVersionId(),
              textEdit: {
                range: fullRange,
                text: renderDemoToPreview(mod, d).replace(
                  /^module\s+"[^"]+"/,
                  `module "${instanceName}"`,
                ),
              },
            } as monaco.languages.IWorkspaceTextEdit,
          ],
        },
      }))
      return { actions, dispose: () => {} }
    },
  }))
}

// ============================================================
// HCL render helpers (config_data → snippet / preview)
// 移植自 manifest-vscode-mockup.html demo
// ============================================================

function quoteIfString(v: unknown): string {
  if (typeof v === 'string') {
    if (v.startsWith('$$')) return v.slice(2)
    return `"${v.replace(/"/g, '\\"')}"`
  }
  if (v === null) return 'null'
  return String(v)
}

function renderArray(arr: unknown[]): string {
  return '[' + arr.map((v) => quoteIfString(v)).join(', ') + ']'
}

function renderObject(obj: Record<string, unknown>, indent: number): string {
  const pad = ' '.repeat(indent)
  const padInner = ' '.repeat(indent + 2)
  const lines: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    if (Array.isArray(v)) {
      lines.push(`${padInner}${k} = ${renderArray(v)}`)
    } else if (v && typeof v === 'object') {
      lines.push(`${padInner}${k} = ${renderObject(v as Record<string, unknown>, indent + 2)}`)
    } else {
      lines.push(`${padInner}${k} = ${quoteIfString(v)}`)
    }
  }
  return `{\n${lines.join('\n')}\n${pad}}`
}

/** 渲染成带 ${N:default} placeholder 的 Monaco snippet */
export function renderDemoToSnippet(module: ModuleSummary, demo: DemoSummary | null): string {
  const inst =
    demo?.name.replace(/[^a-zA-Z0-9_]/g, '_').toLowerCase() || 'my_instance'
  const lines: string[] = []
  lines.push(`module "\${1:${inst}}" {`)
  lines.push(`  source  = "${module.source}"`)
  lines.push(`  version = "${module.latest_version}"`)
  if (demo && Object.keys(demo.config_data).length > 0) {
    lines.push('')
    let idx = 2
    for (const [k, v] of Object.entries(demo.config_data)) {
      if (Array.isArray(v)) {
        lines.push(`  ${k} = \${${idx}:${renderArray(v as unknown[])}}`)
      } else if (v && typeof v === 'object') {
        lines.push(`  ${k} = ${renderObject(v as Record<string, unknown>, 2)}`)
      } else if (typeof v === 'string' && v.startsWith('$$')) {
        lines.push(`  ${k} = ${v.slice(2)}`)
      } else {
        lines.push(`  ${k} = \${${idx}:${quoteIfString(v)}}`)
      }
      idx++
    }
  } else {
    lines.push(`  $0`)
  }
  lines.push(`}`)
  return lines.join('\n')
}

/** 渲染只读预览 (Hover / completion documentation 用,无 placeholder) */
export function renderDemoToPreview(module: ModuleSummary, demo: DemoSummary | null): string {
  const inst =
    demo?.name.replace(/[^a-zA-Z0-9_]/g, '_').toLowerCase() || 'my_instance'
  const lines: string[] = []
  lines.push(`module "${inst}" {`)
  lines.push(`  source  = "${module.source}"`)
  lines.push(`  version = "${module.latest_version}"`)
  if (demo && Object.keys(demo.config_data).length > 0) {
    lines.push('')
    for (const [k, v] of Object.entries(demo.config_data)) {
      if (Array.isArray(v)) {
        lines.push(`  ${k} = ${renderArray(v as unknown[])}`)
      } else if (v && typeof v === 'object') {
        lines.push(`  ${k} = ${renderObject(v as Record<string, unknown>, 2)}`)
      } else if (typeof v === 'string' && v.startsWith('$$')) {
        lines.push(`  ${k} = ${v.slice(2)}`)
      } else {
        lines.push(`  ${k} = ${quoteIfString(v)}`)
      }
    }
  }
  lines.push(`}`)
  return lines.join('\n')
}
