/**
 * Terranova 自研 HCL/Terraform 智能(补全/跳转/诊断/平台 module)。
 * 包装成"扩展",与 HashiCorp grammar 并列挂在 registry 里;
 * 未来可整段替换为 terraform-ls 后端而不改编辑器壳。
 *
 * 注意:真正 register* 仍由 ManifestEditorV2 在拿到 defIndex 后调用,
 * 这里只做标记/文档角色;load 为空操作,避免与编辑器初始化时序打架。
 */
export async function loadTerranovaIntel(): Promise<void> {
  // no-op: providers 依赖编辑器侧 getIndex 闭包,由 ManifestEditorV2 挂载
}
