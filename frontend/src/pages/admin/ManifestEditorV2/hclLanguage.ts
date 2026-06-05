/**
 * HCL (HashiCorp Configuration Language) 语言注册 + Monarch 高亮
 *
 * 路径选择: Monaco Monarch (vs textmate)
 *  - Monarch 简单, ~80 行覆盖 95% HCL 语法
 *  - 不引入 onigasm WASM / .tmLanguage 文件 / vscode-api textmate service
 *  - 真复杂 HCL (heredoc / 嵌套表达式) 准确度略差,但实用够用
 *  - 后续可平滑升级到 textmate, provider 代码不变
 *
 * 移植自 manifest-vscode-mockup.html demo, 一比一复刻。
 */
import * as monaco from 'monaco-editor'

let registered = false

export function registerHclLanguage(): void {
  if (registered) return
  registered = true

  monaco.languages.register({
    id: 'hcl',
    extensions: ['.tf', '.tfvars', '.hcl'],
    aliases: ['Terraform', 'HCL'],
  })

  monaco.languages.setLanguageConfiguration('hcl', {
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
  })

  monaco.languages.setMonarchTokensProvider('hcl', {
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
  })
}
