import React, { useState, useEffect } from 'react';
import api from '../services/api';
import { useToast } from '../contexts/ToastContext';
import styles from './ResourceVersionHistory.module.css';

interface Version {
  id: number;
  version: number;
  change_summary: string;
  created_at: string;
  is_latest: boolean;
  created_by?: number;
}

interface Props {
  isOpen: boolean;
  workspaceId: string;
  resourceId: string;
  currentVersion: number;
  onClose: () => void;
  onViewVersion: (version: Version) => void;
  onCompareVersion: (version: Version) => void;
}

const ResourceVersionHistory: React.FC<Props> = ({
  isOpen,
  workspaceId,
  resourceId,
  currentVersion,
  onClose,
  onViewVersion,
  onCompareVersion
}) => {
  const [versions, setVersions] = useState<Version[]>([]);
  const [loading, setLoading] = useState(false);
  const { showToast } = useToast();

  useEffect(() => {
    if (isOpen) {
      loadVersions();
    }
  }, [isOpen, workspaceId, resourceId]);

  const loadVersions = async () => {
    try {
      setLoading(true);
      const response: any = await api.get(
        `/workspaces/${workspaceId}/resources/${resourceId}/versions`
      );
      
      const versionsData = response.data?.versions || response.versions || [];
      setVersions(versionsData);
    } catch (error: any) {
      showToast('加载版本历史失败', 'error');
      console.error('Failed to load versions:', error);
    } finally {
      setLoading(false);
    }
  };

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    });
  };

  if (!isOpen) return null;

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h2 className={styles.title}>版本历史</h2>
          <button className={styles.closeButton} onClick={onClose}>
            ×
          </button>
        </div>

        <div className={styles.body}>
          {loading ? (
            <div className={styles.loading}>
              <div className={styles.spinner}></div>
              <span>加载中...</span>
            </div>
          ) : versions.length === 0 ? (
            <div className={styles.empty}>暂无历史版本</div>
          ) : (
            <div className={styles.versionList}>
              {versions.map((version) => (
                <div
                  key={version.id}
                  className={`${styles.versionItem} ${
                    version.version === currentVersion ? styles.current : ''
                  }`}
                >
                  <div className={styles.versionInfo}>
                    <div className={styles.versionHeader}>
                      <span className={styles.versionNumber}>
                        v{version.version}
                      </span>
                      {version.is_latest && (
                        <span className={styles.latestBadge}>Latest</span>
                      )}
                      {version.version === currentVersion && (
                        <span className={styles.currentBadge}>Current</span>
                      )}
                    </div>
                    <div className={styles.versionMeta}>
                      <span className={styles.versionDate}>
                        {formatDate(version.created_at)}
                      </span>
                      {version.created_by && (
                        <span className={styles.versionAuthor}>
                          User #{version.created_by}
                        </span>
                      )}
                    </div>
                    {version.change_summary && (
                      <div className={styles.versionSummary}>
                        {version.change_summary}
                      </div>
                    )}
                  </div>

                  <div className={styles.versionActions}>
                    <button
                      className={styles.btnView}
                      onClick={() => onViewVersion(version)}
                      title="查看此版本"
                    >
                      查看
                    </button>
                    {version.version !== currentVersion && (
                      <button
                        className={styles.btnCompare}
                        onClick={() => onCompareVersion(version)}
                        title="对比与最新版本的差异"
                      >
                        对比
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className={styles.footer}>
          <button className={styles.btnClose} onClick={onClose}>
            关闭
          </button>
        </div>
      </div>
    </div>
  );
};

export default ResourceVersionHistory;
