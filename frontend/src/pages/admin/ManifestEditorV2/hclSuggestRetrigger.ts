/**
 * Monaco 默认只在「输入 triggerCharacters」时自动弹出补全;
 * 退格/删除把 `var..` 改回 `var.` 时不会再 trigger,列表就没了。
 *
 * 本模块在内容/光标变化后,若光标前恰好是 HCL 引用前缀,主动 triggerSuggest。
 */
import * as monaco from 'monaco-editor'
import { HCL_LANGUAGE_IDS } from './hclLanguage'

const HCL_LANG = new Set<string>(HCL_LANGUAGE_IDS)

/**
 * 光标前文本是否处于「应弹出引用/类型补全」的位置。
 * 与 hclCompletion 里 var./local./module./data./resource 触发条件对齐。
 */
export function shouldRetriggerHclSuggest(before: string): boolean {
  // var. / local. / module. / data.
  if (/(^|[^\w.])(var|local|module|data)\.$/.test(before)) return true
  // data.type.
  if (/(^|[^\w.])data\.[A-Za-z0-9_-]+\.$/.test(before)) return true
  // aws_instance. / provider_resource.
  if (/(^|[^\w.])[a-z][a-z0-9]*_[a-z0-9_]+\.$/.test(before)) return true
  // resource "… / data "… 引号内类型
  if (/^\s*(resource|data)\s+"[^"]*$/.test(before)) return true
  return false
}

/**
 * 挂到 standalone editor;返回 disposable(编辑器 dispose 前调用即可)。
 */
export function attachHclSuggestRetrigger(
  editor: monaco.editor.IStandaloneCodeEditor,
): monaco.IDisposable {
  let timer: ReturnType<typeof setTimeout> | null = null
  /** 同一位置避免连续 trigger 两次(输入 `.` 时既走 triggerCharacters 又走这里) */
  let lastFiredKey = ''

  const clearTimer = () => {
    if (timer != null) {
      clearTimeout(timer)
      timer = null
    }
  }

  const tryFire = () => {
    const model = editor.getModel()
    const pos = editor.getPosition()
    if (!model || !pos || editor.getOptions().get(monaco.editor.EditorOption.readOnly)) {
      return
    }
    if (!HCL_LANG.has(model.getLanguageId())) return

    const before = model.getLineContent(pos.lineNumber).slice(0, pos.column - 1)
    if (!shouldRetriggerHclSuggest(before)) {
      lastFiredKey = ''
      return
    }

    const key = `${model.id}:${pos.lineNumber}:${pos.column}:${before.length}`
    if (key === lastFiredKey) return
    lastFiredKey = key

    // 下一 macrotask:等退格后的 model/cursor 落稳,且不打断当前输入法
    editor.trigger('hcl-suggest-retrigger', 'editor.action.triggerSuggest', {})
  }

  const schedule = () => {
    clearTimer()
    // 0ms 足够排在 Monaco 内部 delete 处理之后;比 rAF 更不容易丢
    timer = setTimeout(() => {
      timer = null
      tryFire()
    }, 0)
  }

  const dContent = editor.onDidChangeModelContent(() => {
    schedule()
  })
  // 鼠标点到 `var.` 后、或仅移动光标到引用点,也应弹出
  const dCursor = editor.onDidChangeCursorPosition((e) => {
    if (
      e.reason === monaco.editor.CursorChangeReason.Explicit ||
      e.reason === monaco.editor.CursorChangeReason.Undo ||
      e.reason === monaco.editor.CursorChangeReason.Redo ||
      e.reason === monaco.editor.CursorChangeReason.RecoverFromMarkers
    ) {
      // 内容未变时 lastFiredKey 可能挡住「关掉建议后再点回来」— 清 key 再 fire
      lastFiredKey = ''
      schedule()
    }
  })
  const dModel = editor.onDidChangeModel(() => {
    lastFiredKey = ''
  })

  return {
    dispose() {
      clearTimer()
      dContent.dispose()
      dCursor.dispose()
      dModel.dispose()
    },
  }
}
