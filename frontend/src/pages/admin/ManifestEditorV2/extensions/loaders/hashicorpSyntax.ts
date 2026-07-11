/**
 * HashiCorp 官方 TextMate 语法 —— 与 marketplace 插件同源:
 *  - hashicorp.hcl        (grammar only)
 *  - hashicorp.terraform  的高亮部分(grammar 来自 hashicorp/syntax)
 *
 * 不加载完整 VSIX(避免 activate/telemetry/desktop 依赖);
 * 用 monaco-vscode-api 的 registerExtension 注册 contributes.grammars,
 * 语法文件 vendored 在 assets/grammars/,脚本可自动更新。
 *
 * 语言 ID 对齐 HashiCorp 推荐映射:
 *  *.hcl    → hcl
 *  *.tf     → terraform
 *  *.tfvars → terraform-vars
 */
import {
  registerExtension,
  ExtensionHostKind,
} from '@codingame/monaco-vscode-api/extensions'

const EXT_ID_KEY = '__terranovaHashicorpSyntaxExt__'

export async function loadHashicorpSyntax(): Promise<void> {
  const g = globalThis as unknown as { [k: string]: boolean | undefined }
  if (g[EXT_ID_KEY]) return
  g[EXT_ID_KEY] = true

  const result = registerExtension(
    {
      name: 'hcl',
      displayName: 'HashiCorp HCL (Terranova vendored)',
      description:
        'TextMate grammars from hashicorp/syntax — same source as marketplace hashicorp.hcl / terraform highlighting',
      publisher: 'hashicorp',
      version: '0.6.0-terranova',
      engines: { vscode: '*' },
      categories: ['Programming Languages'],
      contributes: {
        languages: [
          {
            id: 'hcl',
            aliases: ['HCL', 'hcl'],
            extensions: ['.hcl'],
            configuration: './language-configuration.json',
          },
          {
            id: 'terraform',
            aliases: ['Terraform', 'terraform', 'tf'],
            extensions: ['.tf'],
            configuration: './language-configuration.json',
          },
          {
            id: 'terraform-vars',
            aliases: ['Terraform Vars', 'tfvars'],
            extensions: ['.tfvars'],
            configuration: './language-configuration.json',
          },
        ],
        grammars: [
          {
            language: 'hcl',
            scopeName: 'source.hcl',
            path: './syntaxes/hcl.tmGrammar.json',
          },
          {
            language: 'terraform',
            scopeName: 'source.hcl.terraform',
            path: './syntaxes/terraform.tmGrammar.json',
          },
          {
            // tfvars 与 terraform 共用 grammar(官方 extension 亦如此)
            language: 'terraform-vars',
            scopeName: 'source.hcl.terraform',
            path: './syntaxes/terraform.tmGrammar.json',
          },
        ],
      },
    },
    // grammar 贡献不需要 activate();LocalProcess 可 registerFileUrl
    ExtensionHostKind.LocalProcess,
  )

  // registerFileUrl 在 LocalProcess 结果上
  const local = result as {
    registerFileUrl: (path: string, url: string) => void
    whenReady: () => Promise<void>
  }

  local.registerFileUrl(
    './syntaxes/hcl.tmGrammar.json',
    new URL('../assets/grammars/hcl.tmGrammar.json', import.meta.url).toString(),
  )
  local.registerFileUrl(
    './syntaxes/terraform.tmGrammar.json',
    new URL('../assets/grammars/terraform.tmGrammar.json', import.meta.url).toString(),
  )
  local.registerFileUrl(
    './language-configuration.json',
    new URL('../assets/language-configuration.json', import.meta.url).toString(),
  )

  await local.whenReady()
}
