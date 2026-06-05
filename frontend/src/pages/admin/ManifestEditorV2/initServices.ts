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
import getConfigurationServiceOverride, {
  initUserConfiguration,
} from '@codingame/monaco-vscode-configuration-service-override'
import getKeybindingsServiceOverride from '@codingame/monaco-vscode-keybindings-service-override'
import getLanguagesServiceOverride from '@codingame/monaco-vscode-languages-service-override'
import getTextmateServiceOverride from '@codingame/monaco-vscode-textmate-service-override'
import getThemeServiceOverride from '@codingame/monaco-vscode-theme-service-override'
import getSnippetsServiceOverride from '@codingame/monaco-vscode-snippets-service-override'

// 默认主题扩展(自带 dark-plus / light-plus 等)
import '@codingame/monaco-vscode-theme-defaults-default-extension'

// Manifest 编辑器默认 vscode 用户配置
const DEFAULT_USER_CONFIG = JSON.stringify({
  // 主题: 必须用 vscode 内置主题真实 ID, 别瞎写 'Default Dark Modern'
  // 'Default Dark+' 是 dark-plus 主题的官方 displayName
  'workbench.colorTheme': 'Default Dark+',
  'editor.fontSize': 13,
  'editor.tabSize': 2,
  'editor.insertSpaces': true,
  'editor.minimap.enabled': true,
  'editor.renderWhitespace': 'selection',
  'editor.bracketPairColorization.enabled': true,
  'files.autoSave': 'off', // 我们自己用 1s debounce 走 PUT API
  'editor.semanticHighlighting.enabled': true,
})

// Monaco Worker 注册: vscode-api 不会自动配置 MonacoEnvironment.getWorker,
// 我们用 Vite 的 ?worker 后缀让 Vite 自己 bundle worker 文件。
// 不写这段会报 "Could not create web worker(s). Falling back to main thread"
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'

if (typeof window !== 'undefined') {
  ;(window as unknown as { MonacoEnvironment: object }).MonacoEnvironment = {
    getWorker(_moduleId: string, _label: string) {
      // 我们目前不启用 ts/json/css/html 等专属 worker (用不到),
      // 全部 fallback 到 editor.worker
      return new EditorWorker()
    },
  }
}

// 模块级单例(全进程一份)。React 19 严格模式 + HMR 都会重复 mount,
// 这里必须保证 initialize() 只被调用一次,否则 vscode-api 会抛
// "Services are already initialized"。
let initPromise: Promise<void> | null = null

/**
 * ensureVscodeServicesReady 必须在创建 Monaco 编辑器实例之前调用。
 * 多次调用 idempotent,内部只 initialize 一次。
 */
export function ensureVscodeServicesReady(): Promise<void> {
  if (initPromise) return initPromise

  // 关键: configuration override 必须最先就位 + 在 initialize *之前* 写入
  // 用户配置, 否则 theme service 启动时拿不到 colorTheme 设置, 默认走白色 vs theme
  initPromise = (async () => {
    // 注意: HMR 替换本模块时,模块级 initPromise 会被重置为 null,但 vscode-api
    // 的全局状态不会。因此第二次调 initializeMonacoServices 会抛
    // "Services are already initialized"。捕获这种错误当作 idempotent 成功。
    try {
      await initializeMonacoServices(
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
      )
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      if (msg.includes('already initialized')) {
        // HMR 重入或 React 严格模式双 mount,vscode-api 已 init 过,跳过
        // eslint-disable-next-line no-console
        console.debug('[ManifestEditorV2] vscode-api already initialized, skipping')
      } else {
        throw err
      }
    }
    // 写入默认配置(含 colorTheme), theme service 会自动 reload。
    // initUserConfiguration 是写文件操作, 重入安全。
    try {
      await initUserConfiguration(DEFAULT_USER_CONFIG)
    } catch {
      // 同上, 配置已存在不报错
    }
  })()

  return initPromise
}
