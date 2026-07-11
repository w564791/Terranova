# Manifest Editor 扩展框架

参考 [HashiCorp Terraform](https://marketplace.visualstudio.com/items?itemName=hashicorp.terraform) /
[HashiCorp HCL](https://marketplace.visualstudio.com/items?itemName=hashicorp.hcl) 的能力分层,
用**代码级注册表**管理扩展,而不是运行时插件市场。

## 怎么"安装"一个扩展?

改 [`registry.ts`](./registry.ts):

```ts
// 1) 把 defaultEnabled 改 true
// 2) 或设置白名单:
export const ENABLED_EXTENSION_IDS: ExtensionId[] | null = [
  'theme-defaults',
  'hashicorp-syntax',
  'terranova-hcl-intel',
  // 'hashicorp-terraform-ls', // 接好 LSP 后再开
]
```

新扩展:在 `loaders/` 写 `loadXxx()`,加入 `EXTENSION_CATALOG`。

## 能力档 (kind)

| kind | 浏览器能否直接跑 | 例子 |
|------|------------------|------|
| `browser` | 能 | TextMate 高亮、主题、自研补全 |
| `lsp-backend` | 不能单独跑 | terraform-ls(官方 terraform 插件的智能) |
| `extension-host` | 部分 | 需要 activate() 的真 VSIX |

## 与官方插件对照

| Marketplace | 我们怎么做 |
|-------------|------------|
| `hashicorp.hcl` | `hashicorp-syntax` loader,grammar 来自 [hashicorp/syntax](https://github.com/hashicorp/syntax) |
| `hashicorp.terraform` 高亮 | 同上 `terraform.tmGrammar.json` |
| `hashicorp.terraform` IntelliSense | `hashicorp-terraform-ls`(默认关),需后端 language server |
| `github.com/hashicorp/hcl` | **Go 解析库**,不是编辑器插件;fmt/validate 可放后端用 hclwrite |

## 自动更新 grammar

```bash
cd frontend && npm run update:hcl-grammar
```

脚本从 `hashicorp/syntax` 拉最新 `hcl.tmGrammar.json` / `terraform.tmGrammar.json`。

## 后续装完整 terraform 智能

1. Agent 或 API 进程启动 `terraform-ls`
2. 暴露 WebSocket LSP endpoint
3. 实现 `loaders/hashicorpTerraformLs.ts` 用 `monaco-languageclient` 连接
4. registry 打开 `hashicorp-terraform-ls`
5. 可选:关掉或降级 `terranova-hcl-intel` 避免双补全

## VSIX 路径(可选)

项目已依赖 `@codingame/monaco-vscode-rollup-vsix-plugin`。
若某扩展是纯 web/grammar VSIX,可:

1. 下载到 `frontend/extensions/foo.vsix`
2. vite 配 `vsixPlugin()`
3. loader 里 `import '../../../extensions/foo.vsix'`

`hashicorp.terraform` 完整 VSIX **不适合**这条路径(含原生二进制)。
