/**
 * 外部数据源 Hook
 * 
 * 提供 React 组件使用外部数据源的便捷方式
 */

import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { ExternalDataSourceManager, createDataSourceManager } from './ExternalDataSourceManager';
import type { DataSourceState } from './ExternalDataSourceManager';
import type { ExternalDataSource, SelectOption, FormContext } from './types';

// Hook 返回类型
export interface UseExternalDataSourceResult {
  // 数据源状态
  states: Map<string, DataSourceState>;
  // 获取指定数据源的选项
  getOptions: (sourceId: string) => SelectOption[];
  // 获取指定数据源的加载状态
  isLoading: (sourceId: string) => boolean;
  // 获取指定数据源的错误
  getError: (sourceId: string) => string | undefined;
  // 手动加载数据源
  loadSource: (sourceId: string, forceRefresh?: boolean) => Promise<SelectOption[]>;
  // 刷新依赖于指定字段的数据源
  refreshDependentSources: (changedField: string) => Promise<void>;
  // 预加载所有数据源
  preloadAll: () => Promise<void>;
  // 清除所有缓存
  clearAllCache: () => void;
  // 数据源管理器实例
  manager: ExternalDataSourceManager | null;
}

// Hook 配置
export interface UseExternalDataSourceConfig {
  sources: ExternalDataSource[];
  context: FormContext;
  autoPreload?: boolean;  // 是否自动预加载
}

/**
 * 外部数据源 Hook
 */
export function useExternalDataSource(config: UseExternalDataSourceConfig): UseExternalDataSourceResult {
  const { sources, context, autoPreload = true } = config;
  
  // 状态
  const [states, setStates] = useState<Map<string, DataSourceState>>(new Map());
  const [, forceUpdate] = useState({});
  
  // 管理器引用
  const managerRef = useRef<ExternalDataSourceManager | null>(null);
  
  // 状态变化回调
  const handleStateChange = useCallback((sourceId: string, state: DataSourceState) => {
    setStates(prev => {
      const next = new Map(prev);
      next.set(sourceId, state);
      return next;
    });
  }, []);

  // 创建/更新管理器
  useEffect(() => {
    if (sources.length === 0) {
      managerRef.current = null;
      return;
    }

    // 创建新的管理器
    const manager = createDataSourceManager(sources, context, handleStateChange);
    managerRef.current = manager;

    // 初始化状态
    const initialStates = new Map<string, DataSourceState>();
    sources.forEach(source => {
      initialStates.set(source.id, { loading: false, data: [] });
    });
    setStates(initialStates);

    // 自动预加载
    if (autoPreload) {
      manager.preloadAll().catch(console.error);
    }

    // 清理
    return () => {
      managerRef.current = null;
    };
  }, [sources, handleStateChange, autoPreload]);

  // 更新上下文
  useEffect(() => {
    if (managerRef.current) {
      managerRef.current.updateContext(context);
    }
  }, [context]);

  // 获取选项
  const getOptions = useCallback((sourceId: string): SelectOption[] => {
    return states.get(sourceId)?.data || [];
  }, [states]);

  // 获取加载状态
  const isLoading = useCallback((sourceId: string): boolean => {
    return states.get(sourceId)?.loading || false;
  }, [states]);

  // 获取错误
  const getError = useCallback((sourceId: string): string | undefined => {
    return states.get(sourceId)?.error;
  }, [states]);

  // 加载数据源
  const loadSource = useCallback(async (sourceId: string, forceRefresh = false): Promise<SelectOption[]> => {
    if (!managerRef.current) {
      return [];
    }
    return managerRef.current.loadSource(sourceId, forceRefresh);
  }, []);

  // 刷新依赖数据源
  const refreshDependentSources = useCallback(async (changedField: string): Promise<void> => {
    if (!managerRef.current) {
      return;
    }
    await managerRef.current.refreshDependentSources(changedField);
  }, []);

  // 预加载所有
  const preloadAll = useCallback(async (): Promise<void> => {
    if (!managerRef.current) {
      return;
    }
    await managerRef.current.preloadAll();
  }, []);

  // 清除缓存
  const clearAllCache = useCallback((): void => {
    if (managerRef.current) {
      managerRef.current.clearAllCache();
      forceUpdate({});
    }
  }, []);

  return {
    states,
    getOptions,
    isLoading,
    getError,
    loadSource,
    refreshDependentSources,
    preloadAll,
    clearAllCache,
    manager: managerRef.current,
  };
}

/**
 * 单个数据源 Hook
 * 用于只需要一个数据源的场景
 */
export interface UseSingleDataSourceResult {
  options: SelectOption[];
  loading: boolean;
  error?: string;
  reload: (forceRefresh?: boolean) => Promise<void>;
}

export function useSingleDataSource(
  source: ExternalDataSource | undefined,
  context: FormContext
): UseSingleDataSourceResult {
  const [options, setOptions] = useState<SelectOption[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | undefined>();
  
  const managerRef = useRef<ExternalDataSourceManager | null>(null);

  // 创建管理器
  useEffect(() => {
    if (!source) {
      setOptions([]);
      setLoading(false);
      setError(undefined);
      return;
    }

    const manager = createDataSourceManager(
      [source],
      context,
      (_, state) => {
        setOptions(state.data);
        setLoading(state.loading);
        setError(state.error);
      }
    );
    managerRef.current = manager;

    // 自动加载
    manager.loadSource(source.id).catch(console.error);

    return () => {
      managerRef.current = null;
    };
  }, [source?.id]);

  // 更新上下文
  useEffect(() => {
    if (managerRef.current) {
      managerRef.current.updateContext(context);
    }
  }, [context]);

  // 重新加载
  const reload = useCallback(async (forceRefresh = true): Promise<void> => {
    if (!managerRef.current || !source) {
      return;
    }
    await managerRef.current.loadSource(source.id, forceRefresh);
  }, [source?.id]);

  return {
    options,
    loading,
    error,
    reload,
  };
}

/**
 * 字段数据源 Hook
 * 根据字段的 UI 配置自动获取数据源
 */
export interface UseFieldDataSourceResult {
  options: SelectOption[];
  loading: boolean;
  error?: string;
  reload: () => Promise<void>;
}

export function useFieldDataSource(
  fieldName: string,
  sourceId: string | undefined,
  sources: ExternalDataSource[],
  context: FormContext
): UseFieldDataSourceResult {
  // 查找数据源配置
  const source = useMemo(() => {
    if (!sourceId) return undefined;
    return sources.find(s => s.id === sourceId);
  }, [sourceId, sources]);

  return useSingleDataSource(source, context);
}

/**
 * 数据源上下文 Provider 的类型定义
 */
export interface DataSourceContextValue {
  manager: ExternalDataSourceManager | null;
  getOptions: (sourceId: string) => SelectOption[];
  isLoading: (sourceId: string) => boolean;
  getError: (sourceId: string) => string | undefined;
  loadSource: (sourceId: string, forceRefresh?: boolean) => Promise<SelectOption[]>;
  refreshDependentSources: (changedField: string) => Promise<void>;
}
