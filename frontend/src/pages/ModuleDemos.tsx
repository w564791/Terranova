import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { moduleDemoService, type ModuleDemo } from '../services/moduleDemos';
import { listVersions, importDemos, type ModuleVersion } from '../services/moduleVersions';
import api from '../services/api';
import styles from './ModuleDemos.module.css';

interface Module {
  id: number;
  name: string;
  provider: string;
  default_version_id?: string;
}

const ModuleDemos: React.FC = () => {
  const { moduleId } = useParams<{ moduleId: string }>();
  const [searchParams] = useSearchParams();
  const urlVersionId = searchParams.get('version_id');
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [module, setModule] = useState<Module | null>(null);
  const [demos, setDemos] = useState<ModuleDemo[]>([]);
  const [versions, setVersions] = useState<ModuleVersion[]>([]);
  const [selectedVersionId, setSelectedVersionId] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [showImportModal, setShowImportModal] = useState(false);
  const [importSourceVersionId, setImportSourceVersionId] = useState<string>('');
  const [sourceDemos, setSourceDemos] = useState<ModuleDemo[]>([]);
  const [importing, setImporting] = useState(false);

  // 获取有 Demo 的版本列表（用于导入选择）
  const versionsWithDemos = versions.filter(v => 
    v.id !== selectedVersionId && (v.demo_count || 0) > 0
  );

  useEffect(() => {
    const fetchData = async () => {
      try {
        // 获取模块信息
        const moduleRes = await api.get(`/modules/${moduleId}`);
        setModule(moduleRes.data);

        // 获取版本列表
        const versionsRes = await listVersions(Number(moduleId));
        setVersions(versionsRes.items || []);

        // 设置选中的版本：优先使用 URL 参数，其次使用默认版本
        if (urlVersionId) {
          // 验证 URL 中的版本 ID 是否有效
          const validVersion = versionsRes.items?.find(v => v.id === urlVersionId);
          if (validVersion) {
            setSelectedVersionId(urlVersionId);
          } else {
            // 无效的版本 ID，使用默认版本
            const defaultVersionId = moduleRes.data.default_version_id;
            if (defaultVersionId) {
              setSelectedVersionId(defaultVersionId);
            } else if (versionsRes.items?.length > 0) {
              setSelectedVersionId(versionsRes.items[0].id);
            }
          }
        } else {
          // 没有 URL 参数，使用默认版本
          const defaultVersionId = moduleRes.data.default_version_id;
          if (defaultVersionId) {
            setSelectedVersionId(defaultVersionId);
          } else if (versionsRes.items?.length > 0) {
            setSelectedVersionId(versionsRes.items[0].id);
          }
        }
      } catch (error) {
        const message = extractErrorMessage(error);
        showToast(message, 'error');
        navigate('/modules');
      } finally {
        setLoading(false);
      }
    };

    if (moduleId) {
      fetchData();
    }
  }, [moduleId, urlVersionId, navigate, showToast]);

  // 当选中版本变化时，加载该版本的 Demo
  useEffect(() => {
    const fetchDemos = async () => {
      if (!selectedVersionId || !moduleId) return;
      
      try {
        const demosData = await moduleDemoService.getDemosByModuleId(
          Number(moduleId),
          selectedVersionId
        );
        setDemos(demosData || []);
      } catch (error) {
        console.error('Failed to load demos:', error);
        setDemos([]);
      }
    };

    fetchDemos();
  }, [moduleId, selectedVersionId]);

  // 当选择导入源版本时，加载该版本的 Demo
  useEffect(() => {
    const fetchSourceDemos = async () => {
      if (!importSourceVersionId || !moduleId) {
        setSourceDemos([]);
        return;
      }
      
      try {
        const demosData = await moduleDemoService.getDemosByModuleId(
          Number(moduleId),
          importSourceVersionId
        );
        setSourceDemos(demosData || []);
      } catch (error) {
        console.error('Failed to load source demos:', error);
        setSourceDemos([]);
      }
    };

    fetchSourceDemos();
  }, [moduleId, importSourceVersionId]);

  const handleImportDemos = async () => {
    if (!importSourceVersionId || !selectedVersionId || !moduleId) return;

    setImporting(true);
    try {
      const result = await importDemos(Number(moduleId), selectedVersionId, {
        from_version_id: importSourceVersionId
      });
      showToast(`成功导入 ${result.imported_count} 个 Demo`, 'success');
      setShowImportModal(false);
      setImportSourceVersionId('');
      
      // 刷新 Demo 列表
      const demosData = await moduleDemoService.getDemosByModuleId(
        Number(moduleId),
        selectedVersionId
      );
      setDemos(demosData || []);
      
      // 刷新版本列表（更新 demo_count）
      const versionsRes = await listVersions(Number(moduleId));
      setVersions(versionsRes.items || []);
    } catch (error) {
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setImporting(false);
    }
  };

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit'
    });
  };

  const selectedVersion = versions.find(v => v.id === selectedVersionId);

  if (loading) {
    return (
      <div className={styles.pageWrapper}>
        <div className={styles.container}>
          <div className={styles.loading}>
            <div className={styles.spinner}></div>
          </div>
        </div>
      </div>
    );
  }

  if (!module) {
    return (
      <div className={styles.pageWrapper}>
        <div className={styles.container}>
          <div className={styles.error}>模块不存在</div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.pageWrapper}>
      <div className={styles.container}>
        {/* 面包屑导航 */}
        <nav className={styles.breadcrumb}>
          <span onClick={() => navigate('/modules')} className={styles.breadcrumbLink}>模块</span>
          <span className={styles.breadcrumbSep}>/</span>
          <span onClick={() => navigate(`/modules/${moduleId}`)} className={styles.breadcrumbLink}>{module.name}</span>
          <span className={styles.breadcrumbSep}>/</span>
          <span className={styles.breadcrumbCurrent}>Demo</span>
        </nav>

        {/* 页面头部 */}
        <div className={styles.pageHeader}>
          <div className={styles.headerInfo}>
            <h1 className={styles.pageTitle}>Demo 配置</h1>
            {/* 版本选择器 */}
            <div className={styles.versionSelector}>
              <label>版本：</label>
              <select
                value={selectedVersionId}
                onChange={(e) => setSelectedVersionId(e.target.value)}
                className={styles.versionSelect}
              >
                {versions.map(v => (
                  <option key={v.id} value={v.id}>
                    {v.version} {v.is_default ? '(默认)' : ''} 
                    {v.demo_count ? ` - ${v.demo_count} 个 Demo` : ''}
                  </option>
                ))}
              </select>
            </div>
            <span className={styles.demoCount}>{demos.length} 个配置</span>
          </div>
          {/* 操作按钮 */}
          <div className={styles.headerActions}>
            {versionsWithDemos.length > 0 && (
              <button 
                onClick={() => setShowImportModal(true)} 
                className={styles.secondaryBtn}
              >
                从其他版本导入
              </button>
            )}
            <button 
              onClick={() => navigate(`/modules/${moduleId}/demos/create?version_id=${selectedVersionId}`)} 
              className={styles.primaryBtn}
            >
              创建 Demo
            </button>
          </div>
        </div>

        {/* Demo 列表 */}
        {demos.length === 0 ? (
          <div className={styles.emptyState}>
            <div className={styles.emptyIcon}>
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                <circle cx="12" cy="12" r="10"></circle>
                <polygon points="10 8 16 12 10 16 10 8"></polygon>
              </svg>
            </div>
            <h3>暂无 Demo 配置</h3>
            <p>创建第一个 Demo 来快速部署预设配置</p>
            <div className={styles.emptyActions}>
              {versionsWithDemos.length > 0 && (
                <button 
                  onClick={() => setShowImportModal(true)} 
                  className={styles.secondaryBtn}
                >
                  从其他版本导入
                </button>
              )}
              <button 
                onClick={() => navigate(`/modules/${moduleId}/demos/create?version_id=${selectedVersionId}`)} 
                className={styles.primaryBtn}
              >
                创建 Demo
              </button>
            </div>
          </div>
        ) : (
          <div className={styles.demoGrid}>
            {demos.map((demo) => (
              <div 
                key={demo.id} 
                className={styles.demoCard}
                onClick={() => navigate(`/modules/${moduleId}/demos/${demo.id}`)}
              >
                <div className={styles.cardHeader}>
                  <h3 className={styles.demoName}>{demo.name}</h3>
                  <span className={styles.versionBadge}>v{demo.current_version?.version || 1}</span>
                </div>
                {demo.description && (
                  <p className={styles.demoDesc}>{demo.description}</p>
                )}
                <div className={styles.cardFooter}>
                  <span className={styles.updateTime}>更新于 {formatDate(demo.updated_at)}</span>
                  <span className={styles.cardArrow}>→</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 导入 Demo 弹窗 */}
      {showImportModal && (
        <div className={styles.modalOverlay} onClick={() => setShowImportModal(false)}>
          <div className={styles.modal} onClick={e => e.stopPropagation()}>
            <div className={styles.modalHeader}>
              <h2>导入 Demo</h2>
              <button 
                className={styles.closeBtn}
                onClick={() => setShowImportModal(false)}
              >
                ×
              </button>
            </div>
            <div className={styles.modalBody}>
              <p className={styles.modalDesc}>
                将其他版本的 Demo 导入到当前版本 <strong>{selectedVersion?.version}</strong>
              </p>
              
              <div className={styles.formGroup}>
                <label>选择源版本</label>
                <select
                  value={importSourceVersionId}
                  onChange={(e) => setImportSourceVersionId(e.target.value)}
                  className={styles.select}
                >
                  <option value="">请选择版本</option>
                  {versionsWithDemos.map(v => (
                    <option key={v.id} value={v.id}>
                      {v.version} {v.is_default ? '(默认)' : ''} - {v.demo_count} 个 Demo
                    </option>
                  ))}
                </select>
              </div>

              {importSourceVersionId && sourceDemos.length > 0 && (
                <div className={styles.previewSection}>
                  <h4>将导入以下 Demo：</h4>
                  <ul className={styles.demoList}>
                    {sourceDemos.map(demo => (
                      <li key={demo.id}>
                        <span className={styles.demoListName}>{demo.name}</span>
                        {demo.description && (
                          <span className={styles.demoListDesc}>{demo.description}</span>
                        )}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
            <div className={styles.modalFooter}>
              <button 
                className={styles.cancelBtn}
                onClick={() => setShowImportModal(false)}
              >
                取消
              </button>
              <button 
                className={styles.primaryBtn}
                onClick={handleImportDemos}
                disabled={!importSourceVersionId || importing}
              >
                {importing ? '导入中...' : `导入 ${sourceDemos.length} 个 Demo`}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default ModuleDemos;
