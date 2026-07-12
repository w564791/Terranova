/**
 * Terraform State 解析工具
 * 将 state content 解析为结构化的资源列表
 */

// 原始 Terraform State 资源结构
export interface TerraformResource {
  mode: 'data' | 'managed';
  type: string;
  name: string;
  module?: string;
  provider: string;
  instances: TerraformInstance[];
}

export interface TerraformInstance {
  index_key?: string | number;
  attributes: Record<string, any>;
  sensitive_attributes?: string[];
  schema_version?: number;
  dependencies?: string[];
  private?: string;
}

// 解析后的资源结构
export interface ParsedResource {
  address: string;           // 完整地址，如 "module.vpc.aws_vpc.main"
  mode: 'data' | 'managed';  // 资源模式
  type: string;              // 资源类型
  name: string;              // 资源名称
  module?: string;           // 模块路径
  provider: string;          // Provider
  instances: ParsedInstance[];
}

export interface ParsedInstance {
  indexKey?: string | number;  // 实例索引（for_each 或 count）
  attributes: Record<string, any>;
  sensitiveAttributes: string[];
  dependencies?: string[];
}

// 按模块分组的结构
export interface ModuleGroup {
  modulePath: string;        // 模块路径，如 "module.vpc" 或 "(root)"
  displayName: string;       // 显示名称
  resources: ParsedResource[];
  resourceCount: number;
  dataSourceCount: number;
  managedCount: number;
}

// State 内容结构
export interface StateContent {
  version: number;
  terraform_version: string;
  serial: number;
  lineage: string;
  resources: TerraformResource[];
  outputs?: Record<string, any>;
}

/**
 * 构建资源的完整地址
 */
export function buildResourceAddress(resource: TerraformResource, instanceIndex?: string | number): string {
  const parts: string[] = [];
  
  // 添加模块路径
  if (resource.module) {
    parts.push(resource.module);
  }
  
  // 添加资源类型和名称
  if (resource.mode === 'data') {
    parts.push(`data.${resource.type}.${resource.name}`);
  } else {
    parts.push(`${resource.type}.${resource.name}`);
  }
  
  // 添加实例索引
  if (instanceIndex !== undefined) {
    if (typeof instanceIndex === 'string') {
      parts[parts.length - 1] += `["${instanceIndex}"]`;
    } else {
      parts[parts.length - 1] += `[${instanceIndex}]`;
    }
  }
  
  return parts.join('.');
}

/**
 * 解析 State 资源为结构化数据
 */
export function parseStateResources(stateContent: StateContent): ParsedResource[] {
  if (!stateContent?.resources) {
    return [];
  }

  return stateContent.resources.map(resource => {
    const baseAddress = buildResourceAddress(resource);
    
    return {
      address: baseAddress,
      mode: resource.mode,
      type: resource.type,
      name: resource.name,
      module: resource.module,
      provider: resource.provider,
      instances: resource.instances.map(instance => ({
        indexKey: instance.index_key,
        attributes: instance.attributes || {},
        sensitiveAttributes: instance.sensitive_attributes || [],
        dependencies: instance.dependencies,
      })),
    };
  });
}

/**
 * 从模块路径中提取模块层级
 * 例如: "module.AWS_eks.module.complete[\"key\"]" -> ["module.AWS_eks", "module.complete[\"key\"]"]
 */
export function parseModulePath(modulePath: string): string[] {
  if (!modulePath) return [];
  
  const parts: string[] = [];
  let current = '';
  let depth = 0;
  
  for (let i = 0; i < modulePath.length; i++) {
    const char = modulePath[i];
    
    if (char === '[') {
      depth++;
      current += char;
    } else if (char === ']') {
      depth--;
      current += char;
    } else if (char === '.' && depth === 0 && current.startsWith('module.')) {
      // 检查下一个部分是否是 module
      const remaining = modulePath.slice(i + 1);
      if (remaining.startsWith('module.')) {
        parts.push(current);
        current = '';
      } else {
        current += char;
      }
    } else {
      current += char;
    }
  }
  
  if (current) {
    parts.push(current);
  }
  
  return parts;
}

