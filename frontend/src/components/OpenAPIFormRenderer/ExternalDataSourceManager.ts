/**
 * 外部数据源管理器
 * 
 * 功能：
 * 1. 管理外部数据源的加载和缓存
 * 2. 支持 API 数据源、静态数据源
 * 3. 支持变量替换（providers、fields、workspace、organization）
 * 4. 支持依赖刷新（当依赖字段变化时重新加载）
 * 5. 支持 JMESPath 数据转换
 */

import type { ExternalDataSource, SelectOption, FormContext } from './types';

// 缓存项
interface CacheItem {
  data: SelectOption[];
  timestamp: number;
  key: string;
}

// 数据源加载状态
export interface DataSourceState {
  loading: boolean;
  error?: string;
  data: SelectOption[];
}

// 数据源管理器配置
export interface DataSourceManagerConfig {
  sources: ExternalDataSource[];
  context: FormContext;
  onStateChange?: (sourceId: string, state: DataSourceState) => void;
}

export class ExternalDataSourceManager {
  private sources: Map<string, ExternalDataSource> = new Map();
  private cache: Map<string, CacheItem> = new Map();
  private states: Map<string, DataSourceState> = new Map();
  private context: FormContext;
  private onStateChange?: (sourceId: string, state: DataSourceState) => void;
  private pendingRequests: Map<string, Promise<SelectOption[]>> = new Map();

  constructor(config: DataSourceManagerConfig) {
    this.context = config.context;
    this.onStateChange = config.onStateChange;
    
    // 注册数据源
    config.sources.forEach(source => {
      this.sources.set(source.id, source);
      this.states.set(source.id, { loading: false, data: [] });
    });
  }

  /**
   * 更新上下文
   */
  updateContext(context: FormContext): void {
    this.context = context;
  }

  /**
   * 获取数据源状态
   */
  getState(sourceId: string): DataSourceState {
    return this.states.get(sourceId) || { loading: false, data: [] };
  }

  /**
   * 加载数据源
   */
  async loadSource(sourceId: string, forceRefresh = false): Promise<SelectOption[]> {
    const source = this.sources.get(sourceId);
    if (!source) {
      console.warn(`数据源 ${sourceId} 不存在`);
      return [];
    }

    // 检查缓存
    if (!forceRefresh) {
      const cached = this.getCachedData(source);
      if (cached) {
        return cached;
      }
    }

    // 检查是否有正在进行的请求
    const pendingRequest = this.pendingRequests.get(sourceId);
    if (pendingRequest) {
      return pendingRequest;
    }

    // 更新状态为加载中
    this.updateState(sourceId, { loading: true, data: [], error: undefined });

    // 创建加载 Promise
    const loadPromise = this.doLoadSource(source);
    this.pendingRequests.set(sourceId, loadPromise);

    try {
      const data = await loadPromise;
      this.updateState(sourceId, { loading: false, data, error: undefined });
      
      // 缓存数据
      this.setCachedData(source, data);
      
      return data;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '加载失败';
      this.updateState(sourceId, { loading: false, data: [], error: errorMessage });
      return [];
    } finally {
      this.pendingRequests.delete(sourceId);
    }
  }

  /**
   * 检查字段变化是否需要刷新数据源
   */
  checkDependencyChange(changedField: string): string[] {
    const sourcesToRefresh: string[] = [];
    
    this.sources.forEach((source, sourceId) => {
      if (source.dependsOn?.includes(changedField)) {
        sourcesToRefresh.push(sourceId);
        // 清除缓存
        this.invalidateCache(sourceId);
      }
    });

    return sourcesToRefresh;
  }

  /**
   * 刷新依赖于指定字段的数据源
   */
  async refreshDependentSources(changedField: string): Promise<void> {
    const sourcesToRefresh = this.checkDependencyChange(changedField);
    
    await Promise.all(
      sourcesToRefresh.map(sourceId => this.loadSource(sourceId, true))
    );
  }

  /**
   * 预加载所有数据源
   */
  async preloadAll(): Promise<void> {
    const loadPromises: Promise<SelectOption[]>[] = [];
    
    this.sources.forEach((source, sourceId) => {
      // 只预加载没有依赖的数据源
      if (!source.dependsOn || source.dependsOn.length === 0) {
        loadPromises.push(this.loadSource(sourceId));
      }
    });

    await Promise.all(loadPromises);
  }

  /**
   * 清除所有缓存
   */
  clearAllCache(): void {
    this.cache.clear();
  }

  /**
   * 清除指定数据源的缓存
   */
  invalidateCache(sourceId: string): void {
    const source = this.sources.get(sourceId);
    if (source) {
      const cacheKey = this.getCacheKey(source);
      this.cache.delete(cacheKey);
    }
  }

