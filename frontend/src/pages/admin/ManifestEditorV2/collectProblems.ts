import * as monaco from 'monaco-editor'
import type { ProblemItem } from './ProblemsPanel'

/** 从 Monaco 全局 markers 收集问题列表(仅返回 pathFromUri 能解析的模型)。 */
export function collectManifestProblems(
  pathFromUri: (uri: monaco.Uri) => string | null,
): ProblemItem[] {
  const models = monaco.editor.getModels()
  const out: ProblemItem[] = []
  for (const model of models) {
    const path = pathFromUri(model.uri)
    if (!path) continue
    const markers = monaco.editor.getModelMarkers({ resource: model.uri })
    for (const m of markers) {
      out.push({
        path,
        severity: m.severity,
        message: m.message,
        startLineNumber: m.startLineNumber,
        startColumn: m.startColumn,
        endLineNumber: m.endLineNumber,
        endColumn: m.endColumn,
        owner: m.owner,
      })
    }
  }
  out.sort((a, b) => {
    if (a.severity !== b.severity) return b.severity - a.severity
    if (a.path !== b.path) return a.path.localeCompare(b.path)
    return a.startLineNumber - b.startLineNumber || a.startColumn - b.startColumn
  })
  return out
}