/**
 * 获取模块的显示名称
 * 从 module.{provider}_{module_name}_{resource-name} 格式中提取 resource-name
 * 
 * 命名规则: {provider}_{module-name}_{resource-name}
 * - provider: AWS, GCP, Azure 等（大写开头）
 * - module-name: 模块名称（如 eks-nodegroup-exchang）
 * - resource-name: 用户自定义的资源名称（可能包含下划线）
 * 
 * 例如: 
 * - module.AWS_eks-nodegroup-exchang_ai-generated -> ai-generated
 * - module.AWS_eks-nodegroup-exchang_ddd-64d_clone_570404 -> ddd-64d_clone_570404
 */
export function getModuleDisplayName(modulePath: string): string {
  if (!modulePath) return 'Root Module';
  
  // 解析模块路径
  const parts = parseModulePath(modulePath);
  if (parts.length === 0) return modulePath;
  
  // 获取第一个模块部分（最外层）
  const firstPart = parts[0];
  // 移除 "module." 前缀
  const moduleName = firstPart.replace(/^module\./, '');
  
  // 尝试匹配 {provider}_{module-name}_{resource-name} 格式
  // provider 通常是大写字母开头（AWS, GCP, Azure 等）
  // module-name 通常包含连字符
  // resource-name 是剩余部分
  
  // 正则: ^([A-Z][A-Za-z0-9]*)_([a-z][a-z0-9-]*)_(.+)$
  // 匹配: AWS_eks-nodegroup-exchang_ddd-64d_clone_570404
  // 结果: provider=AWS, module=eks-nodegroup-exchang, resource=ddd-64d_clone_570404
  const match = moduleName.match(/^([A-Z][A-Za-z0-9]*)_([a-z][a-z0-9-]*)_(.+)$/);
  if (match) {
    return match[3]; // 返回 resource-name 部分
  }
  
  // 如果不匹配标准格式，尝试简单的分割
  // 找到第二个下划线的位置
  const firstUnderscoreIndex = moduleName.indexOf('_');
  if (firstUnderscoreIndex > 0) {
    const afterFirst = moduleName.substring(firstUnderscoreIndex + 1);
    const secondUnderscoreIndex = afterFirst.indexOf('_');
    if (secondUnderscoreIndex > 0) {
      return afterFirst.substring(secondUnderscoreIndex + 1);
    }
  }
  
  // 回退：返回整个模块名
  return moduleName;
}

/**
 * 从资源地址中提取简短的资源名称
 * 用于第一级显示
 */
export function extractResourceDisplayName(resource: ParsedResource): string {
  // 如果有模块路径，从模块路径提取
  if (resource.module) {
    return getModuleDisplayName(resource.module);
  }
  
  // 没有模块路径，使用资源名称
  return resource.name;
}

/**
 * 按模块分组资源
 */
export function groupResourcesByModule(resources: ParsedResource[]): ModuleGroup[] {
  const moduleMap = new Map<string, ParsedResource[]>();
  
  // 按模块路径分组
  resources.forEach(resource => {
    const modulePath = resource.module || '';
    if (!moduleMap.has(modulePath)) {
      moduleMap.set(modulePath, []);
    }
    moduleMap.get(modulePath)!.push(resource);
  });
  
  // 转换为 ModuleGroup 数组
  const groups: ModuleGroup[] = [];
  
  moduleMap.forEach((moduleResources, modulePath) => {
    const dataSourceCount = moduleResources.filter(r => r.mode === 'data').length;
    const managedCount = moduleResources.filter(r => r.mode === 'managed').length;
    
    groups.push({
      modulePath: modulePath || '(root)',
      displayName: getModuleDisplayName(modulePath),
      resources: moduleResources,
      resourceCount: moduleResources.length,
      dataSourceCount,
      managedCount,
    });
  });
  
  // 排序：root 模块在前，其他按路径排序
  groups.sort((a, b) => {
    if (a.modulePath === '(root)') return -1;
    if (b.modulePath === '(root)') return 1;
    return a.modulePath.localeCompare(b.modulePath);
  });
  
  return groups;
}

/**
 * 检查属性是否为敏感属性
 */
export function isSensitiveAttribute(path: string, sensitiveAttributes: string[]): boolean {
  return sensitiveAttributes.some(sensitive => {
    // 处理 Terraform 的敏感属性路径格式
    // 例如: ["password"] 或 ["config", "0", "secret"]
    try {
      const parsed = JSON.parse(sensitive);
      if (Array.isArray(parsed)) {
        return parsed.join('.') === path || parsed[0] === path;
      }
    } catch {
      // 如果不是 JSON 格式，直接比较
      return sensitive === path;
    }
    return false;
  });
}

