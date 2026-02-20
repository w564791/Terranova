import api from './api';

// Schema V2 相关类型定义
export interface OpenAPISchema {
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
    ui?: UIConfig;
    external?: {
      sources?: ExternalSource[];
    };
  };
}

export interface PropertySchema {
  type: string;
  title?: string;
  description?: string;
  default?: unknown;
  enum?: string[];
  items?: { type: string; properties?: Record<string, PropertySchema> };
  properties?: Record<string, PropertySchema>;
  pattern?: string;
  format?: string;
  minItems?: number;
  maxItems?: number;
  minLength?: number;
  maxLength?: number;
  minimum?: number;
  maximum?: number;
  deprecated?: boolean;
  readOnly?: boolean;
  'x-sensitive'?: boolean;
  'x-deprecated-message'?: string;
  'x-validation'?: ValidationRule[];
}

export interface ValidationRule {
  condition: string;
  errorMessage: string;
  pattern?: string;
}

export interface UIConfig {
  fields: Record<string, FieldUIConfig>;
  groups: GroupConfig[];
  layout?: {
    type: string;
    position: string;
  };
}

export interface FieldUIConfig {
  label?: string;
  group?: string;
  widget?: string;
  help?: string;
  order?: number;
  source?: string;
  placeholder?: string;
  searchable?: boolean;
  allowCustom?: boolean;
  readonly?: boolean;
  hidden?: boolean;
  hiddenByDefault?: boolean;
  externalSource?: string;
  color?: string;
  refreshButton?: boolean;
  editWarning?: string;
  // Switch 组件专用配置 - 开关旁边的提示标签
  checkedHint?: string;        // 开关打开时显示的提示文本
  checkedHintColor?: string;   // 开关打开时提示的颜色（Ant Design Tag 颜色）
  uncheckedHint?: string;      // 开关关闭时显示的提示文本
  uncheckedHintColor?: string; // 开关关闭时提示的颜色（Ant Design Tag 颜色）
  // CMDB 数据源配置 - 从 CMDB 搜索资源作为选项
  cmdbSource?: {
    resourceType?: string;     // 资源类型过滤，如 aws_instance, aws_vpc
    valueField?: string;       // 选择后填入的字段：cloud_id, cloud_arn, cloud_name, cloud_region, cloud_account, terraform_address
  };
}

export interface GroupConfig {
  id: string;
  title: string;
  icon?: string;
  order: number;
  defaultExpanded?: boolean;
}

export interface ExternalSource {
  id: string;
  type: 'api' | 'static';
  api: string;
  method?: string;
  params?: Record<string, string>;
  cache?: { ttl: number };
  transform?: {
    type: string;
    expression: string;
  };
  dependsOn?: string[];
}

