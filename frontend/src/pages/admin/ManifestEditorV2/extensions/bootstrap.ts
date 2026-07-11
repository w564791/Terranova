/**
 * 按 registry 加载所有启用扩展。
 * 在 vscode services initialize 之后调用。
 */
import { resolveEnabledExtensions } from './registry'
import type { ExtensionId } from './types'

const loaded = new Set<ExtensionId>()

export async function bootstrapExtensions(): Promise<{ loaded: ExtensionId[]; failed: { id: ExtensionId; error: string }[] }> {
  const enabled = resolveEnabledExtensions()
  const failed: { id: ExtensionId; error: string }[] = []

  for (const ext of enabled) {
    if (loaded.has(ext.id)) continue
    try {
      await ext.load()
      loaded.add(ext.id)
      // eslint-disable-next-line no-console
      console.debug(`[manifest-ext] loaded: ${ext.id} (${ext.kind})`)
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      failed.push({ id: ext.id, error: msg })
      // eslint-disable-next-line no-console
      console.error(`[manifest-ext] failed: ${ext.id}`, err)
    }
  }

  return { loaded: [...loaded], failed }
}

export function listLoadedExtensionIds(): ExtensionId[] {
  return [...loaded]
}