/**
 * 遮蔽敏感属性值
 */
export function maskSensitiveValue(value: any): string {
  if (value === null || value === undefined) return '****';
  if (typeof value === 'string') return '****';
  if (typeof value === 'number') return '****';
  if (typeof value === 'boolean') return '****';
  if (Array.isArray(value)) return '[****]';
  if (typeof value === 'object') return '{****}';
  return '****';
}

/**
 * 格式化属性值用于显示
 */
export function formatAttributeValue(value: any, maxLength: number = 100): string {
  if (value === null) return 'null';
  if (value === undefined) return 'undefined';
  
  if (typeof value === 'string') {
    if (value.length > maxLength) {
      return `"${value.substring(0, maxLength)}..."`;
    }
    return `"${value}"`;
  }
  
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  
  if (Array.isArray(value)) {
    if (value.length === 0) return '[]';
    return `[${value.length} items]`;
  }
  
  if (typeof value === 'object') {
    const keys = Object.keys(value);
    if (keys.length === 0) return '{}';
    return `{${keys.length} keys}`;
  }
  
  return String(value);
}

/**
 * 获取资源类型的图标
 */
export function getResourceTypeIcon(mode: 'data' | 'managed'): string {
  return mode === 'data' ? 'data' : 'managed';
}

/**
 * 获取 Provider 的简短名称
 */
export function getProviderShortName(provider: string): string {
  // provider 格式: provider["registry.opentofu.org/hashicorp/aws"]
  const match = provider.match(/provider\["[^"]*\/([^"]+)"\]/);
  if (match) {
    return match[1];
  }
  return provider;
}

/**
 * 统计 State 资源信息
 */
export function getStateStats(resources: ParsedResource[]): {
  totalResources: number;
  dataSourceCount: number;
  managedCount: number;
  moduleCount: number;
  providerCount: number;
} {
  const modules = new Set<string>();
  const providers = new Set<string>();
  let dataSourceCount = 0;
  let managedCount = 0;
  
  resources.forEach(resource => {
    modules.add(resource.module || '(root)');
    providers.add(resource.provider);
    
    if (resource.mode === 'data') {
      dataSourceCount++;
    } else {
      managedCount++;
    }
  });
  
  return {
    totalResources: resources.length,
    dataSourceCount,
    managedCount,
    moduleCount: modules.size,
    providerCount: providers.size,
  };
}

/**
 * 搜索资源
 */
export function searchResources(
  resources: ParsedResource[],
  query: string,
  options?: {
    searchInAttributes?: boolean;
    caseSensitive?: boolean;
  }
): ParsedResource[] {
  if (!query.trim()) return resources;
  
  const { searchInAttributes = false, caseSensitive = false } = options || {};
  const searchQuery = caseSensitive ? query : query.toLowerCase();
  
  return resources.filter(resource => {
    const address = caseSensitive ? resource.address : resource.address.toLowerCase();
    const type = caseSensitive ? resource.type : resource.type.toLowerCase();
    const name = caseSensitive ? resource.name : resource.name.toLowerCase();
    
    // 搜索地址、类型、名称
    if (address.includes(searchQuery) || type.includes(searchQuery) || name.includes(searchQuery)) {
      return true;
    }
    
    // 可选：搜索属性值
    if (searchInAttributes) {
      return resource.instances.some(instance => {
        const attrStr = JSON.stringify(instance.attributes);
        const searchIn = caseSensitive ? attrStr : attrStr.toLowerCase();
        return searchIn.includes(searchQuery);
      });
    }
    
    return false;
  });
}

/**
 * 按类型过滤资源
 */
export function filterResourcesByType(
  resources: ParsedResource[],
  types: string[]
): ParsedResource[] {
  if (types.length === 0) return resources;
  return resources.filter(resource => types.includes(resource.type));
}

/**
 * 按模式过滤资源
 */
export function filterResourcesByMode(
  resources: ParsedResource[],
  mode: 'data' | 'managed' | 'all'
): ParsedResource[] {
  if (mode === 'all') return resources;
  return resources.filter(resource => resource.mode === mode);
}

/**
 * 获取所有唯一的资源类型
 */
export function getUniqueResourceTypes(resources: ParsedResource[]): string[] {
  const types = new Set<string>();
  resources.forEach(resource => types.add(resource.type));
  return Array.from(types).sort();
}
