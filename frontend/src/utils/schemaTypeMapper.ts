/**
 * Schema类型映射工具
 * 将后端的数字类型映射为前端可识别的字符串类型
 * 不包含任何硬编码的业务逻辑，完全基于数据驱动
 */

// 后端ValueType枚举值（对应demo/types.go）
export const BackendValueType = {
  TypeInvalid: 0,
  TypeBool: 1,
  TypeInt: 2,
  TypeFloat: 3,
  TypeString: 4,
  TypeList: 5,
  TypeMap: 6,
  TypeSet: 7,
  TypeObject: 8,
  TypeJsonString: 9,
  TypeText: 10,
  TypeListObject: 11,
} as const;

// 前端使用的类型字符串
export type FrontendValueType = 
  | 'boolean'
  | 'number'
  | 'string'
  | 'array'
  | 'map'
  | 'object'
  | 'json'
  | 'text';

/**
 * 将后端的数字类型映射为前端字符串类型
 * @param backendType 后端返回的数字类型
 * @returns 前端可识别的字符串类型
 */
export function mapBackendTypeToFrontend(backendType: number): FrontendValueType {
  switch (backendType) {
    case BackendValueType.TypeBool:
      return 'boolean';
    
    case BackendValueType.TypeInt:
    case BackendValueType.TypeFloat:
      return 'number';
    
    case BackendValueType.TypeString:
      return 'string';
    
    case BackendValueType.TypeList:
    case BackendValueType.TypeSet:
    case BackendValueType.TypeListObject:
      return 'array';
    
    case BackendValueType.TypeMap:
      return 'map';
    
    case BackendValueType.TypeObject:
      return 'object';
    
    case BackendValueType.TypeJsonString:
      return 'json';
    
    case BackendValueType.TypeText:
      return 'text';
    
    default:
      return 'string'; // 默认返回string类型
  }
}

/**
 * 递归转换schema中的所有type字段
 * 将数字类型转换为字符串类型
 * @param schema 原始schema对象
 * @returns 转换后的schema对象
 */
export function transformSchemaTypes(schema: any): any {
  if (!schema || typeof schema !== 'object') {
    return schema;
  }

  // 如果是数组，递归处理每个元素
  if (Array.isArray(schema)) {
    return schema.map(item => transformSchemaTypes(item));
  }

  const transformed: any = {};
  
  for (const [key, value] of Object.entries(schema)) {
    if (key === 'type' && typeof value === 'number') {
      // 转换type字段
      transformed[key] = mapBackendTypeToFrontend(value);
    } else if (value && typeof value === 'object') {
      // 递归处理嵌套对象
      transformed[key] = transformSchemaTypes(value);
    } else {
      // 保持原值
      transformed[key] = value;
    }
  }
  
  return transformed;
}

/**
 * 处理从API获取的schema数据
 * @param apiResponse API返回的原始数据
 * @returns 处理后的schema数据
 */
export function processApiSchema(apiResponse: any): any {
  // 如果响应中包含schema_data字段，处理它
  if (apiResponse.schema_data) {
    return {
      ...apiResponse,
      schema_data: transformSchemaTypes(apiResponse.schema_data)
    };
  }
  
  // 否则直接转换整个响应
  return transformSchemaTypes(apiResponse);
}

/**
 * 判断字段是否允许用户自由添加键值对
 * 基于type和其他属性动态判断，而不是硬编码
 * @param fieldSchema 字段的schema定义
 * @returns 是否是可自由添加的Map类型
 */
export function isUserEditableMap(fieldSchema: any): boolean {
  // TypeMap: type=6 或 type='map'，且没有固定的properties
  const isMapType = fieldSchema.type === 6 || fieldSchema.type === 'map';
  const hasNoFixedProperties = !fieldSchema.properties;
  
  return isMapType && hasNoFixedProperties;
}

/**
 * 判断字段是否是固定结构的对象
 * @param fieldSchema 字段的schema定义
 * @returns 是否是固定结构的Object类型
 */
export function isFixedObject(fieldSchema: any): boolean {
  // TypeObject: type=8 或 type='object'，且有固定的properties
  const isObjectType = fieldSchema.type === 8 || fieldSchema.type === 'object';
  const hasFixedProperties = !!fieldSchema.properties;
  
  return isObjectType && hasFixedProperties;
}

/**
 * 获取字段的渲染提示信息
 * 基于schema属性动态生成，不硬编码
 * @param fieldSchema 字段的schema定义
 * @returns 渲染提示信息
 */
export function getFieldRenderHints(fieldSchema: any): {
  isRequired: boolean;
  isHidden: boolean;
  hasOptions: boolean;
  mustInclude: string[];
  defaultValue: any;
} {
  return {
    isRequired: fieldSchema.required === true,
    isHidden: fieldSchema.hidden_default === true,
    hasOptions: Array.isArray(fieldSchema.at_least_one_of) && fieldSchema.at_least_one_of.length > 0,
    mustInclude: Array.isArray(fieldSchema.must_include) ? fieldSchema.must_include : [],
    defaultValue: fieldSchema.default !== undefined ? fieldSchema.default : null,
  };
}
