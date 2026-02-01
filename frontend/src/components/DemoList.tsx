import React, { useState, useEffect } from 'react';
import { moduleDemoService, type ModuleDemo } from '../services/moduleDemos';
import styles from './DemoList.module.css';

interface DemoListProps {
  moduleId: number;
  onCreateDemo: () => void;
  onDemoClick: (demo: ModuleDemo) => void;
  refreshTrigger?: number;
  showHeader?: boolean;  // 是否显示标题和创建按钮，默认 true
}

const DemoList: React.FC<DemoListProps> = ({
  moduleId,
  onCreateDemo,
  onDemoClick,
  refreshTrigger,
  showHeader = true,
}) => {
  const [demos, setDemos] = useState<ModuleDemo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (moduleId) {
      loadDemos();
    }
  }, [moduleId, refreshTrigger]);

  const loadDemos = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await moduleDemoService.getDemosByModuleId(moduleId);
      console.log('📊 Loaded demos:', data);
      setDemos(data || []);
    } catch (err: any) {
      setError(err.response?.data?.error || 'Failed to load demos');
      console.error('Error loading demos:', err);
      setDemos([]);
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (demo: ModuleDemo) => {
    if (!window.confirm(`确定要删除 Demo "${demo.name}" 吗？`)) {
      return;
    }

    try {
      await moduleDemoService.deleteDemo(demo.id);
      await loadDemos();
    } catch (err: any) {
      alert(err.response?.data?.error || 'Failed to delete demo');
      console.error('Error deleting demo:', err);
    }
  };

  const formatDate = (dateString: string) => {
    // 确保正确解析UTC时间
    const date = new Date(dateString);
    const now = new Date();
    
    // 使用UTC时间计算差异，避免时区问题
    const diffMs = now.getTime() - date.getTime();
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffMinutes = Math.floor(diffMs / (1000 * 60));

    if (diffMinutes < 1) {
      return 'Just now';
    } else if (diffMinutes < 60) {
      return `${diffMinutes}m ago`;
    } else if (diffHours < 24) {
      return `${diffHours}h ago`;
    } else if (diffDays === 1) {
      return '1d ago';
    } else if (diffDays < 7) {
      return `${diffDays}d ago`;
    } else if (diffDays < 30) {
      return `${Math.floor(diffDays / 7)}w ago`;
    } else {
      return date.toLocaleDateString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit'
      });
    }
  };

  if (loading) {
    return <div className={styles.loading}>Loading demos...</div>;
  }

  if (error) {
    return (
      <div className={styles.error}>
        <p>{error}</p>
        <button onClick={loadDemos}>Retry</button>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {showHeader && (
        <div className={styles.header}>
          <h2>Demo 配置</h2>
          <button className={styles.createButton} onClick={onCreateDemo}>
            创建 Demo
          </button>
        </div>
      )}

      {!demos || demos.length === 0 ? (
        <div className={styles.empty}>
          <p>暂无 Demo 配置</p>
          {!showHeader && (
            <button className={styles.createButton} onClick={onCreateDemo}>
              创建 Demo
            </button>
          )}
        </div>
      ) : (
        <div className={styles.list}>
          {demos.map((demo) => (
            <div 
              key={demo.id} 
              className={styles.demoCard}
              onClick={() => onDemoClick(demo)}
              style={{ cursor: 'pointer' }}
            >
              <div className={styles.demoCardTop}>
                <h3 className={styles.demoName}>{demo.name}</h3>
                <div className={styles.demoMeta}>
                  <span className={styles.version}>
                    v{demo.current_version?.version || 1}
                  </span>
                  <span className={styles.date}>
                    {formatDate(demo.updated_at)}
                  </span>
                </div>
              </div>

              {demo.description && (
                <p className={styles.description}>{demo.description}</p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default DemoList;
