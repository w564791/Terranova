// OpenAPI Form Renderer 组件导出
// 基于 OpenAPI v3 规范的动态表单渲染器

// 主组件
export { default as FormRenderer } from './FormRenderer';
export { default } from './FormRenderer';

// Widget 组件
export * from './widgets';

// 类型定义
export type {
  WidgetProps,
  FormContext,
  OpenAPIFormSchema,
  GroupConfig,
  LayoutConfig,
  ValidationConfig,
  ValidationRule,
  CascadeConfig,
  CascadeRule,
  CascadeAction,
  ExternalConfig,
  ExternalDataSource,
  SelectOption,
  WidgetType,
  FormRendererProps,
  FieldRenderConfig,
} from './types';
