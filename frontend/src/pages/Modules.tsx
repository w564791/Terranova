import React, { useState, useEffect, useMemo } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { moduleService, type Module } from '../services/modules';
import styles from './Modules.module.css';

console.log('Modules component loaded');

// 格式化日期
const formatDate = (dateString: string): string => {
  if (!dateString) return '-';
  const date = new Date(dateString);
  return date.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  });
};

const Modules: React.FC = () => {
  console.log('Modules component rendering');
  const navigate = useNavigate();
  const [modules, setModules] = useState<Module[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedProviders, setSelectedProviders] = useState<string[]>([]);
  const [selectedStatuses, setSelectedStatuses] = useState<string[]>([]);

  const loadModules = async () => {
    console.log('Loading modules...');
    try {
      setLoading(true);
      setError(null);
      const response = await moduleService.getModules();
      console.log('Module API response:', response);
      const responseData = response.data as any;
      const modules = Array.isArray(responseData) ? responseData : (responseData?.items || []);
      setModules(modules);
    } catch (err: any) {
      console.error('API error:', err);
      const errorMessage = err.message === 'Failed to fetch' 
        ? '网络连接失败，请检查网络或稍后重试'
        : err.message || '加载模块列表失败';
      setError(errorMessage);
      setModules([
        {
          id: 1,
          name: 'aws-vpc',
          description: 'AWS VPC模块，用于创建虚拟私有云网络，支持多可用区部署和自定义子网配置',
          version: '1.0.0',
          provider: 'AWS',
          status: 'active',
          source: 'github.com/terraform-aws-modules/terraform-aws-vpc',
          created_at: '2024-01-01',
          updated_at: '2024-01-15'
        },
        {
          id: 2,
          name: 'azure-vm',
          description: 'Azure虚拟机模块，支持Windows和Linux系统，可配置网络、存储和安全组',
          version: '2.1.0',
          provider: 'Azure',
          status: 'active',
          source: 'github.com/Azure/terraform-azurerm-compute',
          created_at: '2024-01-02',
          updated_at: '2024-01-20'
        },
        {
          id: 3,
          name: 'gcp-gke',
          description: 'Google Kubernetes Engine集群模块，支持自动扩缩容和节点池管理',
          version: '3.0.0',
          provider: 'GCP',
          status: 'inactive',
          source: 'github.com/terraform-google-modules/terraform-google-kubernetes-engine',
          created_at: '2024-01-05',
          updated_at: '2024-01-25'
        }
      ]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadModules();
  }, []);

  // 获取所有唯一的 provider 及其数量
  const providerStats = useMemo(() => {
    const stats: Record<string, number> = {};
    modules.forEach(m => {
      stats[m.provider] = (stats[m.provider] || 0) + 1;
    });
    return stats;
  }, [modules]);

  // 获取状态统计
  const statusStats = useMemo(() => {
    const stats = { active: 0, inactive: 0 };
    modules.forEach(m => {
      if (m.status === 'active') stats.active++;
      else stats.inactive++;
    });
    return stats;
  }, [modules]);

  // 过滤模块
  const filteredModules = useMemo(() => {
    return modules.filter(module => {
      // 搜索过滤
      const searchLower = searchTerm.toLowerCase();
      const matchesSearch = !searchTerm || 
        module.name.toLowerCase().includes(searchLower) ||
        (module.description && module.description.toLowerCase().includes(searchLower)) ||
        module.provider.toLowerCase().includes(searchLower);

      // Provider过滤
      const matchesProvider = selectedProviders.length === 0 || selectedProviders.includes(module.provider);

      // Status过滤
      const matchesStatus = selectedStatuses.length === 0 || selectedStatuses.includes(module.status);

      return matchesSearch && matchesProvider && matchesStatus;
    });
  }, [modules, searchTerm, selectedProviders, selectedStatuses]);

  // 切换 provider 选择
  const toggleProvider = (provider: string) => {
    setSelectedProviders(prev => 
      prev.includes(provider) 
        ? prev.filter(p => p !== provider)
        : [...prev, provider]
    );
  };

  // 切换 status 选择
  const toggleStatus = (status: string) => {
    setSelectedStatuses(prev => 
      prev.includes(status) 
        ? prev.filter(s => s !== status)
        : [...prev, status]
    );
  };

  // 重置所有筛选
  const resetFilters = () => {
    setSearchTerm('');
    setSelectedProviders([]);
    setSelectedStatuses([]);
  };

  const hasActiveFilters = searchTerm || selectedProviders.length > 0 || selectedStatuses.length > 0;

  if (loading) {
    return <div className={styles.loading}>加载中...</div>;
  }

  if (error) {
    return (
      <div className={styles.error}>
        <p className={styles.errorMessage}>{error}</p>
        <button onClick={loadModules} className={styles.retryButton}>重新加载</button>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <h1 className={styles.pageTitle}>模块库</h1>
          <span className={styles.moduleCount}>{modules.length} 个模块</span>
        </div>
        <button 
          className={styles.addButton}
          onClick={() => navigate('/modules/import')}
        >
          <span>+</span>
          <span>导入模块</span>
        </button>
      </div>

      <div className={styles.mainContent}>
        {/* 左侧筛选栏 */}
        <div className={styles.filterSidebar}>
          <div className={styles.filterSection}>
            <div className={styles.filterHeader}>
              <span>Filters</span>
            </div>
            <div className={styles.filterContent}>
              {/* 搜索框 */}
              <div className={styles.searchBox}>
                <input
                  type="text"
                  placeholder="搜索模块..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className={styles.searchInput}
                />
                {searchTerm && (
                  <button 
                    onClick={() => setSearchTerm('')}
                    className={styles.clearButton}
                  >
                    ×
                  </button>
                )}
              </div>

              {/* Providers 筛选 */}
              <div className={styles.filterGroup}>
                <div className={styles.filterLabel}>Providers</div>
                {Object.entries(providerStats).map(([provider, count]) => (
                  <label key={provider} className={styles.filterOption}>
                    <input
                      type="checkbox"
                      checked={selectedProviders.includes(provider)}
                      onChange={() => toggleProvider(provider)}
                    />
                    <span>{provider}</span>
                    <span className={styles.filterCount}>{count}</span>
                  </label>
                ))}
              </div>

              {/* Status 筛选 */}
              <div className={styles.filterGroup}>
                <div className={styles.filterLabel}>Status</div>
                <label className={styles.filterOption}>
                  <input
                    type="checkbox"
                    checked={selectedStatuses.includes('active')}
                    onChange={() => toggleStatus('active')}
                  />
                  <span>活跃</span>
                  <span className={styles.filterCount}>{statusStats.active}</span>
                </label>
                <label className={styles.filterOption}>
                  <input
                    type="checkbox"
                    checked={selectedStatuses.includes('inactive')}
                    onChange={() => toggleStatus('inactive')}
                  />
                  <span>停用</span>
                  <span className={styles.filterCount}>{statusStats.inactive}</span>
                </label>
              </div>

              {/* 重置按钮 */}
              {hasActiveFilters && (
                <button onClick={resetFilters} className={styles.resetButton}>
                  重置筛选
                </button>
              )}
            </div>
          </div>
        </div>

        {/* 右侧内容区域 */}
        <div className={styles.contentArea}>
          {/* 结果统计 */}
          <div className={styles.resultCount}>
            找到 <strong>{filteredModules.length}</strong> 个模块
            {filteredModules.length !== modules.length && (
              <span className={styles.totalCount}>（共 {modules.length} 个）</span>
            )}
          </div>
          
          <div className={styles.moduleList}>
            {filteredModules.length === 0 ? (
              <div className={styles.emptyState}>
                <p className={styles.emptyMessage}>没有找到匹配的模块</p>
                <button onClick={resetFilters} className={styles.resetButton}>
                  清除筛选条件
                </button>
              </div>
            ) : (
              filteredModules.map(module => (
                <Link 
                  key={module.id} 
                  to={`/modules/${module.id}`}
                  className={styles.moduleCard}
                >
                  <div className={styles.cardHeader}>
                    <div className={styles.cardTitleSection}>
                      <h3 className={styles.moduleName}>{module.name}</h3>
                      {module.source && (
                        <div className={styles.moduleSource}>{module.source}</div>
                      )}
                    </div>
                    <span className={`${styles.status} ${styles[module.status]}`}>
                      {module.status === 'active' ? '活跃' : '停用'}
                    </span>
                  </div>

                  <p className={styles.description}>
                    {module.description || '暂无描述'}
                  </p>

                  <div className={styles.cardFooter}>
                    <span className={styles.provider}>{module.provider}</span>
                    <span className={styles.version}>v{module.version}</span>
                    <span className={styles.dateInfo}>
                      更新于 {formatDate(module.updated_at)}
                    </span>
                  </div>
                </Link>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default Modules;