// Schema 模型
export interface SchemaV2 {
  id: number;
  module_id: number;
  version: string;
  status: string;
  schema_version: string;
  openapi_schema: OpenAPISchema | null;
  variables_tf: string;
  ui_config: UIConfig | null;
  source_type: string;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

// 请求类型
export interface ParseTFRequest {
  variables_tf?: string;
  outputs_tf?: string;
  module_name?: string;
  provider?: string;
  version?: string;
  layout?: 'top' | 'left';
}

export interface ParseTFResponse {
  openapi_schema: OpenAPISchema;
  field_count: number;
  basic_fields: number;
  advanced_fields: number;
  warnings?: string[];
}

export interface CreateSchemaV2Request {
  version: string;
  openapi_schema: OpenAPISchema;
  variables_tf?: string;
  status?: string;
  source_type?: string;
}

export interface UpdateSchemaV2Request {
  openapi_schema?: OpenAPISchema;
  ui_config?: UIConfig;
  variables_tf?: string;
  status?: string;
}

export interface SchemaFieldUpdateRequest {
  field_name: string;
  property: string;
  value: unknown;
}

// Schema V2 服务
// 注意：axios 拦截器返回 response.data，所以这里的返回类型是直接的数据类型
export const schemaV2Service = {
  // 解析 Terraform variables.tf 并生成 OpenAPI Schema
  parseTF: async (data: ParseTFRequest): Promise<ParseTFResponse> => {
    return api.post('/modules/parse-tf-v2', data);
  },

  // 获取模块的 V2 Schema
  // versionId 可选，不传则返回默认版本的 Schema
  getSchemaV2: async (moduleId: number, versionId?: string): Promise<SchemaV2> => {
    const params = versionId ? { version_id: versionId } : {};
    return api.get(`/modules/${moduleId}/schemas/v2`, { params });
  },

  // 获取模块的所有 Schema（包括 v1 和 v2）
  getAllSchemas: async (moduleId: number): Promise<SchemaV2[]> => {
    return api.get(`/modules/${moduleId}/schemas/all`);
  },

  // 创建 V2 Schema
  createSchemaV2: async (moduleId: number, data: CreateSchemaV2Request): Promise<SchemaV2> => {
    return api.post(`/modules/${moduleId}/schemas/v2`, data);
  },

  // 更新 V2 Schema
  updateSchemaV2: async (moduleId: number, schemaId: number, data: UpdateSchemaV2Request): Promise<SchemaV2> => {
    return api.put(`/modules/${moduleId}/schemas/v2/${schemaId}`, data);
  },

  // 更新单个字段的 UI 配置
  updateSchemaField: async (moduleId: number, schemaId: number, data: SchemaFieldUpdateRequest): Promise<SchemaV2> => {
    return api.patch(`/modules/${moduleId}/schemas/v2/${schemaId}/fields`, data);
  },

  // 将 v1 Schema 迁移到 v2
  migrateToV2: async (moduleId: number, schemaId: number): Promise<SchemaV2> => {
    return api.post(`/modules/${moduleId}/schemas/${schemaId}/migrate-v2`);
  },
};

// 辅助函数：检测 Schema 版本
export function detectSchemaVersion(schema: unknown): 'v1' | 'v2' {
  if (!schema || typeof schema !== 'object') {
    return 'v1';
  }
  
  const s = schema as Record<string, unknown>;
  
  // v2 Schema 特征：包含 openapi 字段
  if (s.openapi && typeof s.openapi === 'string' && (s.openapi as string).startsWith('3.')) {
    return 'v2';
  }
  
  // v2 Schema 特征：包含 x-iac-platform 扩展
  if (s['x-iac-platform']) {
    return 'v2';
  }
  
  // 默认为 v1
  return 'v1';
}

// 辅助函数：从 OpenAPI Schema 提取字段列表
export function extractFieldsFromSchema(schema: OpenAPISchema): Array<{
  name: string;
  property: PropertySchema;
  uiConfig: FieldUIConfig;
}> {
  const properties = schema.components?.schemas?.ModuleInput?.properties || {};
  const uiFields = schema['x-iac-platform']?.ui?.fields || {};
  
  return Object.entries(properties).map(([name, property]) => ({
    name,
    property,
    uiConfig: uiFields[name] || {},
  }));
}

// 辅助函数：获取字段的 Widget 类型
export function getWidgetType(property: PropertySchema, uiConfig: FieldUIConfig): string {
  // 检查是否有 CMDB 数据源（最高优先级）
  // CMDB 数据源可以用于 string 和 array 类型
  // 即使配置了 widget: "text"，只要有 cmdbSource 就使用 cmdb-select
  if (uiConfig.cmdbSource && uiConfig.cmdbSource.valueField) {
    return 'cmdb-select';
  }
  
  // 其次使用 UI 配置中的 widget
  if (uiConfig.widget) {
    return uiConfig.widget;
  }
  
  // 扩展属性检查
  const extProperty = property as PropertySchema & { 
    'x-dynamic-keys'?: boolean;
    'x-widget'?: string;
    additionalProperties?: unknown;
  };
  
  // 检查 x-widget 扩展属性
  if (extProperty['x-widget']) {
    return extProperty['x-widget'];
  }
  
  // 根据类型推断
  switch (property.type) {
    case 'boolean':
      return 'switch';
    case 'array':
      if (property.items?.type === 'object') {
        return 'object-list';
      }
      return 'tags';
    case 'object':
      // 检查是否是动态键对象 (CustomObject 类型)
      if (extProperty['x-dynamic-keys'] === true) {
        return 'dynamic-object';
      }
      // 检查是否有 additionalProperties 且没有 properties（map 类型）
      if (extProperty.additionalProperties && !property.properties) {
        // 如果 additionalProperties 是对象类型，使用 dynamic-object
        if (typeof extProperty.additionalProperties === 'object' && 
            (extProperty.additionalProperties as any).type === 'object') {
          return 'dynamic-object';
        }
        return 'key-value';
      }
      return 'object';
    case 'integer':
    case 'number':
      return 'number';
    default:
      // 检查是否有枚举
      if (property.enum && property.enum.length > 0) {
        return 'select';
      }
      // 检查是否有外部数据源
      if (uiConfig.source) {
        return 'select';
      }
      return 'text';
  }
}

// 辅助函数：按分组组织字段
export function groupFields(
  fields: Array<{ name: string; property: PropertySchema; uiConfig: FieldUIConfig }>,
  groups: GroupConfig[]
): Map<string, Array<{ name: string; property: PropertySchema; uiConfig: FieldUIConfig }>> {
  const grouped = new Map<string, Array<{ name: string; property: PropertySchema; uiConfig: FieldUIConfig }>>();
  
  // 初始化分组
  groups.forEach(group => {
    grouped.set(group.id, []);
  });
  
  // 分配字段到分组
  fields.forEach(field => {
    const groupId = field.uiConfig.group || 'advanced';
    const group = grouped.get(groupId);
    if (group) {
      group.push(field);
    } else {
      // 如果分组不存在，放入 advanced
      const advancedGroup = grouped.get('advanced');
      if (advancedGroup) {
        advancedGroup.push(field);
      }
    }
  });
  
  // 按 order 排序每个分组内的字段
  grouped.forEach((fields) => {
    fields.sort((a, b) => (a.uiConfig.order || 999) - (b.uiConfig.order || 999));
  });
  
  return grouped;
}

export default schemaV2Service;