  // ========== 私有方法 ==========

  /**
   * 执行数据源加载
   */
  private async doLoadSource(source: ExternalDataSource): Promise<SelectOption[]> {
    switch (source.type) {
      case 'static':
        return source.data || [];
      
      case 'api':
        return this.loadApiSource(source);
      
      case 'terraform':
        // Terraform 数据源暂不支持
        console.warn('Terraform 数据源暂不支持');
        return [];
      
      default:
        console.warn(`未知的数据源类型: ${source.type}`);
        return [];
    }
  }

  /**
   * 加载 API 数据源
   */
  private async loadApiSource(source: ExternalDataSource): Promise<SelectOption[]> {
    if (!source.api) {
      console.warn(`数据源 ${source.id} 缺少 api 配置`);
      return [];
    }

    // 替换 URL 中的变量
    const url = this.replaceVariables(source.api);
    
    // 构建请求参数
    const params = new URLSearchParams();
    if (source.params) {
      Object.entries(source.params).forEach(([key, value]) => {
        const replacedValue = this.replaceVariables(value);
        if (replacedValue) {
          params.append(key, replacedValue);
        }
      });
    }

    // 构建完整 URL
    const fullUrl = params.toString() ? `${url}?${params.toString()}` : url;

    try {
      const response = await fetch(fullUrl, {
        method: source.method || 'GET',
        headers: {
          'Content-Type': 'application/json',
          // 添加认证头（如果需要）
          ...(this.getAuthHeaders()),
        },
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      const data = await response.json();

      // 应用数据转换
      return this.transformData(data, source);
    } catch (error) {
      console.error(`加载数据源 ${source.id} 失败:`, error);
      throw error;
    }
  }

  /**
   * 获取认证头
   */
  private getAuthHeaders(): Record<string, string> {
    const token = localStorage.getItem('token');
    if (token) {
      return { 'Authorization': `Bearer ${token}` };
    }
    return {};
  }

  /**
   * 替换变量
   * 支持的变量格式：
   * - ${providers.aws.region}
   * - ${fields.vpc_id}
   * - ${workspace.id}
   * - ${organization.id}
   */
  private replaceVariables(template: string): string {
    return template.replace(/\$\{([^}]+)\}/g, (match, path) => {
      const parts = path.split('.');
      let value: unknown = undefined;

      switch (parts[0]) {
        case 'providers':
          value = this.getNestedValue(this.context.providers, parts.slice(1));
          break;
        case 'fields':
          value = this.getNestedValue(this.context.values, parts.slice(1));
          break;
        case 'workspace':
          value = this.getNestedValue(this.context.workspace, parts.slice(1));
          break;
        case 'organization':
          value = this.getNestedValue(this.context.organization, parts.slice(1));
          break;
        default:
          // 尝试从 values 中获取
          value = this.getNestedValue(this.context.values, parts);
      }

      return value !== undefined && value !== null ? String(value) : '';
    });
  }

  /**
   * 获取嵌套值
   */
  private getNestedValue(obj: unknown, path: string[]): unknown {
    if (!obj || path.length === 0) {
      return obj;
    }

    let current: unknown = obj;
    for (const key of path) {
      if (current === null || current === undefined) {
        return undefined;
      }
      if (typeof current === 'object') {
        current = (current as Record<string, unknown>)[key];
      } else {
        return undefined;
      }
    }
    return current;
  }

  /**
   * 转换数据
   */
  private transformData(data: unknown, source: ExternalDataSource): SelectOption[] {
    if (!source.transform) {
      // 如果没有转换配置，尝试直接使用数据
      if (Array.isArray(data)) {
        return data.map(item => this.normalizeOption(item));
      }
      return [];
    }

    try {
      let result: unknown;

      switch (source.transform.type) {
        case 'jmespath':
          result = this.applyJMESPath(data, source.transform.expression);
          break;
        case 'jsonpath':
          result = this.applyJSONPath(data, source.transform.expression);
          break;
        default:
          result = data;
      }

      if (Array.isArray(result)) {
        return result.map(item => this.normalizeOption(item));
      }
      return [];
    } catch (error) {
      console.error(`数据转换失败:`, error);
      return [];
    }
  }

  /**
   * 应用 JMESPath 表达式
   * 简化实现，支持基本的路径访问
   */
  private applyJMESPath(data: unknown, expression: string): unknown {
    // 简化的 JMESPath 实现
    // 支持格式: data.items[*].{value: id, label: name}
    
    const parts = expression.split('.');
    let current: unknown = data;

    for (const part of parts) {
      if (current === null || current === undefined) {
        return [];
      }

      // 处理数组映射 [*]
      if (part.includes('[*]')) {
        const arrayKey = part.replace('[*]', '');
        if (arrayKey) {
          current = (current as Record<string, unknown>)[arrayKey];
        }
        if (!Array.isArray(current)) {
          return [];
        }
        continue;
      }

      // 处理对象映射 {value: xxx, label: yyy}
      if (part.startsWith('{') && part.endsWith('}')) {
        const mapping = this.parseObjectMapping(part);
        if (Array.isArray(current)) {
          return current.map(item => {
            const result: Record<string, unknown> = {};
            Object.entries(mapping).forEach(([key, path]) => {
              result[key] = this.getNestedValue(item, path.split('.'));
            });
            return result;
          });
        }
        return [];
      }

      // 普通属性访问
      if (typeof current === 'object' && current !== null) {
        current = (current as Record<string, unknown>)[part];
      } else {
        return [];
      }
    }

    return current;
  }

  /**
   * 解析对象映射表达式
   * 格式: {value: id, label: name, description: desc}
   */
  private parseObjectMapping(expr: string): Record<string, string> {
    const mapping: Record<string, string> = {};
    const content = expr.slice(1, -1); // 去掉 { }
    
    // 简单的解析，支持 key: value 格式
    const pairs = content.split(',');
    pairs.forEach(pair => {
      const [key, value] = pair.split(':').map(s => s.trim());
      if (key && value) {
        mapping[key] = value;
      }
    });

    return mapping;
  }

  /**
   * 应用 JSONPath 表达式
   * 简化实现
   */
  private applyJSONPath(data: unknown, expression: string): unknown {
    // 简化的 JSONPath 实现
    // 支持格式: $.data.items[*]
    
    let path = expression;
    if (path.startsWith('$')) {
      path = path.slice(1);
    }
    if (path.startsWith('.')) {
      path = path.slice(1);
    }

    return this.applyJMESPath(data, path);
  }

  /**
   * 标准化选项格式
   */
  private normalizeOption(item: unknown): SelectOption {
    if (typeof item === 'string') {
      return { value: item, label: item };
    }

    if (typeof item === 'object' && item !== null) {
      const obj = item as Record<string, unknown>;
      return {
        value: String(obj.value ?? obj.id ?? ''),
        label: String(obj.label ?? obj.name ?? obj.value ?? obj.id ?? ''),
        description: obj.description ? String(obj.description) : undefined,
        group: obj.group ? String(obj.group) : undefined,
        disabled: Boolean(obj.disabled),
      };
    }

    return { value: String(item), label: String(item) };
  }

  /**
   * 获取缓存键
   */
  private getCacheKey(source: ExternalDataSource): string {
    if (source.cache?.key) {
      return this.replaceVariables(source.cache.key);
    }
    
    // 默认缓存键：数据源ID + 参数
    const params = source.params 
      ? Object.entries(source.params)
          .map(([k, v]) => `${k}=${this.replaceVariables(v)}`)
          .join('&')
      : '';
    
    return `${source.id}:${params}`;
  }

  /**
   * 获取缓存数据
   */
  private getCachedData(source: ExternalDataSource): SelectOption[] | null {
    if (!source.cache) {
      return null;
    }

    const cacheKey = this.getCacheKey(source);
    const cached = this.cache.get(cacheKey);

    if (!cached) {
      return null;
    }

    // 检查是否过期
    const now = Date.now();
    const ttl = (source.cache.ttl || 300) * 1000; // 转换为毫秒
    
    if (now - cached.timestamp > ttl) {
      this.cache.delete(cacheKey);
      return null;
    }

    return cached.data;
  }

  /**
   * 设置缓存数据
   */
  private setCachedData(source: ExternalDataSource, data: SelectOption[]): void {
    if (!source.cache) {
      return;
    }

    const cacheKey = this.getCacheKey(source);
    this.cache.set(cacheKey, {
      data,
      timestamp: Date.now(),
      key: cacheKey,
    });
  }

  /**
   * 更新状态
   */
  private updateState(sourceId: string, state: DataSourceState): void {
    this.states.set(sourceId, state);
    this.onStateChange?.(sourceId, state);
  }
}

/**
 * 创建数据源管理器的 Hook
 */
export function createDataSourceManager(
  sources: ExternalDataSource[],
  context: FormContext,
  onStateChange?: (sourceId: string, state: DataSourceState) => void
): ExternalDataSourceManager {
  return new ExternalDataSourceManager({
    sources,
    context,
    onStateChange,
  });
}
