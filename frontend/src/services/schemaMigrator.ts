// Schema 迁移服务
// 将 V1 Schema 转换为 V2 OpenAPI Schema

import type { V1Schema, V1FieldSchema } from './schemaVersionDetector';
import { V1_TYPE_MAP, V1_TYPE_TO_WIDGET } from './schemaVersionDetector';
import type { OpenAPISchema, PropertySchema, FieldUIConfig } from './schemaV2';

/**
 * 将 V1 Schema 迁移到 V2 OpenAPI Schema
 */
export function migrateV1ToV2(
  v1Schema: V1Schema,
  moduleName: string = 'Module',
  moduleVersion: string = '1.0.0'
): OpenAPISchema {
  const properties: Record<string, PropertySchema> = {};
  const required: string[] = [];
  const uiFields: Record<string, FieldUIConfig> = {};

  // 遍历 V1 字段
  for (const [fieldName, v1Field] of Object.entries(v1Schema)) {
    // 转换属性
    const property = convertV1FieldToProperty(fieldName, v1Field);
    properties[fieldName] = property;

    // 收集必填字段
    if (v1Field.required) {
      required.push(fieldName);
    }

    // 生成 UI 配置
    const uiConfig = generateUIConfig(fieldName, v1Field);
    uiFields[fieldName] = uiConfig;
  }

  // 构建 V2 Schema
  const v2Schema: OpenAPISchema = {
    openapi: '3.1.0',
    info: {
      title: moduleName,
      version: moduleVersion,
      description: `Migrated from V1 Schema`,
    },
    components: {
      schemas: {
        ModuleInput: {
          type: 'object',
          properties,
          required: required.length > 0 ? required : undefined,
        },
      },
    },
    'x-iac-platform': {
      ui: {
        fields: uiFields,
        groups: [
          { id: 'basic', title: '基础配置', order: 1, defaultExpanded: true },
          { id: 'advanced', title: '高级配置', order: 2, defaultExpanded: false },
        ],
      },
    },
  };

  return v2Schema;
}

/**
 * 将 V1 字段转换为 V2 属性
 */
function convertV1FieldToProperty(fieldName: string, v1Field: V1FieldSchema): PropertySchema {
  const type = V1_TYPE_MAP[v1Field.type] || 'string';
  
  const property: PropertySchema = {
    type,
    title: v1Field.alias || fieldName,
    description: v1Field.description || '',
  };

  // 设置默认值
  if (v1Field.default !== undefined) {
    property.default = v1Field.default;
  }

  // 处理数组类型
  if (type === 'array' && v1Field.elem) {
    if (typeof v1Field.elem === 'object' && 'type' in v1Field.elem) {
      // 简单元素类型
      const elemType = V1_TYPE_MAP[(v1Field.elem as V1FieldSchema).type] || 'string';
      property.items = { type: elemType };
    } else {
      // 对象元素类型
      property.items = {
        type: 'object',
        properties: convertV1ElemToProperties(v1Field.elem as Record<string, V1FieldSchema>),
      };
    }
  }

  // 处理对象类型
  if (type === 'object' && v1Field.elem && typeof v1Field.elem === 'object') {
    if (!('type' in v1Field.elem)) {
      property.properties = convertV1ElemToProperties(v1Field.elem as Record<string, V1FieldSchema>);
    }
  }

  // 设置约束
  if (v1Field.max_items !== undefined) {
    property.maxItems = v1Field.max_items;
  }
  if (v1Field.min_items !== undefined) {
    property.minItems = v1Field.min_items;
  }

  // 敏感字段
  if (v1Field.sensitive) {
    property['x-sensitive'] = true;
  }

  // 弃用字段
  if (v1Field.deprecated) {
    property.deprecated = true;
    property['x-deprecated-message'] = v1Field.deprecated;
  }

  // 计算字段
  if (v1Field.computed) {
    property.readOnly = true;
  }

  return property;
}

/**
 * 转换 V1 elem 为 V2 properties
 */
function convertV1ElemToProperties(elem: Record<string, V1FieldSchema>): Record<string, PropertySchema> {
  const properties: Record<string, PropertySchema> = {};
  
  for (const [name, field] of Object.entries(elem)) {
    properties[name] = convertV1FieldToProperty(name, field);
  }
  
  return properties;
}

/**
 * 生成 UI 配置
 */
function generateUIConfig(fieldName: string, v1Field: V1FieldSchema): FieldUIConfig {
  const widget = V1_TYPE_TO_WIDGET[v1Field.type] || 'text';
  
  const uiConfig: FieldUIConfig = {
    label: v1Field.alias || fieldName,
    widget,
    help: v1Field.description || '',
  };

  // 分组：必填字段放基础配置，其他放高级配置
  uiConfig.group = v1Field.required ? 'basic' : 'advanced';

  // 隐藏默认
  if (v1Field.hidden_default) {
    uiConfig.hiddenByDefault = true;
  }

  // 敏感字段使用密码组件
  if (v1Field.sensitive) {
    uiConfig.widget = 'password';
  }

  // 动态数据源
  if (v1Field.dynamic) {
    uiConfig.externalSource = v1Field.dynamic;
  }

  // 只读（计算字段）
  if (v1Field.computed) {
    uiConfig.readonly = true;
  }

  // 颜色
  if (v1Field.color) {
    uiConfig.color = v1Field.color;
  }

  return uiConfig;
}

/**
 * 验证迁移结果
 */
export function validateMigration(v2Schema: OpenAPISchema): {
  valid: boolean;
  errors: string[];
  warnings: string[];
} {
  const errors: string[] = [];
  const warnings: string[] = [];

  // 检查基本结构
  if (!v2Schema.openapi) {
    errors.push('缺少 openapi 版本字段');
  }

  if (!v2Schema.components?.schemas?.ModuleInput) {
    errors.push('缺少 ModuleInput schema');
  }

  const properties = v2Schema.components?.schemas?.ModuleInput?.properties;
  if (!properties || Object.keys(properties).length === 0) {
    warnings.push('Schema 没有定义任何字段');
  }

  // 检查 UI 配置
  const uiFields = v2Schema['x-iac-platform']?.ui?.fields;
  if (properties && uiFields) {
    for (const fieldName of Object.keys(properties)) {
      if (!uiFields[fieldName]) {
        warnings.push(`字段 ${fieldName} 缺少 UI 配置`);
      }
    }
  }

  return {
    valid: errors.length === 0,
    errors,
    warnings,
  };
}

/**
 * 生成迁移报告
 */
export function generateMigrationReport(
  v1Schema: V1Schema,
  v2Schema: OpenAPISchema
): {
  totalFields: number;
  migratedFields: number;
  requiredFields: number;
  computedFields: number;
  sensitiveFields: number;
  deprecatedFields: number;
  complexFields: number;
} {
  const v1Fields = Object.entries(v1Schema);
  const v2Properties = v2Schema.components?.schemas?.ModuleInput?.properties || {};
  const v2Required = v2Schema.components?.schemas?.ModuleInput?.required || [];

  return {
    totalFields: v1Fields.length,
    migratedFields: Object.keys(v2Properties).length,
    requiredFields: v2Required.length,
    computedFields: v1Fields.filter(([_, f]) => f.computed).length,
    sensitiveFields: v1Fields.filter(([_, f]) => f.sensitive).length,
    deprecatedFields: v1Fields.filter(([_, f]) => f.deprecated).length,
    complexFields: v1Fields.filter(([_, f]) => f.elem !== undefined).length,
  };
}
