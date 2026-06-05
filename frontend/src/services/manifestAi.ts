/**
 * Manifest 编辑器 AI 服务 — 资源生成/修复 + 草稿检查
 *
 * 两个能力都走 SSE,与 /ai/form/generate-with-cmdb-skill-sse 协议一致:
 *   event: progress | complete | error
 *   data:  JSON(ProgressEvent)
 *
 * 这里抽出通用 SSE 解析器 consumeSSE,生成和检查共用。
 */

// ========== 通用 SSE 协议类型 ==========

export interface ManifestCompletedStep {
  name: string
  elapsed_ms: number
  used_skills?: string[]
}

export interface ManifestIssue {
  file: string
  line: number
  level: 'error' | 'warning' | 'info'
  message: string
}

export interface ManifestProgressEvent {
  type: 'progress' | 'complete' | 'error'
  step: number
  total_steps: number
  step_name: string
  message?: string
  elapsed_ms: number
  completed_steps?: ManifestCompletedStep[]
  // 完成时数据
  hcl?: string
  issues?: ManifestIssue[]
  usage_log_id?: string
  // 错误时数据
  error?: string
}

const getToken = (): string | null => localStorage.getItem('token')

/**
 * consumeSSE 通用 SSE 消费器
 *
 * 发起 POST 请求,逐块读取 ReadableStream,按 \n\n 分隔解析事件,
 * 对每个事件调用 onProgress。返回最后一个 complete/error 事件。
 */
async function consumeSSE(
  url: string,
  body: unknown,
  onProgress: (event: ManifestProgressEvent) => void,
  signal?: AbortSignal,
): Promise<ManifestProgressEvent> {
  const token = getToken()
  if (!token) throw new Error('未登录')

  const response = await fetch(url, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    },
    body: JSON.stringify(body),
    signal,
  })

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }

  const reader = response.body?.getReader()
  if (!reader) throw new Error('ReadableStream not supported')

  const decoder = new TextDecoder()
  let buffer = ''
  let finalEvent: ManifestProgressEvent | null = null

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })

      // 事件以双换行符分隔
      const blocks = buffer.split('\n\n')
      buffer = blocks.pop() || '' // 保留未完成的事件

      for (const block of blocks) {
        if (!block.trim()) continue

        let eventData = ''
        for (const line of block.split('\n')) {
          if (line.startsWith('data: ')) eventData = line.slice(6)
        }
        if (!eventData) continue

        try {
          const event = JSON.parse(eventData) as ManifestProgressEvent
          onProgress(event)
          if (event.type === 'complete' || event.type === 'error') {
            finalEvent = event
          }
        } catch (e) {
          console.error('[manifestAi] 解析 SSE 事件失败:', e, eventData)
        }
      }
    }
  } finally {
    reader.releaseLock()
  }

  if (!finalEvent) throw new Error('SSE 流结束但未返回最终事件')
  return finalEvent
}

// ========== 资源生成/修复 ==========

export interface GenerateResourceParams {
  description: string
  currentContent?: string // 当前选区或文件内容(修复时提供)
  contextIds?: {
    workspace_id?: string
    organization_id?: string
  }
}

export interface GenerateResourceResult {
  status: 'complete' | 'blocked'
  hcl?: string
  message?: string
  usageLogId?: string
}

/** 生成/修复 manifest 资源(SSE 实时进度) */
export async function generateManifestResource(
  params: GenerateResourceParams,
  onProgress: (event: ManifestProgressEvent) => void,
  signal?: AbortSignal,
): Promise<GenerateResourceResult> {
  const final = await consumeSSE(
    '/api/v1/ai/manifest/generate-resource-sse',
    {
      description: params.description,
      current_content: params.currentContent,
      context_ids: params.contextIds,
    },
    onProgress,
    signal,
  )

  if (final.type === 'error') {
    return { status: 'blocked', message: final.error || final.message || '生成失败' }
  }
  return {
    status: 'complete',
    hcl: final.hcl,
    message: final.message,
    usageLogId: final.usage_log_id,
  }
}

// ========== 草稿检查 ==========

export interface CheckDraftParams {
  filePath?: string
  content: string // 选区或当前文件内容
  startLine?: number // content 在文件中的起始行号(选区时>0,整文件为1)
  contextIds?: {
    workspace_id?: string
    organization_id?: string
  }
}

export interface CheckDraftResult {
  issues: ManifestIssue[]
  message?: string
  usageLogId?: string
}

/** 检查 manifest 草稿(SSE 实时进度) */
export async function checkManifestDraft(
  params: CheckDraftParams,
  onProgress: (event: ManifestProgressEvent) => void,
  signal?: AbortSignal,
): Promise<CheckDraftResult> {
  const final = await consumeSSE(
    '/api/v1/ai/manifest/check-sse',
    {
      file_path: params.filePath,
      content: params.content,
      start_line: params.startLine,
      context_ids: params.contextIds,
    },
    onProgress,
    signal,
  )

  if (final.type === 'error') {
    throw new Error(final.error || final.message || '检查失败')
  }
  return {
    issues: final.issues || [],
    message: final.message,
    usageLogId: final.usage_log_id,
  }
}
