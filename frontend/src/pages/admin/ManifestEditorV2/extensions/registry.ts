/**
 * 扩展注册表 —— 改这里即可"安装/卸载"扩展(代码级,无市场 UI)。
 *
 * 使用:
 *  1. 把某项 defaultEnabled 改 true/false,或改 ENABLED 覆盖列表
 *  2. 需要新扩展时:写 loaders/xxx.ts + 在 CATALOG 加一条
 *  3. grammar 自动更新: npm run update:hcl-grammar
 */
import type { ExtensionDescriptor, ExtensionId } from './types'
import { loadThemeDefaults } from './loaders/themeDefaults'
import { loadHashicorpSyntax } from './loaders/hashicorpSyntax'
import { loadTerranovaIntel } from './loaders/terranovaIntel'
import { loadHashicorpTerraformLs } from './loaders/hashicorpTerraformLs'

/** 完整目录:所有已知扩展 */
export const EXTENSION_CATALOG: ExtensionDescriptor[] = [
  {
    id: 'theme-defaults',
    displayName: 'VS Code Theme Defaults',
    kind: 'browser',
    description: 'Dark+/Light+ 等内置主题',
    updateSource: { type: 'npm', ref: '@codingame/monaco-vscode-theme-defaults-default-extension' },
    defaultEnabled: true,
    load: loadThemeDefaults,
  },
  {
    id: 'hashicorp-syntax',
    marketplaceId: 'hashicorp.hcl + hashicorp.terraform(highlight)',
    displayName: 'HashiCorp HCL/Terraform Syntax',
    kind: 'browser',
    description:
      '官方 TextMate 高亮(hashicorp/syntax)。等同 marketplace hashicorp.hcl 的能力 + terraform 文件高亮。',
    updateSource: {
      type: 'github-raw',
      ref: 'hashicorp/syntax@main',
      notes: 'run: npm run update:hcl-grammar',
    },
    defaultEnabled: true,
    load: loadHashicorpSyntax,
  },
  {
    id: 'terranova-hcl-intel',
    displayName: 'Terranova HCL Intelligence',
    kind: 'browser',
    description: '平台自研补全/跳转/诊断/module demo(非 marketplace)',
    updateSource: { type: 'internal', ref: 'ManifestEditorV2/hcl*' },
    defaultEnabled: true,
    load: loadTerranovaIntel,
  },
  {
    id: 'hashicorp-terraform-ls',
    marketplaceId: 'hashicorp.terraform',
    displayName: 'HashiCorp Terraform Language Server',
    kind: 'lsp-backend',
    description:
      '对标 marketplace hashicorp.terraform 的 IntelliSense。需后端 terraform-ls,默认关闭。',
    updateSource: {
      type: 'npm',
      ref: 'hashicorp/terraform-ls releases',
      notes: '后端二进制升级;前端协议保持 LSP',
    },
    defaultEnabled: false,
    load: loadHashicorpTerraformLs,
  },
]

/**
 * 显式启用列表。
 * - null: 使用各扩展 defaultEnabled
 * - string[]: 仅启用列表中的 id(白名单,便于环境切换)
 */
export const ENABLED_EXTENSION_IDS: ExtensionId[] | null = null

export function resolveEnabledExtensions(): ExtensionDescriptor[] {
  if (ENABLED_EXTENSION_IDS) {
    const set = new Set(ENABLED_EXTENSION_IDS)
    return EXTENSION_CATALOG.filter((e) => set.has(e.id))
  }
  return EXTENSION_CATALOG.filter((e) => e.defaultEnabled)
}
