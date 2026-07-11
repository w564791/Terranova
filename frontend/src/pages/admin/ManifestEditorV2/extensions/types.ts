/**
 * Manifest 编辑器「扩展」描述 —— 对标 VS Code extension 的声明式挂载。
 *
 * 设计目标:
 *  - 不做插件市场 / 运行时动态安装 UI
 *  - 通过改 registry.ts 的 enabled 列表即可"装/卸"扩展(代码级)
 *  - 扩展分能力档:browser 可直接跑 / 需要 LSP 后端 / 需要完整 extension host
 *
 * 与 HashiCorp 生态对应:
 *  - hashicorp.hcl        → grammar-only,browser 可跑(本仓库 vendored 自 hashicorp/syntax)
 *  - hashicorp.terraform  → 依赖 terraform-ls 原生进程,浏览器不能原样加载 VSIX 主逻辑
 *                           后续用 kind:"lsp-backend" 接到 Agent/API 侧 language server
 */
export type ExtensionKind =
  /** 纯前端:TextMate grammar / theme / snippets / 本地 provider */
  | 'browser'
  /** 需要后端 Language Server(WebSocket/HTTP) */
  | 'lsp-backend'
  /** 需要 WebWorker / LocalProcess extension host 跑 activate() */
  | 'extension-host'

export type ExtensionId =
  | 'theme-defaults'
  | 'hashicorp-syntax'
  | 'terranova-hcl-intel'
  // 预留:装上后改 registry 即可接入
  | 'hashicorp-terraform-ls'

export interface ExtensionDescriptor {
  id: ExtensionId
  /** 对标 marketplace itemName,方便文档对照 */
  marketplaceId?: string
  displayName: string
  kind: ExtensionKind
  description: string
  /** 自动更新数据源(grammar 仓库 / npm / vsix URL) */
  updateSource?: {
    type: 'github-raw' | 'npm' | 'vsix-url' | 'internal'
    /** 例如 hashicorp/syntax@main */
    ref: string
    notes?: string
  }
  /** 是否默认启用;registry 可覆盖 */
  defaultEnabled: boolean
  /**
   * 加载器:side-effect import 或 async 注册。
   * 必须幂等(HMR / 双 mount 安全)。
   */
  load: () => void | Promise<void>
}
