// Schema 版本检测服务
// 用于自动识别 v1/v2 Schema 并选择对应渲染器

export type SchemaVersion = 'v1' | 'v2' | 'unknown';

/**
 * 检测 Schema 版本
 * @param schema Schema 数据
 * @returns Schema 版本
 */
export function detectSchemaVersion(schema: unknown): SchemaVersion {
  if (!schema || typeof schema !== 'object') {
    return 'unknown';
  }

  const schemaObj = schema as Record<string, unknown>;

  // v2 Schema 特征：包含 openapi 字段
  if (schemaObj.openapi && typeof schemaObj.openapi === 'string') {
    const version = schemaObj.openapi as string;
    if (version.startsWith('3.')) {
      return 'v2';
    }
  }

  // v2 Schema 特征：包含 x-iac-platform 扩展
  if (schemaObj['x-iac-platform']) {
    return 'v2';
  }

  // v2 Schema 特征：包含 components.schemas.ModuleInput
  if (schemaObj.components && typeof schemaObj.components === 'object') {
    const components = schemaObj.components as Record<string, unknown>;
    if (components.schemas && typeof components.schemas === 'object') {
      const schemas = components.schemas as Record<string, unknown>;
      if (schemas.ModuleInput) {
        return 'v2';
      }
    }
  }

  // v1 Schema 特征：直接包含字段定义（type 为数字）
  const hasV1Fields = Object.values(schemaObj).some(value => {
    if (value && typeof value === 'object') {
      const field = value as Record<string, unknown>;
      // v1 使用数字类型
      return typeof field.type === 'number';
    }
    return false;
  });

  if (hasV1Fields) {
    return 'v1';
  }

  // 检查是否有 v1 的典型字段结构
  const hasV1Structure = Object.values(schemaObj).some(value => {
    if (value && typeof value === 'object') {
      const field = value as Record<string, unknown>;
      // v1 特征字段
      return 'computed' in field || 'force_new' in field || 'elem' in field;
    }
    return false;
  });

  if (hasV1Structure) {
    return 'v1';
  }

  return 'unknown';
}

/**
 * 检查是否为 v2 Schema
 */
export function isV2Schema(schema: unknown): boolean {
  return detectSchemaVersion(schema) === 'v2';
}

/**
 * 检查是否为 v1 Schema
 */
export function isV1Schema(schema: unknown): boolean {
  return detectSchemaVersion(schema) === 'v1';
}

/**
 * 获取 Schema 版本信息
 */
export function getSchemaVersionInfo(schema: unknown): {
  version: SchemaVersion;
  canMigrate: boolean;
  features: string[];
} {
  const version = detectSchemaVersion(schema);
  
  const features: string[] = [];
  const schemaObj = schema as Record<string, unknown>;

  if (version === 'v2') {
    features.push('OpenAPI 3.x 规范');
    if (schemaObj['x-iac-platform']) {
      const platform = schemaObj['x-iac-platform'] as Record<string, unknown>;
      if (platform.ui) features.push('UI 配置');
      if (platform.validation) features.push('验证规则');
      if (platform.cascade) features.push('级联规则');
      if (platform.external) features.push('外部数据源');
    }
  } else if (version === 'v1') {
    features.push('传统 Schema 格式');
    // 检查 v1 特性
    const hasComputed = Object.values(schemaObj).some(v => 
      v && typeof v === 'object' && (v as Record<string, unknown>).computed
    );
    if (hasComputed) features.push('计算字段');
  }

  return {
    version,
    canMigrate: version === 'v1',
    features,
  };
}

// V1 Schema 类型定义（用于迁移）
export interface V1Schema {
  [fieldName: string]: V1FieldSchema;
}

export interface V1FieldSchema {
  type: number; // 0-12 的数字类型
  required?: boolean;
  computed?: boolean;
  force_new?: boolean;
  default?: unknown;
  description?: string;
  elem?: V1FieldSchema | { [key: string]: V1FieldSchema };
  max_items?: number;
  min_items?: number;
  sensitive?: boolean;
  deprecated?: string;
  alias?: string;
  hidden_default?: boolean;
  single_choice?: boolean;
  dynamic?: string;
  color?: string;
}

// V1 类型映射
export const V1_TYPE_MAP: Record<number, string> = {
  0: 'invalid',
  1: 'boolean',
  2: 'integer',
  3: 'number',
  4: 'string',
  5: 'array',
  6: 'object',
  7: 'array', // set
  8: 'object',
  9: 'string', // json string
  10: 'string', // text
  11: 'array', // list object
  12: 'object', // custom object
};

// V1 类型到 Widget 映射
export const V1_TYPE_TO_WIDGET: Record<number, string> = {
  0: 'text',
  1: 'switch',
  2: 'number',
  3: 'number',
  4: 'text',
  5: 'tags',
  6: 'key-value',
  7: 'tags',
  8: 'object',
  9: 'json-editor',
  10: 'textarea',
  11: 'object-list',
  12: 'object',
};
