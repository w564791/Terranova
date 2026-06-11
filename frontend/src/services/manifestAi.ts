/**
 * Manifest 编辑器 AI 服务 — 资源生成/修复 + 草稿检查
 *
 * 两个能力都走 SSE,与 /ai/form/generate-with-cmdb-skill-sse 协议一致:
 *   event: progress | complete | error
 *   data:  JSON(ProgressEvent)
 *
 * 这里抽出通用 SSE 解析器 consumeSSE,生成和检查共用。
 */

import api from './api'

// ========== 通用 SSE 协议类型 ==========

export interface ManifestCompletedStep {
  name: string
  elapsed_ms: number
  used_skills?: string[]
}

/** AI 给出的结构化修复:按行范围替换目标文件内容 */
export interface ManifestFix {
  file: string
  start_line: number
  end_line: number
  new_text: string
}

export interface ManifestIssue {
  file: string
  line: number
  level: 'error' | 'warning' | 'info'
  message: string
  fix?: ManifestFix // 可修复项才有
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
  warnings?: string[]
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
  sessionId?: string // 非空则本次交互落入该会话
  contextIds?: {
    workspace_id?: string
    organization_id?: string
  }
}

export interface GenerateResourceResult {
  status: 'complete' | 'blocked'
  hcl?: string
  message?: string
  warnings?: string[]
  usageLogId?: string
  completedSteps?: ManifestCompletedStep[]
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
      session_id: params.sessionId,
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
    warnings: final.warnings,
    usageLogId: final.usage_log_id,
    completedSteps: final.completed_steps,
  }
}

// ========== 草稿检查 ==========

/** 单个待检查文件(跨文件检查时多于一个) */
export interface CheckFilePayload {
  path: string
  content: string
  start_line: number // content 在文件中的起始行(整文件=1,选区=选区起始行)
}

export interface CheckDraftParams {
  files: CheckFilePayload[]
  sessionId?: string // 非空则本次检查落入该会话
  contextIds?: {
    workspace_id?: string
    organization_id?: string
  }
}

export interface CheckDraftResult {
  issues: ManifestIssue[]
  message?: string
  usageLogId?: string
  completedSteps?: ManifestCompletedStep[]
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
      files: params.files,
      session_id: params.sessionId,
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
    completedSteps: final.completed_steps,
  }
}

// ========== AI 会话(按 manifest + 用户隔离,后端持久化)==========

export interface ManifestAISession {
  id: string
  manifest_id: string
  org_id: string
  user_id: string
  title: string
  created_at: string
  updated_at: string
}

export interface ManifestAIMessage {
  id: string
  session_id: string
  role: 'user' | 'assistant'
  kind: 'generate' | 'check'
  content: string // JSON 字符串:生成 {description}/{hcl,warnings};检查 {files}/{issues,message}
  created_at: string
}

/** 列出当前用户在某 manifest 下的会话(updated_at 倒序)*/
export async function listAISessions(manifestId: string, orgId?: string): Promise<ManifestAISession[]> {
  const qs = `manifest_id=${encodeURIComponent(manifestId)}${orgId ? `&org_id=${encodeURIComponent(orgId)}` : ''}`
  const body = (await api.get(`/ai/manifest/sessions?${qs}`)) as { sessions?: ManifestAISession[] }
  return body.sessions || []
}

/** 新建会话 */
export async function createAISession(
  manifestId: string,
  orgId?: string,
  title?: string,
): Promise<ManifestAISession> {
  return (await api.post('/ai/manifest/sessions', {
    manifest_id: manifestId,
    org_id: orgId,
    title,
  })) as ManifestAISession
}

/** 拉某会话的消息(时间正序)*/
export async function getAISessionMessages(sessionId: string): Promise<ManifestAIMessage[]> {
  const body = (await api.get(`/ai/manifest/sessions/${encodeURIComponent(sessionId)}/messages`)) as {
    messages?: ManifestAIMessage[]
  }
  return body.messages || []
}

/** 删除会话 */
export async function deleteAISession(sessionId: string): Promise<void> {
  await api.delete(`/ai/manifest/sessions/${encodeURIComponent(sessionId)}`)
}
