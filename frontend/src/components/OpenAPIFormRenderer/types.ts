// OpenAPI Form Renderer 类型定义

import type { PropertySchema, FieldUIConfig } from '../../services/schemaV2';

// Widget 通用属性
export interface WidgetProps {
  name: string;
  schema: PropertySchema & {
    required?: boolean;
    minLength?: number;
    maxLength?: number;
    minimum?: number;
    maximum?: number;
    pattern?: string;
    enum?: string[];
    items?: PropertySchema;
    'x-validation'?: Array<{
      condition?: string;
      errorMessage?: string;
      pattern?: string;
    }>;
  };
  uiConfig?: FieldUIConfig;
  value?: unknown;
  onChange?: (value: unknown) => void;
  disabled?: boolean;
  readOnly?: boolean;
  error?: string;
  context?: FormContext;
  initialValue?: unknown;  // 初始值，用于判断存量数据
}

// Module 节点信息（用于 Manifest 编辑器中的引用）
export interface ManifestModuleNode {
  id: string;
  instance_name: string;
  module_id?: number;
  module_source?: string;
  outputs?: ManifestModuleOutput[];
}

export interface ManifestModuleOutput {
  name: string;
  type?: string;
  description?: string;
}

// Manifest 上下文（用于 Module 间引用）
export interface ManifestContext {
  currentNodeId: string;  // 当前正在编辑的节点 ID
  connectedNodeIds?: string[];  // 已连线的节点 ID 列表（只有这些节点可以被引用）
  nodes: ManifestModuleNode[];  // 画布中所有的 Module 节点
  onAddEdge?: (sourceNodeId: string, targetNodeId: string, sourceOutput: string, targetInput: string) => void;
}

// Workspace 资源引用上下文（用于资源编辑时引用其他资源的 output）
export interface WorkspaceResourceContext {
  workspaceId: string;            // 当前 workspace ID
  currentResourceId: string;      // 当前正在编辑的资源 ID
  resources: WorkspaceResourceNode[];  // workspace 中的其他资源
  remoteData?: RemoteDataNode[];  // 远程数据引用（来自其他 workspace）
}

// 远程数据节点信息（用于引用其他 workspace 的 outputs）
export interface RemoteDataNode {
  remote_data_id: string;         // 远程数据 ID
  data_name: string;              // 数据名称（用于生成引用，如 local.{data_name}.xxx）
  source_workspace_id: string;    // 源 workspace ID
  source_workspace_name?: string; // 源 workspace 名称（用于显示）
  description?: string;           // 描述
  available_outputs?: RemoteDataOutput[];  // 可用的 outputs
}

// 远程数据 Output 信息
export interface RemoteDataOutput {
  key: string;                    // output 键名
  type?: string;                  // 类型
  sensitive: boolean;             // 是否敏感
  value?: unknown;                // 值（非敏感时可能返回）
}

// Workspace 资源节点信息
export interface WorkspaceResourceNode {
  id: string;                     // 资源 ID
  resource_name: string;          // 资源名称（用于显示）
  resource_type: string;          // 资源类型
  tf_module_key?: string;         // Terraform module key（用于引用，格式：{Provider}_{module-type}_{resource_name}）
  module_id?: number;             // 关联的 Module ID
  module_source?: string;         // Module 来源
  outputs?: ManifestModuleOutput[];  // 可用的 outputs
}

// 表单上下文
export interface FormContext {
  values: Record<string, unknown>;
  errors: Record<string, string>;
  touched: Record<string, boolean>;
  schema: OpenAPIFormSchema;
  providers?: Record<string, unknown>;
  workspace?: {
    id: string;
    name: string;
  };
  organization?: {
    id: string;
    name: string;
  };
  manifest?: ManifestContext;  // Manifest 编辑器上下文
  workspaceResource?: WorkspaceResourceContext;  // Workspace 资源引用上下文
  triggerValuesUpdate?: () => void;  // 手动触发值更新的回调（用于 Widget 在 setFieldsValue 后通知 FormRenderer）
}

// OpenAPI 表单 Schema
export interface OpenAPIFormSchema {
  openapi: string;
  info: {
    title: string;
    version: string;
    description?: string;
    'x-module-source'?: string;
    'x-provider'?: string;
  };
  components: {
    schemas: {
      ModuleInput: {
        type: string;
        properties: Record<string, PropertySchema>;
        required?: string[];
      };
    };
  };
  'x-iac-platform'?: {
    ui?: {
      fields?: Record<string, FieldUIConfig>;
      groups?: GroupConfig[];
      layout?: LayoutConfig;
    };
    validation?: ValidationConfig;
    cascade?: CascadeConfig;
    external?: ExternalConfig;
  };
}

