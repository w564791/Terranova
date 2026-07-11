/**
 * 预留:marketplace hashicorp.terraform 的智能部分(terraform-ls)。
 *
 * 不能在浏览器直接 import 官方 VSIX:
 *  - extension 依赖 Node/Desktop API + 捆绑的 terraform-ls 二进制
 *  - 正确路径:后端/Agent 跑 terraform-ls,前端用 LSP over WebSocket
 *
 * 启用方式(未来):
 *  1. registry 把 enabled 设为 true
 *  2. 实现下方 load() 连接 /api/.../terraform-ls/ws
 *  3. 用 monaco-languageclient 注册到 language: terraform / terraform-vars
 */
export async function loadHashicorpTerraformLs(): Promise<void> {
  // eslint-disable-next-line no-console
  console.info(
    '[manifest-ext] hashicorp-terraform-ls 已在 registry 启用,但 LSP 后端尚未接线。' +
      '请实现 loadHashicorpTerraformLs (WebSocket → terraform-ls)。',
  )
}
