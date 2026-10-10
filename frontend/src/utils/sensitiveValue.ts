/**
 * 后端脱敏占位符(services.SensitivePlaceholder,393f665 / 1aed72a):
 * plan JSON 与资源变更里的敏感值一律替换为这个精确字符串(与 Terraform CLI 渲染一致)。
 * 前端只负责识别并以占位 chip 展示,不做任何客户端脱敏。
 */
export const SENSITIVE_PLACEHOLDER = '(sensitive value)'

export function isSensitivePlaceholder(v: unknown): boolean {
  return v === SENSITIVE_PLACEHOLDER
}

// 带引号(序列化后的字符串字面量)优先整体匹配,其次匹配裸文本
const SENSITIVE_RE = /"\(sensitive value\)"|\(sensitive value\)/g

/**
 * 把渲染文本按占位符切分:string 片段原样,null 表示此处渲染占位 chip。
 * 不含占位符时返回 [text]。
 */
export function splitSensitive(text: string): Array<string | null> {
  if (!text.includes(SENSITIVE_PLACEHOLDER)) return [text]
  const out: Array<string | null> = []
  let last = 0
  for (const m of text.matchAll(SENSITIVE_RE)) {
    const i = m.index ?? 0
    if (i > last) out.push(text.slice(last, i))
    out.push(null)
    last = i + m[0].length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}
