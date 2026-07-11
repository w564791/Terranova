/**
 * HCL / Terraform 语言注册
 *
 * 语言 ID 对齐 HashiCorp 官方映射(hashicorp/syntax + marketplace 插件):
 *  - hcl            → *.hcl
 *  - terraform      → *.tf
 *  - terraform-vars → *.tfvars
 *
 * 高亮优先级:
 *  1. TextMate(extensions/loaders/hashicorpSyntax, 官方 grammar)
 *  2. Monarch 兜底(TextMate 未就绪或失败时仍可读)
 *
 * Provider(补全/跳转)注册在 HCL_LANGUAGE_IDS 全部 id 上。
 */
import * as monaco from 'monaco-editor'

/** 编辑器/Provider 统一使用的语言 id 列表 */
export const HCL_LANGUAGE_IDS = ['hcl', 'terraform', 'terraform-vars'] as const
export type HclLanguageId = (typeof HCL_LANGUAGE_IDS)[number]

let registered = false

export function registerHclLanguage(): void {
  if (registered) return
  registered = true

  // 官方映射:分语言注册,便于 TextMate grammar 绑定
  monaco.languages.register({
    id: 'hcl',
    extensions: ['.hcl'],
    aliases: ['HCL', 'hcl'],
  })
  monaco.languages.register({
    id: 'terraform',
    extensions: ['.tf'],
    aliases: ['Terraform', 'terraform', 'tf'],
  })
  monaco.languages.register({
    id: 'terraform-vars',
    extensions: ['.tfvars'],
    aliases: ['Terraform Vars', 'tfvars'],
  })

  const langConfig: monaco.languages.LanguageConfiguration = {
    comments: { lineComment: '#', blockComment: ['/*', '*/'] },
    brackets: [
      ['{', '}'],
      ['[', ']'],
      ['(', ')'],
    ],
    autoClosingPairs: [
      { open: '{', close: '}' },
      { open: '[', close: ']' },
      { open: '(', close: ')' },
      { open: '"', close: '"' },
      { open: "'", close: "'" },
    ],
    surroundingPairs: [
      { open: '{', close: '}' },
      { open: '[', close: ']' },
      { open: '(', close: ')' },
      { open: '"', close: '"' },
    ],
  }

  for (const id of HCL_LANGUAGE_IDS) {
    monaco.languages.setLanguageConfiguration(id, langConfig)
    // Monarch 兜底:TextMate 加载后会覆盖 tokenizer,失败时仍有高亮
    monaco.languages.setMonarchTokensProvider(id, monarchHcl())
  }
}

function monarchHcl(): monaco.languages.IMonarchLanguage {
  return {
    defaultToken: '',
    keywords: [
      'resource', 'module', 'variable', 'output', 'data', 'locals', 'provider', 'terraform',
      'for_each', 'count', 'depends_on', 'dynamic', 'content', 'lifecycle', 'provisioner',
      'connection', 'required_version', 'required_providers', 'source', 'version', 'backend',
      'true', 'false', 'null',
    ],
    operators: ['=', '==', '!=', '<', '>', '<=', '>=', '+', '-', '*', '/', '%', '&&', '||', '!', '?', ':'],
    symbols: /[=><!~?:&|+\-*/^%]+/,
    escapes: /\\(?:[abfnrtv\\"']|x[0-9A-Fa-f]{1,4}|u[0-9A-Fa-f]{4}|U[0-9A-Fa-f]{8})/,
    tokenizer: {
      root: [
        [
          /[a-zA-Z_][\w-]*/,
          {
            cases: {
              '@keywords': 'keyword',
              '@default': 'identifier',
            },
          },
        ],
        { include: '@whitespace' },
        [/[{}()[\]]/, '@brackets'],
        [
          /@symbols/,
          {
            cases: {
              '@operators': 'operator',
              '@default': '',
            },
          },
        ],
        [/[0-9]+(\.[0-9]+)?/, 'number'],
        [/"([^"\\]|\\.)*$/, 'string.invalid'],
        [/"/, { token: 'string.quote', bracket: '@open', next: '@string' }],
      ],
      whitespace: [
        [/[ \t\r\n]+/, ''],
        [/#.*$/, 'comment'],
        [/\/\/.*$/, 'comment'],
        [/\/\*/, 'comment', '@blockComment'],
      ],
      blockComment: [
        [/[^/*]+/, 'comment'],
        [/\*\//, 'comment', '@pop'],
        [/[/*]/, 'comment'],
      ],
      string: [
        [/\$\{/, { token: 'delimiter.bracket', next: '@interpolation' }],
        [/[^\\"$]+/, 'string'],
        [/@escapes/, 'string.escape'],
        [/\\./, 'string.escape.invalid'],
        [/"/, { token: 'string.quote', bracket: '@close', next: '@pop' }],
      ],
      interpolation: [
        [/\}/, { token: 'delimiter.bracket', next: '@pop' }],
        { include: 'root' },
      ],
    },
  }
}
