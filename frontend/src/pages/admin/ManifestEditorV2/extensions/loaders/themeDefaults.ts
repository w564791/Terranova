/**
 * VS Code 默认主题扩展(codingame 已打包)。
 * 对应 marketplace 无单独插件,是 workbench 内置。
 */
export async function loadThemeDefaults(): Promise<void> {
  await import('@codingame/monaco-vscode-theme-defaults-default-extension')
}
