/**
 * vscode-api 服务初始化(B2 模式: 只用 services,不挂 workbench)
 *
 * 关键约束:
 *  - initialize 全进程只能调一次,所以模块加载时执行,使用 promise 缓存
 *  - 只挂我们真正用得上的 services,workbench 相关一概不引入
 *
 * 装的 services:
 *  - configuration: tab size / wordWrap / files.autoSave 等 settings.json 体系
 *  - keybindings:   vscode 标准快捷键(Ctrl+/, Alt+Up/Down 等)
 *  - languages:     语言注册基础设施(provider 都靠这个)
 *  - textmate:      .tmLanguage 引擎,真精度高亮(对比 demo 的 Monarch 正则)
 *  - theme:         vscode 主题加载(配合 dark-plus 默认主题)
 *  - snippets:      vscode 真品 snippet 引擎(我们的 demo 插入会更稳)
 */
import { initialize as initializeMonacoServices, LogLevel } from '@codingame/monaco-vscode-api'
import getConfigurationServiceOverride from '@codingame/monaco-vscode-configuration-service-override'
import getKeybindingsServiceOverride from '@codingame/monaco-vscode-keybindings-service-override'
import getLanguagesServiceOverride from '@codingame/monaco-vscode-languages-service-override'
import getTextmateServiceOverride from '@codingame/monaco-vscode-textmate-service-override'
import getThemeServiceOverride from '@codingame/monaco-vscode-theme-service-override'
import getSnippetsServiceOverride from '@codingame/monaco-vscode-snippets-service-override'

// 默认主题扩展(自带 dark-plus / light-plus 等)
import '@codingame/monaco-vscode-theme-defaults-default-extension'

let initPromise: Promise<void> | null = null

/**
 * ensureVscodeServicesReady 必须在创建 Monaco 编辑器实例之前调用。
 * 多次调用 idempotent,内部只 initialize 一次。
 */
export function ensureVscodeServicesReady(): Promise<void> {
  if (initPromise) return initPromise

  initPromise = initializeMonacoServices(
    {
      ...getConfigurationServiceOverride(),
      ...getKeybindingsServiceOverride(),
      ...getLanguagesServiceOverride(),
      ...getTextmateServiceOverride(),
      ...getThemeServiceOverride(),
      ...getSnippetsServiceOverride(),
    },
    undefined,
    {
      productConfiguration: {
        nameShort: 'Terranova Manifest Editor',
        nameLong: 'Terranova Manifest Editor',
      },
      developmentOptions: {
        logLevel: LogLevel.Warning,
      },
    },
  ).then(() => {
    // 这里之后可以加全局 register 命令、加载 vsix 扩展等
  })

  return initPromise
}