// 分组配置
export interface GroupConfig {
  id: string;
  title: string;
  icon?: string;
  order?: number;
  defaultExpanded?: boolean;
  hiddenByDefault?: boolean;
  layout?: 'tabs' | 'accordion' | 'sections';
  level?: 'basic' | 'advanced';
}

// 布局配置
export interface LayoutConfig {
  type: 'tabs' | 'sections' | 'accordion';
  position?: 'top' | 'left';
}

// 验证配置
export interface ValidationConfig {
  rules?: ValidationRule[];
}

export interface ValidationRule {
  id: string;
  type: 'conflicts' | 'requiredWith' | 'exactlyOneOf' | 'atLeastOneOf';
  fields?: string[];
  trigger?: {
    field: string;
    operator: 'eq' | 'ne' | 'gt' | 'lt' | 'in' | 'notIn';
    value: unknown;
  };
  requires?: string[];
  message: string;
}

// 级联配置
export interface CascadeConfig {
  rules?: CascadeRule[];
}

export interface CascadeRule {
  id: string;
  description?: string;  // 规则描述
  trigger: {
    field: string;
    operator: 'eq' | 'ne' | 'gt' | 'lt' | 'gte' | 'lte' | 'in' | 'notIn' | 'empty' | 'notEmpty' | 'contains' | 'startsWith' | 'endsWith' | 'matches';
    value?: unknown;
  };
  actions: CascadeAction[];
}

export interface CascadeAction {
  type: 'show' | 'hide' | 'enable' | 'disable' | 'setValue' | 'setOptions' | 'setRequired' | 'clearValue' | 'reloadSource';
  fields?: string[];
  field?: string;
  value?: unknown;
  message?: string;
  required?: boolean;  // 用于 setRequired 动作
}

// 外部数据源配置
export interface ExternalConfig {
  sources?: ExternalDataSource[];
}

export interface ExternalDataSource {
  id: string;
  type: 'api' | 'static' | 'terraform';
  api?: string;
  method?: string;
  params?: Record<string, string>;
  cache?: {
    ttl: number;
    key?: string;
  };
  transform?: {
    type: 'jmespath' | 'jsonpath';
    expression: string;
  };
  dependsOn?: string[];
  data?: Array<{ value: string; label: string }>;
}

// 选项类型
export interface SelectOption {
  value: string | number;
  label: string;
  description?: string;
  disabled?: boolean;
  group?: string;
}

// Widget 类型映射
export type WidgetType = 
  | 'text'
  | 'textarea'
  | 'number'
  | 'select'
  | 'multi-select'
  | 'switch'
  | 'tags'
  | 'key-value'
  | 'object'
  | 'object-list'
  | 'dynamic-object'  // 动态键对象编辑器 (CustomObject 类型)
  | 'json-editor'
  | 'password'
  | 'datetime'
  | 'code-editor'     // 代码编辑器，支持多语言语法高亮
  | 'cmdb-select';    // CMDB 资源选择器，从 CMDB 搜索资源

// AI 助手配置
export interface AIAssistantConfig {
  enabled: boolean;
  moduleId: number;           // 必须传入，用于后端获取 Module 信息
  workspaceId?: string;       // 可选上下文
  organizationId?: string;
  manifestId?: string;
  position?: 'inline' | 'panel' | 'floating';
  capabilities?: ('generate' | 'suggest' | 'validate')[];
}

// 表单渲染器属性
export interface FormRendererProps {
  schema: OpenAPIFormSchema;
  initialValues?: Record<string, unknown>;
  onChange?: (values: Record<string, unknown>) => void;
  onSubmit?: (values: Record<string, unknown>) => void;
  disabled?: boolean;
  readOnly?: boolean;
  providers?: Record<string, unknown>;
  workspace?: { id: string; name: string };
  organization?: { id: string; name: string };
  manifest?: ManifestContext;  // Manifest 编辑器上下文
  workspaceResource?: WorkspaceResourceContext;  // Workspace 资源引用上下文
  activeGroupId?: string;  // 当前活跃的分组 ID（用于 tabs 布局的 URL 参数持久化）
  onGroupChange?: (groupId: string) => void;  // 分组切换回调
  aiAssistant?: AIAssistantConfig;  // AI 助手配置
}

// 字段渲染配置
export interface FieldRenderConfig {
  name: string;
  schema: PropertySchema;
  uiConfig: FieldUIConfig;
  widget: WidgetType;
  required: boolean;
  group: string;
  order: number;
  visible: boolean;
  disabled: boolean;
}
